package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/gofont/gosmallcaps"
	"golang.org/x/image/font/gofont/gosmallcapsitalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// ---------- config ----------

const (
	maxW, maxH, maxFrames = 800, 400, 120
)

var (
	cacheDir = getEnv("CACHE_DIR", "cache")
	port     = getEnv("PORT", "8080")
	fontsDir = getEnv("FONTS_DIR", "fonts")
	faceMu   sync.Mutex
	faceMap  = map[faceKey]font.Face{}
	nameRe   = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)
)

const defaultFont = "monobold"

type faceKey struct {
	name string
	size float64
}

// font có sẵn trong golang.org/x/image (không cần tải thêm)
var builtinFonts = map[string][]byte{
	"mono":            gomono.TTF,
	"monobold":        gomonobold.TTF,
	"monoitalic":      gomonoitalic.TTF,
	"monobolditalic":  gomonobolditalic.TTF,
	"regular":         goregular.TTF,
	"bold":            gobold.TTF,
	"italic":          goitalic.TTF,
	"bolditalic":      gobolditalic.TTF,
	"smallcaps":       gosmallcaps.TTF,
	"smallcapsitalic": gosmallcapsitalic.TTF,
}

// chuẩn hoá tên font; trả về defaultFont nếu không tồn tại
func resolveFont(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || !nameRe.MatchString(name) {
		return defaultFont
	}
	if _, ok := builtinFonts[name]; ok {
		return name
	}
	for _, ext := range []string{".ttf", ".otf"} {
		if _, err := os.Stat(filepath.Join(fontsDir, name+ext)); err == nil {
			return name
		}
	}
	return defaultFont
}

func fontData(name string) []byte {
	if b, ok := builtinFonts[name]; ok {
		return b
	}
	for _, ext := range []string{".ttf", ".otf"} {
		if b, err := os.ReadFile(filepath.Join(fontsDir, name+ext)); err == nil {
			return b
		}
	}
	return builtinFonts[defaultFont]
}

func listFonts() []string {
	seen := map[string]bool{}
	for k := range builtinFonts {
		seen[k] = true
	}
	if ents, err := os.ReadDir(fontsDir); err == nil {
		for _, e := range ents {
			n := strings.ToLower(e.Name())
			ext := filepath.Ext(n)
			if ext == ".ttf" || ext == ".otf" {
				b := strings.TrimSuffix(n, ext)
				if nameRe.MatchString(b) {
					seen[b] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getFace(name string, size float64) font.Face {
	faceMu.Lock()
	defer faceMu.Unlock()
	k := faceKey{name, size}
	if f, ok := faceMap[k]; ok {
		return f
	}
	parsed, err := opentype.Parse(fontData(name))
	if err != nil {
		log.Printf("font %q lỗi (%v), dùng %s", name, err, defaultFont)
		parsed, err = opentype.Parse(builtinFonts[defaultFont])
		if err != nil {
			log.Fatal(err)
		}
	}
	f, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatal(err)
	}
	faceMap[k] = f
	return f
}

// ---------- emoji fallback ----------

// danh sách đường dẫn font emoji phổ biến trên các hệ điều hành
var emojiFontPaths = []string{
	"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
	"/usr/share/fonts/truetype/noto-emoji/NotoColorEmoji.ttf",
	"/usr/share/fonts/noto/NotoColorEmoji.ttf",
	"/usr/share/fonts/google-noto-emoji/NotoColorEmoji.ttf",
	"/usr/share/fonts/truetype/ancient-scripts/Symbola_hint.ttf",
	"/System/Library/Fonts/Apple Color Emoji.ttc",
	"/System/Library/Fonts/AppleColorEmoji.ttc",
	"/Library/Fonts/Apple Color Emoji.ttc",
	"/mnt/c/Windows/Fonts/seguiemj.ttf",
	"C:\\Windows\\Fonts\\seguiemj.ttf",
	"/data/data/com.termux/files/usr/share/fonts/TTF/NotoColorEmoji.ttf",
	"/data/data/com.termux/files/usr/share/fonts/noto/NotoColorEmoji.ttf",
}

var (
	emojiFaceOnce sync.Once
	emojiFaceCache font.Face
	emojiParsed   *opentype.Font
)

// scanEmojiFonts quét toàn bộ thư mục font hệ thống + fontsDir,
// trả về danh sách path của mọi file font có tên chứa "emoji".
func scanEmojiFonts() []string {
	roots := []string{
		"/usr/share/fonts",
		"/usr/local/share/fonts",
		"/usr/share/fonts/truetype",
		"/System/Library/Fonts",
		"/Library/Fonts",
		"~/.fonts",
		"~/.local/share/fonts",
		"/data/data/com.termux/files/usr/share/fonts",
		"/data/data/com.termux/files/home/.termux/fonts",
		"/data/data/com.termux/files/home/.fonts",
		fontsDir,
	}

	var out []string
	seen := map[string]bool{}
	for _, root := range roots {
		// mở rộng ~ thành home
		if strings.HasPrefix(root, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			root = filepath.Join(home, root[2:])
		}
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			name := strings.ToLower(d.Name())
			ext := filepath.Ext(name)
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
				return nil
			}
			if !strings.Contains(name, "emoji") && !strings.Contains(name, "symbola") {
				return nil
			}
			if !seen[path] {
				seen[path] = true
				out = append(out, path)
			}
			return nil
		})
	}
	return out
}

// isOutlineEmojiFace trả về true nếu face render được outline emoji (không phải bitmap).
func isOutlineEmojiFace(f *opentype.Font) bool {
	testFace, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size: 32, DPI: 72, Hinting: font.HintingNone,
	})
	if err != nil {
		return false
	}
	// thử vài emoji phổ biến — tất cả phải có glyph
	for _, r := range []rune{0x1F30D, 0x1F600, 0x2764, 0x1F680} {
		if _, ok := testFace.GlyphAdvance(r); !ok {
			return false
		}
	}
	return true
}

// loadEmojiFont quét mọi font emoji trong hệ thống, chọn font outline đầu tiên.
func loadEmojiFont() {
	var candidates []string

	// 1. env override (ưu tiên tuyệt đối)
	if p := os.Getenv("EMOJI_FONT"); p != "" {
		candidates = append(candidates, p)
	}

	// 2. scan toàn hệ thống
	candidates = append(candidates, scanEmojiFonts()...)

	// 3. fallback path cứng phòng khi scan miss
	candidates = append(candidates,
		"/usr/share/fonts/noto/NotoEmoji-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoEmoji-Regular.ttf",
		"/usr/share/fonts/truetype/symbola/Symbola.ttf",
		"/usr/share/fonts/truetype/ancient-scripts/Symbola_hint.ttf",
	)

	// khử trùng
	seen := map[string]bool{}
	uniq := candidates[:0]
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		uniq = append(uniq, c)
	}
	candidates = uniq

	log.Printf("scanning %d emoji font candidates", len(candidates))

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		if !isOutlineEmojiFace(f) {
			log.Printf("skip (bitmap/no outline): %s", path)
			continue
		}
		emojiParsed = f
		log.Printf("emoji font loaded: %s", path)
		return
	}
	log.Printf("no outline emoji font found — emojis will render as boxes")
}

// emojiFace trả về face emoji ở kích thước cho trước, hoặc nil.
func emojiFace(size float64) font.Face {
	emojiFaceOnce.Do(loadEmojiFont)
	if emojiParsed == nil {
		return nil
	}
	faceMu.Lock()
	defer faceMu.Unlock()
	k := faceKey{"__emoji__", size}
	if f, ok := faceMap[k]; ok {
		return f
	}
	// font emoji thường có sẵn ở size cố định (Noto Color Emoji chỉ 109px)
	// thử size yêu cầu trước, fallback 109 nếu lỗi
	for _, sz := range []float64{size, 109, 96, 64, 48, 32, 24, 16} {
		f, err := opentype.NewFace(emojiParsed, &opentype.FaceOptions{
			Size: sz, DPI: 72, Hinting: font.HintingNone,
		})
		if err == nil {
			faceMap[k] = f
			return f
		}
	}
	return nil
}

// isEmojiRune trả về true nếu rune nằm trong vùng emoji Unicode.
func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1FAFF: // emoji chính
		return true
	case r >= 0x2600 && r <= 0x27BF: // misc symbols + dingbats
		return true
	case r >= 0x1F000 && r <= 0x1F02F: // mahjong
		return true
	case r >= 0x1F0A0 && r <= 0x1F0FF: // playing cards
		return true
	case r >= 0x1F100 && r <= 0x1F1FF: // enclosed alphanumeric + flags
		return true
	case r >= 0x1F200 && r <= 0x1F2FF: // enclosed ideographic
		return true
	case r == 0x200D: // zero width joiner
		return true
	case r == 0xFE0F: // variation selector
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF: // skin tone modifiers
		return true
	}
	return false
}

// fontHasRune kiểm tra face có glyph cho rune không.
func fontHasRune(f font.Face, r rune) bool {
	if f == nil {
		return false
	}
	_, ok := f.GlyphAdvance(r)
	return ok
}

// drawTextWithEmoji vẽ text, tự động fallback sang font emoji khi cần.
func drawTextWithEmoji(dst *image.RGBA, text string, x, baseline fixed.Int26_6, c color.RGBA, face font.Face, emoji font.Face) {
	drawer := &font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: face}
	drawer.Dot = fixed.Point26_6{X: x, Y: baseline}

	emojiDrawer := &font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: emoji}

	for _, r := range text {
		if fontHasRune(face, r) && !isEmojiRune(r) {
			drawer.DrawString(string(r))
			continue
		}
		// thử emoji
		if emoji != nil && fontHasRune(emoji, r) {
			emojiDrawer.Dot = fixed.Point26_6{X: drawer.Dot.X, Y: baseline}
			emojiDrawer.DrawString(string(r))
			adv, _ := emoji.GlyphAdvance(r)
			drawer.Dot.X += adv
			continue
		}
		// không có font nào → vẽ bằng font chính (sẽ ra box)
		drawer.DrawString(string(r))
	}
}

// measureTextWithEmoji đo width tổng của text với fallback emoji.
func measureTextWithEmoji(text string, face font.Face, emoji font.Face) fixed.Int26_6 {
	var total fixed.Int26_6
	for _, r := range text {
		if fontHasRune(face, r) && !isEmojiRune(r) {
			w, _ := face.GlyphAdvance(r)
			total += w
			continue
		}
		if emoji != nil && fontHasRune(emoji, r) {
			w, _ := emoji.GlyphAdvance(r)
			total += w
			continue
		}
		w, _ := face.GlyphAdvance(r)
		total += w
	}
	return total
}

// ensureEmojiFont kiểm tra toàn hệ thống xem có font emoji outline chưa.
// Nếu chưa, tự tải từ mirror CTAN và lưu vào fontsDir.
func ensureEmojiFont() {
// 1. quét TOÀN BỘ path (giống loadEmojiFont)
all := scanEmojiFonts()
all = append(all,
"/usr/share/fonts/noto/NotoEmoji-Regular.ttf",
"/usr/share/fonts/truetype/noto/NotoEmoji-Regular.ttf",
"/usr/share/fonts/truetype/symbola/Symbola.ttf",
"/usr/share/fonts/truetype/ancient-scripts/Symbola_hint.ttf",
)

// khử trùng
seen := map[string]bool{}
uniq := all[:0]
for _, c := range all {
if c == "" || seen[c] {
continue
}
seen[c] = true
uniq = append(uniq, c)
}
all = uniq

// 2. kiểm tra từng path
for _, path := range all {
data, err := os.ReadFile(path)
if err != nil {
continue
}
f, err := opentype.Parse(data)
if err != nil {
continue
}
if isOutlineEmojiFace(f) {
log.Printf("emoji font present: %s", path)
return
}
}

// 3. chưa có → tải
mirrors := []string{
"https://mirrors.ctan.org/fonts/noto-emoji/NotoEmoji-Regular.ttf",
"https://raw.githubusercontent.com/notofonts/notofonts.github.io/main/fonts/NotoEmoji/unhinted/ttf/NotoEmoji-Regular.ttf",
}
dest := filepath.Join(fontsDir, "NotoEmoji-Regular.ttf")

for _, url := range mirrors {
log.Printf("downloading emoji font: %s", url)
if err := downloadFile(url, dest); err != nil {
log.Printf("download failed (%v), trying next mirror", err)
continue
}
data, err := os.ReadFile(dest)
if err != nil {
log.Printf("read downloaded file failed: %v", err)
os.Remove(dest)
continue
}
f, err := opentype.Parse(data)
if err != nil {
log.Printf("parse downloaded font failed: %v", err)
os.Remove(dest)
continue
}
if !isOutlineEmojiFace(f) {
log.Printf("downloaded font is bitmap, not usable — removing")
os.Remove(dest)
continue
}
log.Printf("emoji font installed: %s", dest)
return
}
log.Printf("could not download emoji font — emojis will not render")
}

func downloadFile(url, dest string) error {
client := &http.Client{Timeout: 30 * time.Second}
resp, err := client.Get(url)
if err != nil {
return err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusOK {
return fmt.Errorf("http %d", resp.StatusCode)
}
tmp := dest + ".tmp"
out, err := os.Create(tmp)
if err != nil {
return err
}
if _, err := io.Copy(out, resp.Body); err != nil {
out.Close()
os.Remove(tmp)
return err
}
if err := out.Close(); err != nil {
os.Remove(tmp)
return err
}
return os.Rename(tmp, dest)
}

// ---------- color / font ----------

func parseHex(s string) color.RGBA {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.RGBA{255, 46, 136, 255}
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

func makeBase(text, fontName string, w, h int, c color.RGBA, size float64, bg color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if bg.A > 0 {
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, bg.A
		}
	}
	if text == "" {
		return img
	}
	face := getFace(fontName, size)
	em := emojiFace(size)
	mt := face.Metrics()

	// đo bằng fallback để width chính xác khi có emoji
	m := measureTextWithEmoji(text, face, em)

	// canvas tạm vừa khít glyph — Drawer không thể vẽ tràn
	tw := int(m >> 6)
	th := int((mt.Ascent + mt.Descent) >> 6)
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}

	tmp := image.NewRGBA(image.Rect(0, 0, tw, th))
	if em != nil {
		// nếu có font emoji: chỉ vẽ chữ (emoji sẽ overlay sau effect)
		plainText, emojis := splitTextEmoji(text, face, em)
		if len(emojis) > 0 {
			d := &font.Drawer{Dst: tmp, Src: image.NewUniform(c), Face: face}
			d.Dot = fixed.Point26_6{X: 0, Y: mt.Ascent}
			d.DrawString(plainText)
		} else {
			drawTextWithEmoji(tmp, text, 0, mt.Ascent, c, face, em)
		}
	} else {
		drawTextWithEmoji(tmp, text, 0, mt.Ascent, c, face, em)
	}

	// căn giữa ngang, căn giữa dọc
	dx := (w - tw) / 2
	dy := h/2 - th/2

	for y := 0; y < th; y++ {
		ty := dy + y
		if ty < 0 || ty >= h {
			continue
		}
		for x := 0; x < tw; x++ {
			tx := dx + x
			if tx < 0 || tx >= w {
				continue
			}
			si := tmp.PixOffset(x, y)
			sa := uint32(tmp.Pix[si+3])
			if sa == 0 {
				continue
			}
			di := img.PixOffset(tx, ty)
			if sa == 255 {
				copy(img.Pix[di:di+4], tmp.Pix[si:si+4])
				continue
			}
			da := uint32(img.Pix[di+3])
			inv := 255 - sa
			img.Pix[di+0] = uint8(uint32(tmp.Pix[si+0]) + uint32(img.Pix[di+0])*inv/255)
			img.Pix[di+1] = uint8(uint32(tmp.Pix[si+1]) + uint32(img.Pix[di+1])*inv/255)
			img.Pix[di+2] = uint8(uint32(tmp.Pix[si+2]) + uint32(img.Pix[di+2])*inv/255)
			img.Pix[di+3] = uint8(sa + da*inv/255)
		}
	}
	return img
}

// ---------- primitives (premultiplied RGBA) ----------

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

// chromaShift tách kênh R và B theo trục ngang.
func chromaShift(src *image.RGBA, dr, db int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(b)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := src.PixOffset(x, y)
			sri := src.PixOffset(clampInt(x-dr, 0, w-1), y)
			sbi := src.PixOffset(clampInt(x-db, 0, w-1), y)
			di := dst.PixOffset(x, y)
			dst.Pix[di+0] = src.Pix[sri+0]
			dst.Pix[di+1] = src.Pix[si+1]
			dst.Pix[di+2] = src.Pix[sbi+2]
			dst.Pix[di+3] = src.Pix[si+3]
		}
	}
	return dst
}

// addNoise rắc hạt xám bán trong suốt (alpha-composite, premultiplied).
func addNoise(src *image.RGBA, density, intensity float64, rng *rand.Rand) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := cloneRGBA(src)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if rng.Float64() >= density {
				continue
			}
			v := uint8(rng.Intn(256))
			na := uint32(intensity * 255 * (0.5 + 0.5*rng.Float64()))
			if na > 255 {
				na = 255
			}
			if na == 0 {
				continue
			}
			vPrem := uint32(v) * na / 255
			di := dst.PixOffset(x, y)
			da := uint32(dst.Pix[di+3])
			for c := 0; c < 3; c++ {
				dc := uint32(dst.Pix[di+c])
				dst.Pix[di+c] = uint8(vPrem + dc*(255-na)/255)
			}
			dst.Pix[di+3] = uint8(na + da*(255-na)/255)
		}
	}
	return dst
}

// addScanlines phủ dòng đen cách quãng.
func addScanlines(src *image.RGBA, alpha, spacing int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := cloneRGBA(src)
	na := uint32(alpha)
	for y := 0; y < h; y += spacing {
		for x := 0; x < w; x++ {
			di := dst.PixOffset(x, y)
			da := uint32(dst.Pix[di+3])
			for c := 0; c < 3; c++ {
				dc := uint32(dst.Pix[di+c])
				dst.Pix[di+c] = uint8(dc * (255 - na) / 255)
			}
			dst.Pix[di+3] = uint8(na + da*(255-na)/255)
		}
	}
	return dst
}

// bandShift dịch ngang một dải hàng (cho tracking bar).
func bandShift(src *image.RGBA, y0, bh, dx int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := cloneRGBA(src)
	for row := 0; row < bh; row++ {
		y := y0 + row
		if y < 0 || y >= h {
			continue
		}
		for x := 0; x < w; x++ {
			sx := x - dx
			di := dst.PixOffset(x, y)
			if sx < 0 || sx >= w {
				dst.Pix[di+0], dst.Pix[di+1], dst.Pix[di+2], dst.Pix[di+3] = 0, 0, 0, 0
				continue
			}
			si := src.PixOffset(sx, y)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// noiseBand thay một dải hàng bằng nhiễu trắng.
func noiseBand(src *image.RGBA, y0, bh int, density, intensity float64, rng *rand.Rand) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := cloneRGBA(src)
	for row := 0; row < bh; row++ {
		y := y0 + row
		if y < 0 || y >= h {
			continue
		}
		for x := 0; x < w; x++ {
			if rng.Float64() >= density {
				continue
			}
			v := uint8(rng.Intn(256))
			na := uint32(intensity * 255)
			if na == 0 {
				continue
			}
			vPrem := uint32(v) * na / 255
			di := dst.PixOffset(x, y)
			dst.Pix[di+0] = uint8(vPrem)
			dst.Pix[di+1] = uint8(vPrem)
			dst.Pix[di+2] = uint8(vPrem)
			dst.Pix[di+3] = uint8(na)
		}
	}
	return dst
}

// ---------- effects ----------

func fxVHS(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	out := make([]*image.RGBA, 0, n)
	h := base.Bounds().Dy()
	for i := 0; i < n; i++ {
		f := chromaShift(base, rng.Intn(5)-2, rng.Intn(5)-2)
		f = addScanlines(f, 55, 2)
		f = addNoise(f, 0.03, 0.25, rng)
		bh := rng.Intn(5) + 2
		y := h - bh - 2
		f = bandShift(f, y, bh, rng.Intn(7)-3)
		f = noiseBand(f, y, bh, 0.4, 0.7, rng)
		out = append(out, f)
	}
	return out
}

func fxStatic(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		f := addNoise(base, 0.2+0.3*rng.Float64(), 0.6, rng)
		f = addScanlines(f, 40, 3)
		out = append(out, f)
	}
	return out
}

func fxDropout(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	out := make([]*image.RGBA, 0, n)
	h := base.Bounds().Dy()
	for i := 0; i < n; i++ {
		f := cloneRGBA(base)
		for k := 0; k < rng.Intn(3)+1; k++ {
			y := rng.Intn(h - 4)
			bh := rng.Intn(4) + 2
			if rng.Float64() < 0.5 {
				f = noiseBand(f, y, bh, 0.8, 1.0, rng)
			} else {
				f = bandShift(f, y, bh, rng.Intn(13)-6)
			}
		}
		f = addScanlines(f, 45, 2)
		out = append(out, f)
	}
	return out
}

func fxRGB(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	shiftSet := []int{-5, -4, -3, 3, 4, 5}
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		dr := shiftSet[rng.Intn(len(shiftSet))]
		db := -dr + rng.Intn(3) - 1
		f := chromaShift(base, dr, db)
		f = addScanlines(f, 30, 2)
		out = append(out, f)
	}
	return out
}

func fxFlicker(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	vals := []float64{0.3, 0.5, 0.7, 0.85, 1.0, 1.0, 1.0}
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		br := vals[rng.Intn(len(vals))]
		f := cloneRGBA(base)
		for j := 0; j < len(f.Pix); j++ {
			f.Pix[j] = uint8(float64(f.Pix[j]) * br)
		}
		f = addScanlines(f, 20, 3)
		out = append(out, f)
	}
	return out
}

func fxGhost(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]*image.RGBA, 0, n)
	var trail []*image.RGBA
	for i := 0; i < n; i++ {
		dx := rng.Intn(5) - 2
		dy := rng.Intn(3) - 1
		f := image.NewRGBA(b)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				sx, sy := x-dx, y-dy
				if sx < 0 || sx >= w || sy < 0 || sy >= h {
					continue
				}
				si := base.PixOffset(sx, sy)
				di := f.PixOffset(x, y)
				copy(f.Pix[di:di+4], base.Pix[si:si+4])
			}
		}
		trail = append(trail, f)
		if len(trail) > 4 {
			trail = trail[1:]
		}
		combined := cloneRGBA(f)
		for j := 0; j < len(trail)-1; j++ {
			frac := float64(j+1) / float64(len(trail))
			t := trail[j]
			for p := 0; p < len(t.Pix); p += 4 {
				ta := uint32(t.Pix[p+3])
				if ta == 0 {
					continue
				}
				scaledA := uint32(float64(ta) * frac * 0.6)
				ca := uint32(combined.Pix[p+3])
				outA := scaledA + ca*(255-scaledA)/255
				if outA == 0 {
					continue
				}
				for c := 0; c < 3; c++ {
					tc := uint32(t.Pix[p+c])
					srcPre := tc * scaledA / ta
					cc := uint32(combined.Pix[p+c])
					combined.Pix[p+c] = uint8(srcPre + cc*(255-scaledA)/255)
				}
				combined.Pix[p+3] = uint8(outA)
			}
		}
		out = append(out, combined)
	}
	return out
}

func fxRoll(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		offset := (i * 2 * h / n) % h
		f := image.NewRGBA(b)
		for y := 0; y < h; y++ {
			sy1 := y - (offset - h)
			sy2 := y - offset
			for x := 0; x < w; x++ {
				var si int
				switch {
				case sy1 >= 0 && sy1 < h:
					si = base.PixOffset(x, sy1)
				case sy2 >= 0 && sy2 < h:
					si = base.PixOffset(x, sy2)
				default:
					continue
				}
				di := f.PixOffset(x, y)
				copy(f.Pix[di:di+4], base.Pix[si:si+4])
			}
		}
		f = addScanlines(f, 50, 2)
		f = addNoise(f, 0.05, 0.3, rng)
		out = append(out, f)
	}
	return out
}

func fxGlitch(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	h := base.Bounds().Dy()
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		dr := rng.Intn(7) - 3
		f := chromaShift(base, dr, -dr)
		for k := 0; k < rng.Intn(2); k++ {
			y := rng.Intn(h - 4)
			bh := rng.Intn(3) + 2
			f = bandShift(f, y, bh, rng.Intn(11)-5)
		}
		f = addNoise(f, 0.08, 0.4, rng)
		f = addScanlines(f, 40, 2)
		out = append(out, f)
	}
	return out
}

// typeHoldSec: số giây con trỏ nhấp nháy sau khi gõ xong, trước khi GIF lặp lại.
const typeHoldSec = 2.0

// typeHoldFrames: số frame của pha giữ trỏ (độc lập với pha gõ).
func typeHoldFrames(fps int) int {
	return int(typeHoldSec * float64(fps))
}

// typeTotalFrames: tổng frame = pha gõ (theo dur) + pha giữ trỏ.
func typeTotalFrames(fps int, dur float64) int {
	hold := typeHoldFrames(fps)
	typing := clampInt(int(float64(fps)*dur), 1, maxFrames-hold)
	return typing + hold
}

// fxType ghép 2 pha riêng biệt: gõ chữ, rồi giữ chữ đầy đủ cho trỏ nhấp nháy.
// n là tổng frame (typeTotalFrames); pha giữ luôn đúng typeHoldFrames(fps).
func fxType(text, fontName string, w, h int, c color.RGBA, size float64, bg color.RGBA, n, fps int, rng *rand.Rand) []*image.RGBA {
face := getFace(fontName, size)
em := emojiFace(size)
mt := face.Metrics()
cw := int(size) / 8
if cw < 3 {
cw = 3
}
if cw > 10 {
cw = 10
}
// baseline giữ ở đơn vị 26.6 để chính xác, đổi về int ngay khi dùng
baseline26 := fixed.I(h)/2 + (mt.Ascent-mt.Descent)/2
cursorTop := int((baseline26 - mt.Ascent) >> 6)
cursorBot := int((baseline26 + mt.Descent) >> 6)
// baseline cho overlayEmoji (cùng công thức)
emojiBaseline := baseline26

// vẽ 1 frame: chữ đã gõ + emoji overlay + (tuỳ chọn) con trỏ
draw := func(partial string, cursorOn bool) *image.RGBA {
f := makeBase(partial, fontName, w, h, c, size, bg)
// overlay emoji lên frame (makeBase chỉ vẽ chữ)
if em != nil {
overlayEmoji(f, partial, face, em, c, fixed.I(w)/2, emojiBaseline)
}
if cursorOn {
// đo width bao gồm cả emoji để đặt cursor đúng
meas := measureTextWithEmoji(partial, face, em)
x := int(((fixed.I(w)-meas)/2 + meas + fixed.I(4)) >> 6)
for yy := cursorTop; yy < cursorBot; yy++ {
for xx := x; xx < x+cw; xx++ {
if xx >= 0 && xx < w && yy >= 0 && yy < h {
f.SetRGBA(xx, yy, c)
}
}
}
}
return addNoise(f, 0.05, 0.3, rng)
}

holdFrames := typeHoldFrames(fps)
typeFrames := n - holdFrames
if typeFrames < 1 {
typeFrames = 1
}
out := make([]*image.RGBA, 0, typeFrames+holdFrames)

// pha 1: gõ chữ — cắt theo RUNE
runes := []rune(text)
totalRunes := len(runes)
for i := 0; i < typeFrames; i++ {
chars := totalRunes * (i + 1) / typeFrames
if chars < 1 && totalRunes > 0 {
chars = 1
}
if chars > totalRunes {
chars = totalRunes
}
out = append(out, draw(string(runes[:chars]), (i/2)%2 == 0))
}

// pha 2: giữ chữ đầy đủ, trỏ nhấp nháy
half := fps / 3
if half < 1 {
half = 1
}
for i := 0; i < holdFrames; i++ {
out = append(out, draw(text, (i/half)%2 == 0))
}
return out
}

// ---------- additional effects ----------

// scaleBrightness nhân toàn bộ pixel với một hệ số (kẹp ở 255).
func scaleBrightness(src *image.RGBA, f float64) *image.RGBA {
	dst := cloneRGBA(src)
	for j := 0; j < len(dst.Pix); j++ {
		dst.Pix[j] = uint8(float64(dst.Pix[j]) * f)
	}
	return dst
}

// cropShift dịch toàn bộ ảnh theo (dx,dy), phần ngoài biên để trong suốt.
func cropShift(src *image.RGBA, dx, dy int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(b)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx, sy := x-dx, y-dy
			if sx < 0 || sx >= w || sy < 0 || sy >= h {
				continue
			}
			si := src.PixOffset(sx, sy)
			di := dst.PixOffset(x, y)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// fxScan: dải sáng quét dọc màn hình.
func fxScan(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	b := base.Bounds()
	h := b.Dy()
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		f := scaleBrightness(base, 0.85)
		y := (i * h / n) % h
		bandH := h / 12
		if bandH < 2 {
			bandH = 2
		}
		for dy := 0; dy < bandH; dy++ {
			yy := y + dy
			if yy < 0 || yy >= h {
				continue
			}
			alpha := 1.0 - float64(dy)/float64(bandH)
			for x := 0; x < b.Dx(); x++ {
				di := f.PixOffset(x, yy)
				r := uint32(f.Pix[di+0])
				g := uint32(f.Pix[di+1])
				bl := uint32(f.Pix[di+2])
				a := uint32(f.Pix[di+3])
				f.Pix[di+0] = uint8(math.Min(255, float64(r)+120*alpha))
				f.Pix[di+1] = uint8(math.Min(255, float64(g)+120*alpha))
				f.Pix[di+2] = uint8(math.Min(255, float64(bl)+120*alpha))
				f.Pix[di+3] = uint8(math.Min(255, float64(a)+80*alpha))
			}
		}
		f = addScanlines(f, 30, 2)
		out = append(out, f)
	}
	return out
}

// fxWave: dịch ngang từng dòng theo sóng sin.
func fxWave(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]*image.RGBA, 0, n)
	amp := h / 12
	if amp < 2 {
		amp = 2
	}
	for i := 0; i < n; i++ {
		f := image.NewRGBA(b)
		phase := float64(i) / float64(n) * 2 * math.Pi
		for y := 0; y < h; y++ {
			dy := int(float64(amp) * math.Sin(phase+float64(y)*0.2))
			for x := 0; x < w; x++ {
				sx := x - dy
				if sx < 0 || sx >= w {
					continue
				}
				si := base.PixOffset(sx, y)
				di := f.PixOffset(x, y)
				copy(f.Pix[di:di+4], base.Pix[si:si+4])
			}
		}
		f = addScanlines(f, 25, 2)
		f = addNoise(f, 0.03, 0.2, rng)
		out = append(out, f)
	}
	return out
}

// fxSlice: cắt thành từng dải ngang, xáo trộn vị trí + lệch ngang.
func fxSlice(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]*image.RGBA, 0, n)
	sliceH := h / 8
	if sliceH < 2 {
		sliceH = 2
	}
	for i := 0; i < n; i++ {
		f := image.NewRGBA(b)
		for y := 0; y < h; y += sliceH {
			// bốc ngẫu nhiên một dải nguồn
			srcY := rng.Intn(h - sliceH)
			dx := rng.Intn(7) - 3
			for dy := 0; dy < sliceH && y+dy < h; dy++ {
				for x := 0; x < w; x++ {
					sx := x - dx
					if sx < 0 || sx >= w {
						continue
					}
					si := base.PixOffset(sx, srcY+dy)
					di := f.PixOffset(x, y+dy)
					copy(f.Pix[di:di+4], base.Pix[si:si+4])
				}
			}
		}
		f = addScanlines(f, 35, 2)
		out = append(out, f)
	}
	return out
}

// fxInvert: đảo màu luân phiên giữa các frame (nhấp nháy đảo).
func fxInvert(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		f := cloneRGBA(base)
		if i%3 == 0 {
			for j := 0; j < len(f.Pix); j += 4 {
				f.Pix[j+0] = 255 - f.Pix[j+0]
				f.Pix[j+1] = 255 - f.Pix[j+1]
				f.Pix[j+2] = 255 - f.Pix[j+2]
			}
		}
		f = addScanlines(f, 30, 2)
		f = addNoise(f, 0.04, 0.25, rng)
		out = append(out, f)
	}
	return out
}

// fxPixelate: giảm độ phân giải theo block, block size thay đổi theo frame.
func fxPixelate(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]*image.RGBA, 0, n)
	// chu kỳ: nhỏ → lớn → nhỏ
	sizes := []int{2, 4, 6, 10, 14, 10, 6, 4}
	for i := 0; i < n; i++ {
		block := sizes[i%len(sizes)]
		if block < 1 {
			block = 1
		}
		f := image.NewRGBA(b)
		for by := 0; by < h; by += block {
			for bx := 0; bx < w; bx += block {
				// màu trung bình của block
				var sr, sg, sb, sa, cnt uint32
				for y := by; y < by+block && y < h; y++ {
					for x := bx; x < bx+block && x < w; x++ {
						si := base.PixOffset(x, y)
						sr += uint32(base.Pix[si+0])
						sg += uint32(base.Pix[si+1])
						sb += uint32(base.Pix[si+2])
						sa += uint32(base.Pix[si+3])
						cnt++
					}
				}
				if cnt == 0 {
					continue
				}
				r := uint8(sr / cnt)
				g := uint8(sg / cnt)
				bl := uint8(sb / cnt)
				a := uint8(sa / cnt)
				for y := by; y < by+block && y < h; y++ {
					for x := bx; x < bx+block && x < w; x++ {
						di := f.PixOffset(x, y)
						f.Pix[di+0], f.Pix[di+1], f.Pix[di+2], f.Pix[di+3] = r, g, bl, a
					}
				}
			}
		}
		f = addScanlines(f, 20, 2)
		out = append(out, f)
	}
	return out
}

// fxBloom: phóng to + mờ + chồng lên bản gốc (bloom/glow).
func fxBloom(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		// bản gốc mờ: giảm alpha, rồi phóng to + lệch nhẹ
		glow := scaleBrightness(base, 0.5)
		glow = cropShift(glow, rng.Intn(3)-1, rng.Intn(3)-1)
		f := cloneRGBA(base)
		// cộng dồn glow (alpha-composite)
		for j := 0; j < len(f.Pix); j += 4 {
			a := uint32(glow.Pix[j+3])
			if a == 0 {
				continue
			}
			inv := 255 - a
			f.Pix[j+0] = uint8(uint32(glow.Pix[j+0]) + uint32(f.Pix[j+0])*inv/255)
			f.Pix[j+1] = uint8(uint32(glow.Pix[j+1]) + uint32(f.Pix[j+1])*inv/255)
			f.Pix[j+2] = uint8(uint32(glow.Pix[j+2]) + uint32(f.Pix[j+2])*inv/255)
			f.Pix[j+3] = uint8(a + uint32(f.Pix[j+3])*inv/255)
		}
		f = addNoise(f, 0.04, 0.2, rng)
		out = append(out, f)
	}
	return out
}

// fxShake: lắc ngẫu nhiên vị trí ảnh (rung lắc màn hình).
func fxShake(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		dx := rng.Intn(9) - 4
		dy := rng.Intn(5) - 2
		f := cropShift(base, dx, dy)
		f = addScanlines(f, 30, 2)
		f = addNoise(f, 0.03, 0.2, rng)
		out = append(out, f)
	}
	return out
}

// fxZoom: phóng to/thu nhỏ luân phiên (dolly zoom).
func fxZoom(base *image.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]*image.RGBA, 0, n)
	for i := 0; i < n; i++ {
		// factor 1.0 → 1.15 → 1.0
		t := float64(i) / float64(n)
		zoom := 1.0 + 0.15*math.Abs(math.Sin(t*math.Pi*2))
		cx := float64(w) / 2
		cy := float64(h) / 2
		f := image.NewRGBA(b)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				sx := int(cx + (float64(x)-cx)/zoom)
				sy := int(cy + (float64(y)-cy)/zoom)
				if sx < 0 || sx >= w || sy < 0 || sy >= h {
					continue
				}
				si := base.PixOffset(sx, sy)
				di := f.PixOffset(x, y)
				copy(f.Pix[di:di+4], base.Pix[si:si+4])
			}
		}
		f = addScanlines(f, 25, 2)
		out = append(out, f)
	}
	return out
}

// splitTextEmoji chia text thành chữ + vị trí emoji.
// Trả về: string chỉ chữ (emoji thay bằng space) và slice emoji (rune, offset).
type emojiPos struct {
	r     rune
	xOff  fixed.Int26_6
	width fixed.Int26_6
}

func splitTextEmoji(text string, face, emoji font.Face) (string, []emojiPos) {
	var plain strings.Builder
	var emojis []emojiPos
	var total fixed.Int26_6
	for _, r := range text {
		isEm := isEmojiRune(r) && emoji != nil && fontHasRune(emoji, r)
		if isEm {
			w, _ := emoji.GlyphAdvance(r)
			emojis = append(emojis, emojiPos{r, total, w})
			plain.WriteRune(' ')
			total += w
			continue
		}
		w, _ := face.GlyphAdvance(r)
		plain.WriteRune(r)
		total += w
	}
	return plain.String(), emojis
}

// overlayEmoji vẽ emoji lên frame (sau effect) tại đúng vị trí.
func overlayEmoji(dst *image.RGBA, text string, face, emoji font.Face, c color.RGBA, x0, baseline fixed.Int26_6) {
	if emoji == nil {
		return
	}
	_, emojis := splitTextEmoji(text, face, emoji)
	if len(emojis) == 0 {
		return
	}
	var totalW fixed.Int26_6
	for _, r := range text {
		if isEmojiRune(r) && emoji != nil && fontHasRune(emoji, r) {
			w, _ := emoji.GlyphAdvance(r)
			totalW += w
		} else {
			w, _ := face.GlyphAdvance(r)
			totalW += w
		}
	}
	start := x0 - totalW/2

	emojiDrawer := &font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: emoji}
	for _, e := range emojis {
		emojiDrawer.Dot = fixed.Point26_6{
			X: start + e.xOff,
			Y: baseline,
		}
		emojiDrawer.DrawString(string(e.r))
	}
}

// ---------- GIF encode ----------

// buildPalette trả về palette 256 màu, index 0 trong suốt + 216 web-safe.
func buildPalette() color.Palette {
	pal := make(color.Palette, 256)
	pal[0] = color.RGBA{0, 0, 0, 0}
	for i := 1; i < 256; i++ {
		n := i - 1
		pal[i] = color.RGBA{
			uint8((n / 36) * 51),
			uint8(((n / 6) % 6) * 51),
			uint8((n % 6) * 51),
			255,
		}
	}
	return pal
}

func nearest6(v int) int {
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return (v + 25) / 51
}

func encodeGIF(frames []*image.RGBA, fps int) ([]byte, error) {
	pal := buildPalette()
	delay := 100 / fps
	if delay < 2 {
		delay = 2
	}
	g := &gif.GIF{LoopCount: 0, BackgroundIndex: 0}
	for _, f := range frames {
		b := f.Bounds()
		w, h := b.Dx(), b.Dy()
		p := image.NewPaletted(b, pal)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				si := f.PixOffset(x, y)
				di := p.PixOffset(x, y)
				a := f.Pix[si+3]
				if a < 32 {
					p.Pix[di] = 0
					continue
				}
				// un-premultiply về straight RGB
				sr := uint32(f.Pix[si+0]) * 255 / uint32(a)
				sg := uint32(f.Pix[si+1]) * 255 / uint32(a)
				sb := uint32(f.Pix[si+2]) * 255 / uint32(a)
				p.Pix[di] = uint8(1 + nearest6(int(sr))*36 + nearest6(int(sg))*6 + nearest6(int(sb)))
			}
		}
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, delay)
		g.Disposal = append(g.Disposal, gif.DisposalBackground)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------- dispatch ----------

func renderFrames(text, effect, fontName string, w, h int, c1, bgStr string, size float64, fps, n int, seed int64) ([]*image.RGBA, error) {
	rng := rand.New(rand.NewSource(seed))
	c := parseHex(c1)
	var bg color.RGBA
	if bgStr == "" || bgStr == "transparent" {
		bg = color.RGBA{0, 0, 0, 0}
	} else {
		bg = parseHex(bgStr)
	}
	if effect == "type" {
		// fxType vẽ emoji per-frame trong makeBase, không overlay
		return fxType(text, fontName, w, h, c, size, bg, n, fps, rng), nil
	}

	// tách text: chữ vẽ vào base, emoji overlay sau effect
	face := getFace(fontName, size)
	em := emojiFace(size)
	_, emojis := splitTextEmoji(text, face, em)
	hasEmoji := em != nil && len(emojis) > 0

	var base *image.RGBA
	if hasEmoji {
		// base chỉ có chữ, emoji thay bằng space placeholder
		plainText, _ := splitTextEmoji(text, face, em)
		base = makeBase(plainText, fontName, w, h, c, size, bg)
	} else {
		base = makeBase(text, fontName, w, h, c, size, bg)
	}

	var frames []*image.RGBA
	switch effect {
	case "vhs":
		frames = fxVHS(base, n, rng)
	case "static":
		frames = fxStatic(base, n, rng)
	case "dropout":
		frames = fxDropout(base, n, rng)
	case "rgb":
		frames = fxRGB(base, n, rng)
	case "flicker":
		frames = fxFlicker(base, n, rng)
	case "ghost":
		frames = fxGhost(base, n, rng)
	case "roll":
		frames = fxRoll(base, n, rng)
	case "glitch":
		frames = fxGlitch(base, n, rng)
	case "scan":
		frames = fxScan(base, n, rng)
	case "wave":
		frames = fxWave(base, n, rng)
	case "slice":
		frames = fxSlice(base, n, rng)
	case "invert":
		frames = fxInvert(base, n, rng)
	case "pixelate":
		frames = fxPixelate(base, n, rng)
	case "bloom":
		frames = fxBloom(base, n, rng)
	case "shake":
		frames = fxShake(base, n, rng)
	case "zoom":
		frames = fxZoom(base, n, rng)
	default:
		return nil, fmt.Errorf("unknown effect: %s", effect)
	}

	// overlay emoji sắc nét lên từng frame (sau effect)
	if hasEmoji {
		mt := face.Metrics()
		baseline := fixed.I(h)/2 + (mt.Ascent-mt.Descent)/2
		for _, f := range frames {
			overlayEmoji(f, text, face, em, c, fixed.I(w)/2, baseline)
		}
	}

	return frames, nil
}

// ---------- HTTP ----------

func atoi(s string, d int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return d
}

func atof(s string, d float64) float64 {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return d
}

func cacheKey(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func handleRender(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	text := q.Get("text")
	if text == "" {
		text = "content has been deleted"
	}
	fontName := resolveFont(q.Get("font"))
	effect := q.Get("effect")
	if effect == "" {
		effect = "vhs"
	}
	width := clampInt(atoi(q.Get("w"), 520), 16, maxW)
	height := clampInt(atoi(q.Get("h"), 60), 16, maxH)
	c1 := q.Get("c1")
	if c1 == "" {
		c1 = "ff2e88"
	}
	bg := q.Get("bg")
	if bg == "" {
		bg = "transparent"
	}
	size := atof(q.Get("size"), math.Max(12, float64(height-20)))
	fps := clampInt(atoi(q.Get("fps"), 15), 2, 30)
	dur := math.Min(10, atof(q.Get("dur"), 2.5))
	n := clampInt(int(float64(fps)*dur), 1, maxFrames)
	if effect == "type" {
		n = typeTotalFrames(fps, dur) // dur = thời gian gõ, cộng riêng pha giữ trỏ
	}

	var seed int64
	if s := q.Get("seed"); s != "" {
		seed, _ = strconv.ParseInt(s, 10, 64)
	} else {
		seed = rand.Int63()
	}

	key := cacheKey(fmt.Sprintf("%s|%s|%s|%d|%d|%s|%s|%.2f|%d|%d|%d",
		text, effect, fontName, width, height, c1, bg, size, fps, n, seed))
	cf := filepath.Join(cacheDir, key+".gif")

	var data []byte
	if b, err := os.ReadFile(cf); err == nil {
		data = b
	} else {
		frames, err := renderFrames(text, effect, fontName, width, height, c1, bg, size, fps, n, seed)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		data, err = encodeGIF(frames, fps)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = os.WriteFile(cf, data, 0644)
	}

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(data)
}

func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

const publicBase = "https://gor-mez8.onrender.com"

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	base := publicBase
	effects := []string{"vhs", "static", "dropout", "rgb", "flicker", "ghost", "roll", "glitch", "type", "scan", "wave", "slice", "invert", "pixelate", "bloom", "shake", "zoom"}

	var cards strings.Builder
	for _, e := range effects {
		cards.WriteString(fmt.Sprintf(
			`<div class="card"><h2>%s</h2><img src="/render.gif?text=content+has+been+deleted&effect=%s&c1=ff2e88&fps=12&dur=2&size=28&seed=7"></div>`,
			e, e))
	}

	var fonts strings.Builder
	for _, f := range listFonts() {
		fonts.WriteString(fmt.Sprintf(`<code>%s</code> `, f))
	}

	example := base + "/render.gif?text=I+miss&effect=glitch&font=bolditalic&w=700&h=130&size=76&c1=d6d3d1&bg=0b0b0d&fps=15&dur=3&seed=7"
	exampleEsc := strings.ReplaceAll(example, "&", "&amp;")

	docs := `<section class="docs">
<h1>docs</h1>
<p>Generate glitchy text GIFs from a single URL. Drop it straight into a README or an <code>&lt;img&gt;</code> tag.</p>

<h3>endpoint</h3>
<pre>GET ` + base + `/render.gif?text=...&amp;effect=...   (recommended — works with Discord)</pre>
<pre>GET ` + base + `/render?text=...&amp;effect=...       (alias, same output)</pre>
<pre>GET ` + base + `/fonts   (list available fonts)</pre>

<h3>parameters</h3>
<table>
<tr><th>name</th><th>default</th><th>description</th></tr>
<tr><td>text</td><td>content has been deleted</td><td>text content; use <code>+</code> or <code>%20</code> for spaces</td></tr>
<tr><td>effect</td><td>vhs</td><td>vhs, static, dropout, rgb, flicker, ghost, roll, glitch, type, scan, wave, slice, invert, pixelate, bloom, shake, zoom</td></tr>
<tr><td>font</td><td>monobold</td><td>see the font list below</td></tr>
<tr><td>w</td><td>520</td><td>width, 16 to ` + strconv.Itoa(maxW) + `</td></tr>
<tr><td>h</td><td>60</td><td>height, 16 to ` + strconv.Itoa(maxH) + `</td></tr>
<tr><td>size</td><td>h - 20</td><td>font size in px (minimum 12)</td></tr>
<tr><td>c1</td><td>ff2e88</td><td>text color, hex without <code>#</code> (or <code>%23</code>)</td></tr>
<tr><td>bg</td><td>transparent</td><td>background hex color or <code>transparent</code></td></tr>
<tr><td>fps</td><td>15</td><td>frames per second, 2 to 30</td></tr>
<tr><td>dur</td><td>2.5</td><td>duration in seconds, max 10 (type: typing time, plus 2s of cursor blinking) (max ` + strconv.Itoa(maxFrames) + ` frames)</td></tr>
<tr><td>seed</td><td>random</td><td>freezes the noise pattern. Always set it, e.g. <code>seed=7</code>, so the cache is reused</td></tr>
</table>

<h3>fonts</h3>
<p>` + fonts.String() + `</p>
<p class="muted">Custom fonts: drop a .ttf or .otf file into the <code>` + fontsDir + `/</code> directory, use a lowercase filename, then call <code>font=filename</code>. Unknown fonts fall back to <code>monobold</code>.</p>

<h3>example</h3>
<pre>` + exampleEsc + `</pre>
<pre>&lt;img src="` + exampleEsc + `" alt="I miss" /&gt;</pre>
<p><img src="` + strings.TrimPrefix(example, base) + `"></p>

<h3>notes</h3>
<ul>
<li>Always set <code>seed</code> so the server doesn't mint a new GIF and write a new cache entry on every load.</li>
<li>The first render is slower; hit the URL once before embedding it on GitHub.</li>
<li>On Render's free tier, the server sleeps after ~15 minutes with no traffic.</li>
<li><b>Emoji:</b> emoji in <code>text</code> render via the system emoji font when available (Noto Color Emoji, Apple Color Emoji, Segoe UI Emoji). In Docker, install <code>fonts-noto-color-emoji</code>.</li>
<li><b>Discord:</b> use the <code>/render.gif</code> URL (with the <code>.gif</code> extension). Discord sometimes treats extension-less image URLs as static images and only shows the first frame.</li>
</ul>
</section>
<h1>effects</h1>`

	page := `<!DOCTYPE html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Glitch Effects</title>
<meta name="description" content="Generate glitchy text GIFs from a single URL. Works with Discord, GitHub, Markdown.">
<meta property="og:type" content="website">
<meta property="og:site_name" content="glitch">
<meta property="og:title" content="Glitch Effects">
<meta property="og:description" content="Generate glitchy text GIFs from a single URL. Works with Discord, GitHub, Markdown.">
<meta property="og:url" content="https://gor-mez8.onrender.com/">
<meta property="og:image" content="https://gor-mez8.onrender.com/render.gif?text=GLITCH+EFFECTS&effect=glitch&font=monobold&w=700&h=120&size=72&c1=ff2e88&bg=transparent&fps=15&dur=3&seed=7">
<meta property="og:image:type" content="image/gif">
<meta property="og:image:width" content="700">
<meta property="og:image:height" content="120">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="Glitch Effects">
<meta name="twitter:description" content="Generate glitchy text GIFs from a single URL. Works with Discord, GitHub, Markdown.">
<meta name="twitter:image" content="https://gor-mez8.onrender.com/render.gif?text=GLITCH+EFFECTS&effect=glitch&font=monobold&w=700&h=120&size=72&c1=ff2e88&bg=transparent&fps=15&dur=3&seed=7">
<link rel="icon" type="image/gif" href="/render.gif?text=G&effect=glitch&w=32&h=32&size=24&c1=ff2e88&fps=8&dur=1&seed=7">
<link rel="apple-touch-icon" href="/render.gif?text=G&effect=glitch&w=180&h=180&size=80&c1=ff2e88&fps=8&dur=1&seed=7">
<style>
body{background:#0b0b0f;color:#d6d3d1;font-family:ui-monospace,monospace;padding:2rem;max-width:900px;margin:0 auto;line-height:1.5}
h1{font-size:.8rem;color:#71717a;letter-spacing:.2em;text-transform:uppercase;margin-top:2rem}
h3{font-size:.8rem;color:#b91c1c;letter-spacing:.15em;text-transform:uppercase;margin:1.6rem 0 .5rem}
code,pre{background:#111;border:1px solid #222;border-radius:4px;color:#a8a29e}
code{padding:.05rem .35rem}
pre{padding:.6rem .8rem;overflow-x:auto;white-space:pre-wrap;word-break:break-all}
table{border-collapse:collapse;width:100%;font-size:.85rem}
th,td{border:1px solid #222;padding:.4rem .6rem;text-align:left;vertical-align:top}
th{color:#71717a;font-weight:normal}
.muted{color:#71717a;font-size:.85rem}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:1rem}
.card{background:#111;border:1px solid #222;padding:1rem;border-radius:8px;text-align:center}
.card h2{font-size:.7rem;color:#666;margin:0 0 .75rem;letter-spacing:.15em}
img{max-width:100%}
</style></head><body>` + docs + `<div class="grid">` + cards.String() + `</div></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

func handleFonts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(strings.Join(listFonts(), "\n") + "\n"))
}

func main() {
	_ = os.MkdirAll(fontsDir, 0755)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Fatal(err)
	}
	// tự tải font emoji nếu chưa có ở đâu
	ensureEmojiFont()
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/render", handleRender)
	http.HandleFunc("/render.gif", handleRender)
	http.HandleFunc("/fonts", handleFonts)
	addr := ":" + port
	log.Printf("listening on %s (cache=%s)", addr, cacheDir)
	log.Fatal(http.ListenAndServe(addr, nil))
}

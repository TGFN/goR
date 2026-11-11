package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
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
	d := &font.Drawer{Src: image.NewUniform(c), Face: face}
	m := d.MeasureString(text)
	mt := face.Metrics()

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
	d.Dst = tmp
	d.Dot = fixed.Point26_6{X: 0, Y: mt.Ascent}
	d.DrawString(text)

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

func fxType(text, fontName string, w, h int, c color.RGBA, size float64, bg color.RGBA, n int, rng *rand.Rand) []*image.RGBA {
	out := make([]*image.RGBA, 0, n)
	face := getFace(fontName, size)
	mt := face.Metrics()
	cw := int(size) / 3
	if cw < 4 {
		cw = 4
	}

	// baseline giữ ở đơn vị 26.6 để chính xác, đổi về int ngay khi dùng
	baseline26 := fixed.I(h)/2 + (mt.Ascent-mt.Descent)/2
	cursorTop := int((baseline26 - mt.Ascent) >> 6)
	cursorBot := int((baseline26 + mt.Descent) >> 6)

	for i := 0; i < n; i++ {
		chars := len(text) * (i + 1) / n
		if chars < 1 && len(text) > 0 {
			chars = 1
		}
		partial := text[:chars]
		f := makeBase(partial, fontName, w, h, c, size, bg)
		if (i/2)%2 == 0 {
			d := &font.Drawer{Dst: f, Src: image.NewUniform(c), Face: face}
			meas := d.MeasureString(partial)
			x := int(((fixed.I(w)-meas)/2 + meas + fixed.I(4)) >> 6)
			for yy := cursorTop; yy < cursorBot; yy++ {
				for xx := x; xx < x+cw; xx++ {
					if xx >= 0 && xx < w && yy >= 0 && yy < h {
						f.SetRGBA(xx, yy, c)
					}
				}
			}
		}
		f = addNoise(f, 0.05, 0.3, rng)
		out = append(out, f)
	}
	return out
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
		return fxType(text, fontName, w, h, c, size, bg, n, rng), nil
	}
	base := makeBase(text, fontName, w, h, c, size, bg)
	switch effect {
	case "vhs":
		return fxVHS(base, n, rng), nil
	case "static":
		return fxStatic(base, n, rng), nil
	case "dropout":
		return fxDropout(base, n, rng), nil
	case "rgb":
		return fxRGB(base, n, rng), nil
	case "flicker":
		return fxFlicker(base, n, rng), nil
	case "ghost":
		return fxGhost(base, n, rng), nil
	case "roll":
		return fxRoll(base, n, rng), nil
	case "glitch":
		return fxGlitch(base, n, rng), nil
	}
	return nil, fmt.Errorf("unknown effect: %s", effect)
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
	effects := []string{"vhs", "static", "dropout", "rgb", "flicker", "ghost", "roll", "glitch", "type"}

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
<tr><td>effect</td><td>vhs</td><td>vhs, static, dropout, rgb, flicker, ghost, roll, glitch, type</td></tr>
<tr><td>font</td><td>monobold</td><td>see the font list below</td></tr>
<tr><td>w</td><td>520</td><td>width, 16 to ` + strconv.Itoa(maxW) + `</td></tr>
<tr><td>h</td><td>60</td><td>height, 16 to ` + strconv.Itoa(maxH) + `</td></tr>
<tr><td>size</td><td>h - 20</td><td>font size in px (minimum 12)</td></tr>
<tr><td>c1</td><td>ff2e88</td><td>text color, hex without <code>#</code> (or <code>%23</code>)</td></tr>
<tr><td>bg</td><td>transparent</td><td>background hex color or <code>transparent</code></td></tr>
<tr><td>fps</td><td>15</td><td>frames per second, 2 to 30</td></tr>
<tr><td>dur</td><td>2.5</td><td>duration in seconds, max 10 (max ` + strconv.Itoa(maxFrames) + ` frames)</td></tr>
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
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/render", handleRender)
	http.HandleFunc("/render.gif", handleRender)
	http.HandleFunc("/fonts", handleFonts)
	addr := ":" + port
	log.Printf("listening on %s (cache=%s)", addr, cacheDir)
	log.Fatal(http.ListenAndServe(addr, nil))
}

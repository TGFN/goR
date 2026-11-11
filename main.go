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
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomonobold"
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
	faceMu   sync.Mutex
	faceMap  = map[float64]font.Face{}
)

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getFace(size float64) font.Face {
	faceMu.Lock()
	defer faceMu.Unlock()
	if f, ok := faceMap[size]; ok {
		return f
	}
	parsed, err := opentype.Parse(gomonobold.TTF)
	if err != nil {
		log.Fatal(err)
	}
	f, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatal(err)
	}
	faceMap[size] = f
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

func makeBase(text string, w, h int, c color.RGBA, size float64, bg color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if bg.A > 0 {
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, bg.A
		}
	}
	if text == "" {
		return img
	}
	face := getFace(size)
	d := &font.Drawer{Dst: img, Src: image.NewUniform(c), Face: face}
	m := d.MeasureString(text)
	mt := face.Metrics()
	d.Dot = fixed.Point26_6{
		X: (fixed.I(w) - m) / 2,
		Y: fixed.I(h)/2 + (mt.Ascent-mt.Descent)/2,
	}
	d.DrawString(text)
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

func fxType(text string, w, h int, c color.RGBA, size float64, bg color.RGBA, n int, rng *rand.Rand) []*image.RGBA {
out := make([]*image.RGBA, 0, n)
face := getFace(size)
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
f := makeBase(partial, w, h, c, size, bg)
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

func renderFrames(text, effect string, w, h int, c1, bgStr string, size float64, fps, n int, seed int64) ([]*image.RGBA, error) {
	rng := rand.New(rand.NewSource(seed))
	c := parseHex(c1)
	var bg color.RGBA
	if bgStr == "" || bgStr == "transparent" {
		bg = color.RGBA{0, 0, 0, 0}
	} else {
		bg = parseHex(bgStr)
	}
	if effect == "type" {
		return fxType(text, w, h, c, size, bg, n, rng), nil
	}
	base := makeBase(text, w, h, c, size, bg)
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

	key := cacheKey(fmt.Sprintf("%s|%s|%d|%d|%s|%s|%.2f|%d|%d|%d",
		text, effect, width, height, c1, bg, size, fps, n, seed))
	cf := filepath.Join(cacheDir, key+".gif")

	var data []byte
	if b, err := os.ReadFile(cf); err == nil {
		data = b
	} else {
		frames, err := renderFrames(text, effect, width, height, c1, bg, size, fps, n, seed)
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

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	effects := []string{"vhs", "static", "dropout", "rgb", "flicker", "ghost", "roll", "glitch", "type"}
	var cards strings.Builder
	for _, e := range effects {
		cards.WriteString(fmt.Sprintf(
			`<div class="card"><h2>%s</h2><img src="/render?text=content+has+been+deleted&effect=%s&c1=ff2e88&fps=12&dur=2&size=28"></div>`,
			e, e))
	}
	page := `<!DOCTYPE html><html><head><meta charset="utf-8"><title>glitch effects</title>
<style>
body{background:#0b0b0f;color:#eee;font-family:ui-monospace,monospace;padding:2rem}
h1{font-size:.8rem;color:#71717a;letter-spacing:.2em;text-transform:uppercase}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:1rem}
.card{background:#111;border:1px solid #222;padding:1rem;border-radius:8px;text-align:center}
.card h2{font-size:.7rem;color:#666;margin:0 0 .75rem;letter-spacing:.15em}
</style></head><body><h1>glitch effects</h1><div class="grid">` + cards.String() + `</div></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

func main() {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/render", handleRender)
	addr := ":" + port
	log.Printf("listening on %s (cache=%s)", addr, cacheDir)
	log.Fatal(http.ListenAndServe(addr, nil))
}

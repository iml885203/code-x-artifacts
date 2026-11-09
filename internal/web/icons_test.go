package web

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// The pictures the metadata points at (home-screen icons and the share
// preview) are drawn here from the mark in DESIGN.md, so they cannot drift from
// it: the test draws them again and compares. After a deliberate change to the
// mark or the composition, PICTOR_UPDATE_ICONS=1 go test -run TestTheIcons
// writes the files.

// oklch converts the design's colours to sRGB.
func oklch(l, c, hDeg float64) color.NRGBA {
	a, b := c*math.Cos(hDeg*math.Pi/180), c*math.Sin(hDeg*math.Pi/180)
	cube := func(v float64) float64 { return v * v * v }
	l1, m1, s1 := cube(l+0.3963377774*a+0.2158037573*b), cube(l-0.1055613458*a-0.0638541728*b), cube(l-0.0894841775*a-1.2914855480*b)
	lin := [3]float64{4.0767416621*l1 - 3.3077115913*m1 + 0.2309699292*s1, -1.2684380046*l1 + 2.6097574011*m1 - 0.3413193965*s1, -0.0041960863*l1 - 0.7034186147*m1 + 1.7076147010*s1}
	var out [3]uint8
	for i, v := range lin {
		v = math.Max(0, math.Min(1, v))
		if v <= 0.0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1/2.4) - 0.055
		}
		out[i] = uint8(math.Round(v * 255))
	}
	return color.NRGBA{out[0], out[1], out[2], 255}
}

var night = oklch(0.15, 0.025, 265)

// drawMark rasterizes the two exact offset-field polygons from the SVG.
func drawMark(img *image.NRGBA, x0, y0, size int) {
	for y := range size {
		for x := range size {
			cx, cy := (float64(x)+0.5)*24/float64(size), (float64(y)+0.5)*24/float64(size)
			first := cx >= 3 && cx < 15 && cy >= 3 && cy < 15 && cx+cy < 24
			second := cx >= 10 && cx < 22 && cy >= 10 && cy < 22 && cx+cy >= 26
			c := color.NRGBA{245, 243, 237, 255}
			if first || second {
				c = color.NRGBA{173, 73, 104, 255}
			}
			img.SetNRGBA(x0+x, y0+y, c)
		}
	}
}

func fill(img *image.NRGBA, c color.NRGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
}

func squareIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	drawMark(img, 0, 0, size)
	return img
}

// transparentIcon keeps only the berry mark for browser tab backgrounds.
func transparentIcon(size int) *image.NRGBA {
	img := squareIcon(size)
	for y := range size {
		for x := range size {
			if img.NRGBAAt(x, y) != (color.NRGBA{173, 73, 104, 255}) {
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	return img
}

// shareImage is the 1200 × 630 preview: the mark on the night, with a little dust.
func shareImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 1200, 630))
	fill(img, night)
	dust := oklch(0.95, 0, 0)
	seed := uint32(7)
	for range 90 {
		seed = seed*1664525 + 1013904223
		x := int(seed>>8) % 1200
		seed = seed*1664525 + 1013904223
		y := int(seed>>8) % 630
		seed = seed*1664525 + 1013904223
		a := uint8(40 + seed>>24%90)
		if x > 380 && x < 820 && y > 95 && y < 535 {
			continue
		}
		// Laid over the night, so the file holds opaque pixels and no format has to get alpha right.
		draw.Draw(img, image.Rect(x, y, x+2, y+1), &image.Uniform{color.NRGBA{dust.R, dust.G, dust.B, a}}, image.Point{}, draw.Over)
	}
	drawMark(img, 390, 105, 420)
	return img
}

func encode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTheIconsAndTheSharePreviewAreTheMarkAsDesignMdDrawsIt(t *testing.T) {
	t.Parallel()
	want := map[string]*image.NRGBA{
		"favicon.png":          transparentIcon(16),
		"apple-touch-icon.png": squareIcon(180),
		"icon-192.png":         squareIcon(192),
		"icon-512.png":         squareIcon(512),
		"og-image.png":         shareImage(),
	}
	for name, img := range want {
		path := filepath.Join("static", name)
		if os.Getenv("PICTOR_UPDATE_ICONS") != "" {
			if err := os.WriteFile(path, encode(t, img), 0o644); err != nil { //nolint:gosec // G306: a static asset served to everyone
				t.Fatal(err)
			}
			continue // Regeneration writes disk; the next run verifies freshly embedded bytes.
		}
		raw, err := staticFS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Bounds() != img.Bounds() {
			t.Errorf("%s is %v, want %v", name, got.Bounds(), img.Bounds())
			continue
		}
		for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
			for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
				if color.NRGBAModel.Convert(got.At(x, y)) != img.NRGBAAt(x, y) {
					t.Fatalf("%s differs from the mark at (%d,%d); PICTOR_UPDATE_ICONS=1 redraws it", name, x, y)
				}
			}
		}
	}
}

// ico encodes the same mark at the sizes used by desktop browser tabs.
func iconFile(t *testing.T) []byte {
	sizes := []uint8{16, 32, 48}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, 3})
	offset := 6 + 16*len(sizes)
	var images [][]byte
	for _, size := range sizes {
		data := encode(t, transparentIcon(int(size)))
		out.Write([]byte{size, size, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
		_ = binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(data)), uint32(offset)}) //nolint:gosec // G115: fixed 16–48px PNGs are far below uint32 limits.
		offset += len(data)
		images = append(images, data)
	}
	for _, data := range images {
		out.Write(data)
	}
	return out.Bytes()
}

func TestTheICOContainsTheBrandMarkAtDesktopTabSizes(t *testing.T) {
	t.Parallel()
	want := iconFile(t)
	for _, path := range []string{"static/favicon.ico", "../../website/favicon.ico"} {
		if os.Getenv("PICTOR_UPDATE_ICONS") != "" {
			if err := os.WriteFile(path, want, 0o644); err != nil { //nolint:gosec // G306: public static assets.
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path) //nolint:gosec // G304: paths are fixed repository assets.
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs from the brand mark; PICTOR_UPDATE_ICONS=1 redraws it", path)
		}
	}
}

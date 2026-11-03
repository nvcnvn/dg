package export

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestEncodeGIF(t *testing.T) {
	tests := []struct {
		name     string
		lossless bool
		colors   int
		palette  []color.RGBA
	}{
		{name: "edge", lossless: true},
		{name: "still", lossless: true},
		{name: "many-colors", colors: 255},
		{name: "ties", lossless: true, palette: []color.RGBA{{0, 0, 255, 255}, {0, 255, 0, 255}, {255, 0, 0, 255}, {0, 0, 0, 255}, {255, 255, 255, 255}, {}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join("..", "..", "testdata", "gif", tt.name)
			paths, err := filepath.Glob(filepath.Join(dir, "frame-*.png"))
			if err != nil || len(paths) == 0 {
				t.Fatal(err, "no frames")
			}
			var frames []image.Image
			for _, p := range paths {
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.Decode(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				frames = append(frames, img)
			}
			got, err := encodeGIF(frames, GIFDelay)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, dir+".gif", got)

			g, err := gif.DecodeAll(bytes.NewReader(got))
			if err != nil {
				t.Fatal(err)
			}
			if len(g.Image) != len(frames) || len(frames) > 1 && g.LoopCount != 0 {
				t.Fatalf("%d frames looping %d times, want %d looping forever", len(g.Image), g.LoopCount, len(frames))
			}
			// The decoder pads the palette to a power of two, so the transparent color is the last
			// one encoded, not necessarily the last one decoded.
			pal := g.Image[0].Palette
			t0 := slices.IndexFunc(pal, func(c color.Color) bool { _, _, _, a := c.RGBA(); return a == 0 })
			if t0 < 0 {
				t.Fatal("the palette has no transparent color")
			}
			unchanged := uint8(t0)
			if tt.colors > 0 && int(unchanged) != tt.colors {
				t.Errorf("palette of %d colors, want %d", unchanged, tt.colors)
			}
			if tt.palette != nil {
				for i, c := range tt.palette {
					if pal[i] != c {
						t.Errorf("palette[%d] = %v, want %v", i, pal[i], c)
					}
				}
			}
			canvas := image.NewRGBA(g.Image[0].Bounds())
			for i, img := range g.Image {
				if g.Disposal[i] != gif.DisposalNone {
					t.Errorf("frame %d has disposal %d, want none", i, g.Disposal[i])
				}
				if i == 0 && bytes.IndexByte(img.Pix, unchanged) >= 0 {
					t.Fatal("the first frame has transparent pixels")
				}
				draw.Draw(canvas, img.Bounds(), img, img.Bounds().Min, draw.Over)
				for y := canvas.Rect.Min.Y; y < canvas.Rect.Max.Y; y++ {
					for x := canvas.Rect.Min.X; x < canvas.Rect.Max.X; x++ {
						shown := canvas.RGBAAt(x, y)
						want := color.RGBAModel.Convert(frames[i].At(x, y)).(color.RGBA)
						if !tt.lossless {
							want = pal[pal[:unchanged].Index(want)].(color.RGBA)
						}
						if shown != want {
							t.Fatalf("frame %d shows %v at %d,%d, want %v", i, shown, x, y, want)
						}
						if i > 0 && frames[i].At(x, y) == frames[i-1].At(x, y) && img.ColorIndexAt(x, y) != unchanged {
							t.Fatalf("frame %d repeats the pixel at %d,%d instead of leaving it transparent", i, x, y)
						}
					}
				}
			}
		})
	}
}

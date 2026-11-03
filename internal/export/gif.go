package export

import (
	"bytes"
	"cmp"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"maps"
	"slices"
	"time"
)

// encodeGIF encodes frames of one size, each shown for delay, as a GIF that loops forever. The
// palette is the 255 colors most frequent across all frames, ties broken by color value, and
// every pixel takes the nearest of them, without dithering. After the first frame, a pixel that
// does not change is transparent, drawn over the frame before.
func encodeGIF(frames []image.Image, delay time.Duration) ([]byte, error) {
	rgba := make([]*image.RGBA, len(frames))
	count := map[color.RGBA]int{}
	for i, f := range frames {
		rgba[i] = image.NewRGBA(f.Bounds())
		draw.Draw(rgba[i], f.Bounds(), f, f.Bounds().Min, draw.Src)
		for p := 0; p < len(rgba[i].Pix); p += 4 {
			count[color.RGBA{rgba[i].Pix[p], rgba[i].Pix[p+1], rgba[i].Pix[p+2], rgba[i].Pix[p+3]}]++
		}
	}
	colors := slices.SortedFunc(maps.Keys(count), func(a, b color.RGBA) int {
		return cmp.Or(count[b]-count[a], cmp.Compare(packRGBA(a), packRGBA(b)))
	})
	pal := make(color.Palette, min(255, len(colors)), min(255, len(colors))+1)
	for i := range pal {
		pal[i] = colors[i]
	}
	unchanged := uint8(len(pal))
	pal = append(pal, color.RGBA{})
	b := frames[0].Bounds()
	g := &gif.GIF{Config: image.Config{ColorModel: pal, Width: b.Dx(), Height: b.Dy()}}
	index := map[color.RGBA]uint8{}
	var prev []uint8
	for _, f := range rgba {
		out := image.NewPaletted(f.Bounds(), pal)
		cur := make([]uint8, len(out.Pix))
		for p := 0; p < len(f.Pix); p += 4 {
			c := color.RGBA{f.Pix[p], f.Pix[p+1], f.Pix[p+2], f.Pix[p+3]}
			i, ok := index[c]
			if !ok {
				i = uint8(pal[:unchanged].Index(c))
				index[c] = i
			}
			cur[p/4] = i
			if out.Pix[p/4] = i; prev != nil && prev[p/4] == i {
				out.Pix[p/4] = unchanged
			}
		}
		prev = cur
		g.Image = append(g.Image, out)
		g.Delay = append(g.Delay, int(delay/(10*time.Millisecond)))
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func packRGBA(c color.RGBA) uint32 {
	return uint32(c.R)<<24 | uint32(c.G)<<16 | uint32(c.B)<<8 | uint32(c.A)
}

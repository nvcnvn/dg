package view

import (
	"bytes"
	"testing"
)

// FuzzDecodeLayout checks that a layout that decodes without errors encodes to a canonical form
// that decodes without errors and encodes to the same bytes.
func FuzzDecodeLayout(f *testing.F) {
	f.Add([]byte("version: 1\nplace:\n  a%3Ab: {in: c, x: 0, y: -10, w: 20, h: 30, order: [x, y]}\n  t:\n    x: 1\n    ports:\n      p: {side: top, offset: 0.4}\n"))
	f.Add([]byte("version: 1\nedge_hints:\n  e: {from_side: left, to_side: right, via: [[1, 2]], label_pos: 0.5}\n  lift:a:b%25: {}\ndecorations:\n  - {id: t, type: text, x: 0, y: 0, text: \"a: b\", stroke_dash: [1, 2.5], opacity: 0.3}\n"))
	f.Add([]byte("version: 1\nplace: {a: {x: 1}, a: {x: 2}}\n"))
	f.Add([]byte("version: 1.0\nplace: {a: {x: 1.5, y: .inf}}\ndecorations:\n  - {id: d, type: rect, stroke_width: .nan, opacity: -0.0}\n"))
	f.Add([]byte("version: 1\n---\nversion: 1\n"))
	f.Add([]byte("version: 1\ndecorations:\n  - {id: a, type: arrow, points: [[0, 0], [10, 5]], attach: n, dx: 1, dy: -2, z: 3}\n  - {id: f, type: freehand, path: \"M0 0 L1 1 Z\", in: g, x: 1, fill: \"#fff\"}\n  - {id: i, type: image, src: a/b.svg, w: 1, h: 2}\n  - {id: e, type: ellipse, w: 1, h: 2, text: x}\n"))
	f.Add([]byte("---\n"))
	f.Add([]byte("version: 1\nplace:\n  t: {w: -10, ports: {a%3Ab: {side: left}}}\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		l, diags := DecodeLayout(src, "f.layout.yaml")
		if diags.HasErrors() {
			return
		}
		first := l.Encode()
		again, diags := DecodeLayout(first, "f.layout.yaml")
		if diags.HasErrors() {
			t.Fatalf("canonical form does not decode: %v\n%s", diags, first)
		}
		if second := again.Encode(); !bytes.Equal(first, second) {
			t.Fatalf("canonical form is not stable:\n%s\n%s", first, second)
		}
	})
}

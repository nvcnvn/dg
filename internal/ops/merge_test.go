package ops

import (
	"bytes"
	"testing"
)

func TestMerge3(t *testing.T) {
	base := "a\nb\nc\nd\ne\n"
	for _, tc := range []struct {
		name, a, b, want string
		ok               bool
	}{
		{"only a", "a\nB\nc\nd\ne\n", base, "a\nB\nc\nd\ne\n", true},
		{"only b", base, "a\nb\nc\nD\ne\n", "a\nb\nc\nD\ne\n", true},
		{"apart", "a\nB\nc\nd\ne\n", "a\nb\nc\nD\ne\n", "a\nB\nc\nD\ne\n", true},
		{"adjacent lines", "a\nB\nc\nd\ne\n", "a\nb\nC\nd\ne\n", "a\nB\nC\nd\ne\n", true},
		{"same change", "a\nX\nc\nd\ne\n", "a\nX\nc\nd\ne\n", "a\nX\nc\nd\ne\n", true},
		{"same line", "a\nX\nc\nd\ne\n", "a\nY\nc\nd\ne\n", "", false},
		{"insert and delete", "a\nb\nc\nd\ne\nf\n", "b\nc\nd\ne\n", "b\nc\nd\ne\nf\n", true},
		{"insert at one place", "a\nb\nx\nc\nd\ne\n", "a\nb\ny\nc\nd\ne\n", "", false},
		{"no final newline", "a\nb\nc\nd\ne\nf", "A\nb\nc\nd\ne\n", "A\nb\nc\nd\ne\nf", true},
	} {
		got, ok := merge3([]byte(base), []byte(tc.a), []byte(tc.b))
		if ok != tc.ok || ok && string(got) != tc.want {
			t.Errorf("%s: merge3 = %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func FuzzDiff(f *testing.F) {
	f.Add("a\nb\nc\n", "a\nc\nd\n")
	f.Add("", "x")
	f.Add("a\nb\nc\nd\ne\n", "a\nb\nx\nc\nd\ne\n")
	f.Add("x\ny\nx\ny\n", "y\nx\ny\nx\n")
	f.Fuzz(func(t *testing.T, a, b string) {
		al, bl := lines([]byte(a)), lines([]byte(b))
		var out [][]byte
		at := 0
		for _, h := range diff(al, bl) {
			if h.start < at || h.end < h.start || h.to < h.from {
				t.Fatalf("hunks out of order: %+v", diff(al, bl))
			}
			out = append(append(out, al[at:h.start]...), bl[h.from:h.to]...)
			at = h.end
		}
		if got := bytes.Join(append(out, al[at:]...), nil); string(got) != b {
			t.Fatalf("applying diff(%q, %q) gives %q", a, b, got)
		}
		if got, ok := merge3([]byte(a), []byte(b), []byte(a)); !ok || string(got) != b {
			t.Fatalf("merge3 with one side unchanged = %q, %v", got, ok)
		}
		if got, ok := merge3([]byte(a), []byte(b), []byte(b)); !ok || string(got) != b {
			t.Fatalf("merge3 of the same change = %q, %v", got, ok)
		}
	})
}

func FuzzMerge3(f *testing.F) {
	f.Add("a\nb\nc\nd\ne\n", "a\nB\nc\nd\ne\n", "a\nb\nc\nD\ne\n")
	f.Add("a\nb\nc\n", "x\na\nb\nc\n", "a\nb\nc\ny\n")
	f.Add("a\nb\n", "a\nx\nb\n", "a\ny\nb\n")
	f.Add("", "a\n", "b\n")
	f.Fuzz(func(t *testing.T, base, a, b string) {
		ab, okAB := merge3([]byte(base), []byte(a), []byte(b))
		ba, okBA := merge3([]byte(base), []byte(b), []byte(a))
		if okAB != okBA || okAB && !bytes.Equal(ab, ba) {
			t.Fatalf("merge3 is not symmetric: %q, %v and %q, %v", ab, okAB, ba, okBA)
		}
		if !okAB {
			return
		}
		for _, side := range []string{a, b} {
			sl := lines([]byte(side))
			for _, h := range diff(lines([]byte(base)), sl) {
				if added := bytes.Join(sl[h.from:h.to], nil); !bytes.Contains(ab, added) {
					t.Fatalf("merge3(%q, %q, %q) = %q lacks %q", base, a, b, ab, added)
				}
			}
		}
	})
}

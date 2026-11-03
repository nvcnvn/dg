package ops

import (
	"bytes"
	"slices"
)

// hunk replaces the lines [start, end) of a base with the lines [from, to) of another version.
type hunk struct {
	start, end, from, to int
}

// merge3 merges into base both the changes from base to a and those from base to b, line by
// line. It fails when a change of a and one of b touch the same lines of base, unless they are
// the same change, or insert at the same place.
func merge3(base, a, b []byte) ([]byte, bool) {
	bl, al, cl := lines(base), lines(a), lines(b)
	ha, hb := diff(bl, al), diff(bl, cl)
	type side struct {
		h     hunk
		lines [][]byte
	}
	var all []side
	for _, h := range ha {
		all = append(all, side{h, al[h.from:h.to]})
	}
	for _, h := range hb {
		other := cl[h.from:h.to]
		dup := false
		for _, s := range all[:len(ha)] {
			if !overlaps(s.h, h) {
				continue
			}
			if s.h.start != h.start || s.h.end != h.end || !slices.EqualFunc(s.lines, other, bytes.Equal) {
				return nil, false
			}
			dup = true
		}
		if !dup {
			all = append(all, side{h, other})
		}
	}
	slices.SortStableFunc(all, func(x, y side) int { return x.h.start - y.h.start })
	var out [][]byte
	at := 0
	for _, s := range all {
		out = append(append(out, bl[at:s.h.start]...), s.lines...)
		at = s.h.end
	}
	return bytes.Join(append(out, bl[at:]...), nil), true
}

// overlaps reports whether two hunks of the same base touch the same lines, or insert at the
// same place.
func overlaps(x, y hunk) bool {
	return x.start < y.end && y.start < x.end || x.start == y.start
}

// lines splits s after each newline, keeping the newlines.
func lines(s []byte) [][]byte {
	var out [][]byte
	for len(s) > 0 {
		i := bytes.IndexByte(s, '\n') + 1
		if i == 0 {
			i = len(s)
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	return out
}

// diff returns the hunks that turn a into b, by Myers' algorithm on the lines between their
// common prefix and suffix.
func diff(a, b [][]byte) []hunk {
	pre := 0
	for pre < len(a) && pre < len(b) && bytes.Equal(a[pre], b[pre]) {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && bytes.Equal(a[len(a)-1-suf], b[len(b)-1-suf]) {
		suf++
	}
	x, y := a[pre:len(a)-suf], b[pre:len(b)-suf]
	n, m := len(x), len(y)
	if n == 0 && m == 0 {
		return nil
	}
	off := n + m
	v := make([]int, 2*off+2)
	var trace [][]int
	d := 0
search:
	for ; d <= n+m; d++ {
		trace = append(trace, slices.Clone(v))
		for k := -d; k <= d; k += 2 {
			var i int
			if k == -d || k != d && v[off+k-1] < v[off+k+1] {
				i = v[off+k+1]
			} else {
				i = v[off+k-1] + 1
			}
			j := i - k
			for i < n && j < m && bytes.Equal(x[i], y[j]) {
				i, j = i+1, j+1
			}
			v[off+k] = i
			if i >= n && j >= m {
				break search
			}
		}
	}
	// Walk back from (n, m): each step of trace[d] ends with a diagonal of matched lines.
	type match struct{ i, j int }
	var matches []match
	i, j := n, m
	for ; d >= 0; d-- {
		pv := trace[d]
		k := i - j
		pk := k - 1
		if k == -d || k != d && pv[off+k-1] < pv[off+k+1] {
			pk = k + 1
		}
		pi := pv[off+pk]
		for i > pi && i-k > pi-pk && i > 0 && i-k > 0 {
			matches = append(matches, match{i - 1, i - k - 1})
			i--
		}
		i, j = pi, pi-pk
	}
	slices.Reverse(matches)
	var out []hunk
	pi, pj := 0, 0
	for _, mt := range append(matches, match{n, m}) {
		if mt.i > pi || mt.j > pj {
			out = append(out, hunk{pre + pi, pre + mt.i, pre + pj, pre + mt.j})
		}
		pi, pj = mt.i+1, mt.j+1
	}
	return out
}

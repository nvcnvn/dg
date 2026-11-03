package lsp

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// lines is a text with the offsets where its lines start, to convert between byte offsets and
// the protocol's positions, zero-based lines and columns in UTF-16 code units.
type lines struct {
	text   string
	starts []int
}

func newLines(text string) *lines {
	l := &lines{text: text, starts: []int{0}}
	for i := strings.IndexByte(text, '\n'); i >= 0; {
		start := l.starts[len(l.starts)-1] + i + 1
		l.starts = append(l.starts, start)
		i = strings.IndexByte(text[start:], '\n')
	}
	return l
}

// position returns the position of byte offset off.
func (l *lines) position(off int) Position {
	off = min(max(off, 0), len(l.text))
	line := sort.Search(len(l.starts), func(i int) bool { return l.starts[i] > off }) - 1
	return Position{Line: line, Character: utf16Len(l.text[l.starts[line]:off])}
}

// offset returns the byte offset of p, clamped to its line and to the text.
func (l *lines) offset(p Position) int {
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(l.starts) {
		return len(l.text)
	}
	off := l.starts[p.Line]
	for units := 0; units < p.Character && off < len(l.text) && l.text[off] != '\n'; {
		r, size := utf8.DecodeRuneInString(l.text[off:])
		units += utf16Width(r)
		off += size
	}
	return off
}

func (l *lines) rng(start, end int) Range {
	return Range{l.position(start), l.position(end)}
}

// utf16Len is the length of s in UTF-16 code units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Width(r)
	}
	return n
}

func utf16Width(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

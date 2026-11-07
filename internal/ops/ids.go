package ops

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nvcnvn/dg/internal/spec"
)

// slug lowercases s and keeps a-z and 0-9, every other run of characters becoming one dash,
// trimmed of dashes and cut to 64 characters (spec 6.2).
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	return strings.TrimSuffix(b.String()[:min(b.Len(), 64)], "-")
}

// newID returns an ID for base that taken does not hold and is not reserved: base itself, or
// base with the first free suffix -2, -3, …; a base that does not start with a letter is
// prefixed with n-.
func newID(base string, taken func(string) bool) string {
	if base == "" || base[0] < 'a' || base[0] > 'z' {
		base = strings.TrimSuffix("n-"+base, "-")
	}
	id := base
	for i := 2; taken(id) || slices.Contains(spec.Reserved, id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

// nodeID is the ID of a new node (spec 6.2), and whether it is derived from kind, its kind's name
// without an installed pack's prefix.
func nodeID(kind, label string, taken func(string) bool) (string, bool) {
	if base := slug(label); base != "" {
		return newID(base, taken), false
	}
	return newID(slug(kind), taken), true
}

// edgeID is the ID of a new edge from node from to node to (spec 6.2).
func edgeID(from, to string, taken func(string) bool) string {
	return newID(slug(slug(from)+"-"+slug(to)), taken)
}

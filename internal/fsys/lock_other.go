//go:build !(darwin || linux || freebsd || netbsd || openbsd || dragonfly || windows)

package fsys

import "time"

// lockDir has no lock across processes on this system; writers in one process are still
// serialized.
func lockDir(string, time.Time) (func(), error) {
	return func() {}, nil
}

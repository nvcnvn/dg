// Package version holds the version of this build of dg.
package version

import "runtime/debug"

// stamped is the version the Makefile links into the binaries of a release, whose Git checkout
// the release workflow leaves modified.
var stamped string

// String returns the version the Makefile or the Go toolchain stamped in the binary, the latter
// from the build's module or its Git checkout.
func String() string {
	if stamped != "" {
		return stamped
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}

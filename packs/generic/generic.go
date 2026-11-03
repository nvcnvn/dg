// Package generic embeds the built-in generic pack.
package generic

import "embed"

// Files holds the pack's spec.hcl and style.hcl.
//
//go:embed spec.hcl style.hcl
var Files embed.FS

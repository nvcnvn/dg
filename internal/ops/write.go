// Package ops applies operations and writes files.
package ops

import "github.com/nvcnvn/dg/internal/fsys"

// WriteFiles writes files outside a project, by their paths on disk; see fsys.WriteFiles.
func WriteFiles(files map[string][]byte) error {
	return fsys.WriteFiles(files)
}

// Package ops applies operations and writes files.
package ops

import "github.com/nvcnvn/dg/internal/fsys"

// Save writes file name as a text editor saves it, as the only writer of the project.
func Save(files fsys.FS, name string, data []byte) error {
	unlock, err := files.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	return files.Write(map[string][]byte{name: data})
}

// WriteFiles writes files outside a project, by their paths on disk; see fsys.WriteFiles.
func WriteFiles(files map[string][]byte) error {
	return fsys.WriteFiles(files)
}

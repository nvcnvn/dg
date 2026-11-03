// Package ops applies operations and writes files.
package ops

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// WriteFile writes data to path atomically, through a temporary file renamed over it.
func WriteFile(path string, data []byte) error {
	return WriteFiles(map[string][]byte{path: data})
}

// WriteFiles writes every file through a temporary file beside it. No file is replaced until
// every temporary file is written; then each is renamed over its target. A file that exists keeps
// its permissions, and a symbolic link is written through to the file it names.
func WriteFiles(files map[string][]byte) error {
	var targets, tmps []string
	cleanup := func(err error) error {
		for _, t := range tmps {
			err = errors.Join(err, os.Remove(t))
		}
		return err
	}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		target, mode, err := resolve(path)
		if err != nil {
			return cleanup(err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp*")
		if err != nil {
			return cleanup(err)
		}
		tmps = append(tmps, tmp.Name())
		targets = append(targets, target)
		_, err = tmp.Write(files[path])
		if err = errors.Join(err, tmp.Chmod(mode), tmp.Close()); err != nil {
			return cleanup(err)
		}
	}
	for i, target := range targets {
		if err := os.Rename(tmps[i], target); err != nil {
			tmps = tmps[i:]
			return cleanup(err)
		}
	}
	return nil
}

// resolve returns the file that writing path replaces, following symbolic links, and the
// permissions to give it.
func resolve(path string) (string, fs.FileMode, error) {
	target, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, 0o644, nil
	}
	if err != nil {
		return "", 0, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", 0, err
	}
	return target, info.Mode().Perm(), nil
}

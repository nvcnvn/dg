package fsys

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Disk is the project in a directory. Reads do not follow links out of it, and writes are refused
// where a link would lead out of it.
type Disk string

// Open opens file name of the project, through an os.Root held only as long as the call, so that
// a long-lived Disk neither pins the directory nor misses it being replaced.
func (d Disk) Open(name string) (fs.File, error) {
	root, err := os.OpenRoot(string(d))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.FS().Open(name)
}

// Write writes files through WriteFiles. Paths that lead out of the project through a symbolic
// link are refused before anything is written, and directories created for new files are removed
// again if the write fails.
func (d Disk) Write(files map[string][]byte) error {
	dir := string(d)
	realRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	out := map[string][]byte{}
	var missing []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		existing := p
		for {
			if _, err := os.Lstat(existing); err == nil {
				break
			}
			if files[name] != nil && existing != p && !slices.Contains(missing, existing) {
				missing = append(missing, existing)
			}
			existing = filepath.Dir(existing)
		}
		real, err := filepath.EvalSymlinks(existing)
		if err != nil {
			return err
		}
		if rel, err := filepath.Rel(realRoot, real); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s links outside the project", name)
		}
		out[p] = files[name]
	}
	slices.Sort(missing)
	var created []string
	for _, m := range missing {
		if err = os.Mkdir(m, 0o755); err != nil {
			break
		}
		created = append(created, m)
	}
	if err == nil {
		err = WriteFiles(out)
	}
	if err != nil {
		for _, c := range slices.Backward(created) {
			os.Remove(c)
		}
	}
	return err
}

// WriteFiles writes every file, named by its path on disk, through a temporary file beside it. No
// file is replaced until every temporary file is written; then each is renamed over its target,
// and the files whose content is nil are removed. A file that exists keeps its permissions, and a
// symbolic link is written through to the file it names.
func WriteFiles(files map[string][]byte) error {
	var targets, tmps, removed []string
	cleanup := func(err error) error {
		for _, t := range tmps {
			err = errors.Join(err, os.Remove(t))
		}
		return err
	}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		if files[path] == nil {
			removed = append(removed, path)
			continue
		}
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
	for _, path := range removed {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
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
	if info.IsDir() {
		return "", 0, fmt.Errorf("%s is a directory", path)
	}
	return target, info.Mode().Perm(), nil
}

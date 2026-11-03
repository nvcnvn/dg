package ops

import (
	"errors"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"testing/fstest"
)

// files is a project's files as an operation sees them: the files it changed over the project
// directory, remembering every file it read there so that a write can check they are unchanged.
// A nil content is a removed file.
type files struct {
	base    fs.FS
	changed map[string][]byte
	read    map[string][]byte
}

func newFiles(base fs.FS) *files {
	return &files{base: base, changed: map[string][]byte{}, read: map[string][]byte{}}
}

// get returns the current content of file name, and whether it exists.
func (f *files) get(name string) ([]byte, bool) {
	if data, ok := f.changed[name]; ok {
		return data, data != nil
	}
	data, err := f.ReadFile(name)
	return data, err == nil
}

func (f *files) put(name string, data []byte) {
	f.get(name)
	if data == nil {
		data = []byte{}
	}
	f.changed[name] = data
}

func (f *files) remove(name string) {
	f.get(name)
	f.changed[name] = nil
}

// overlay is a file system of the changed files that exist.
func (f *files) overlay() fstest.MapFS {
	m := fstest.MapFS{}
	for name, data := range f.changed {
		if data != nil {
			m[name] = &fstest.MapFile{Data: data, Mode: 0o644}
		}
	}
	return m
}

func (f *files) ReadFile(name string) ([]byte, error) {
	if data, ok := f.changed[name]; ok {
		if data == nil {
			return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
		}
		return slices.Clone(data), nil
	}
	if data, ok := f.read[name]; ok {
		if data == nil {
			return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
		}
		return slices.Clone(data), nil
	}
	data, err := fs.ReadFile(f.base, name)
	switch {
	case err == nil:
		f.read[name] = data
	case errors.Is(err, fs.ErrNotExist):
		f.read[name] = nil
	}
	return slices.Clone(data), err
}

func (f *files) Open(name string) (fs.File, error) {
	if _, ok := f.changed[name]; ok || f.isNewDir(name) {
		return f.overlay().Open(name)
	}
	if info, err := fs.Stat(f.base, name); err == nil && info.IsDir() {
		return f.base.Open(name)
	}
	data, err := f.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return fstest.MapFS{name: {Data: data}}.Open(name)
}

func (f *files) Stat(name string) (fs.FileInfo, error) {
	if _, ok := f.changed[name]; ok || f.isNewDir(name) {
		return f.overlay().Stat(name)
	}
	return fs.Stat(f.base, name)
}

// isNewDir reports whether name is a directory that only changed files are in.
func (f *files) isNewDir(name string) bool {
	if _, err := fs.Stat(f.base, name); err == nil {
		return false
	}
	return slices.ContainsFunc(slices.Collect(maps.Keys(f.overlay())), func(p string) bool {
		return strings.HasPrefix(p, name+"/")
	})
}

func (f *files) ReadDir(name string) ([]fs.DirEntry, error) {
	base, err := fs.ReadDir(f.base, name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	added, addErr := f.overlay().ReadDir(name)
	if err != nil && addErr != nil {
		return nil, err
	}
	out := slices.DeleteFunc(base, func(e fs.DirEntry) bool {
		_, changed := f.changed[path.Join(name, e.Name())]
		return changed
	})
	for _, e := range added {
		if !slices.ContainsFunc(out, func(o fs.DirEntry) bool { return o.Name() == e.Name() }) {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return out, nil
}

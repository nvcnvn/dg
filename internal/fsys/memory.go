package fsys

import (
	"io/fs"
	"maps"
	"path"
	"slices"
	"sync"
	"syscall"
	"testing/fstest"
)

// Memory is a project held in memory.
type Memory struct {
	writer sync.Mutex
	mu     sync.RWMutex
	files  fstest.MapFS
}

// NewMemory returns a project of files, by path; the content is copied.
func NewMemory(files map[string][]byte) (*Memory, error) {
	m := &Memory{files: fstest.MapFS{}}
	if err := m.Write(files); err != nil {
		return nil, err
	}
	return m, nil
}

// Open opens file name of the project.
func (m *Memory) Open(name string) (fs.File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.files.Open(name)
}

// Lock makes the caller the only writer of the project until unlock is called.
func (m *Memory) Lock() (unlock func(), err error) {
	m.writer.Lock()
	return m.writer.Unlock, nil
}

// Write writes files; a path that is not valid, or that passes through a file, refuses them all.
func (m *Memory) Write(files map[string][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := maps.Clone(m.files)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if !fs.ValidPath(name) || name == "." {
			return &fs.PathError{Op: "write", Path: name, Err: fs.ErrInvalid}
		}
		if files[name] == nil {
			delete(next, name)
			continue
		}
		next[name] = &fstest.MapFile{Data: slices.Clone(files[name]), Mode: 0o644}
	}
	for _, name := range slices.Sorted(maps.Keys(next)) {
		for d := path.Dir(name); d != "."; d = path.Dir(d) {
			if _, isFile := next[d]; isFile {
				return &fs.PathError{Op: "write", Path: name, Err: syscall.ENOTDIR}
			}
		}
	}
	m.files = next
	return nil
}

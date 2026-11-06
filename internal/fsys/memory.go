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

// Memory is a project held in memory, where a directory exists only while it holds a file.
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
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if throughFile(m.files, name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: syscall.ENOTDIR}
	}
	return m.files.Open(name)
}

// Lock makes the caller the only writer of the project until unlock is called.
func (m *Memory) Lock() (unlock func(), err error) {
	m.writer.Lock()
	return m.writer.Unlock, nil
}

// Write writes files, or none of them.
func (m *Memory) Write(files map[string][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := maps.Clone(m.files)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if err := writable(m.files, name); err != nil {
			return err
		}
		if files[name] == nil {
			delete(next, name)
			continue
		}
		next[name] = &fstest.MapFile{Data: slices.Clone(files[name]), Mode: 0o644}
	}
	for _, name := range slices.Sorted(maps.Keys(next)) {
		if err := writable(next, name); err != nil {
			return err
		}
	}
	m.files = next
	return nil
}

// writable returns why name cannot be written or removed in files.
func writable(files fstest.MapFS, name string) error {
	if !fs.ValidPath(name) || name == "." {
		return &fs.PathError{Op: "write", Path: name, Err: fs.ErrInvalid}
	}
	if throughFile(files, name) {
		return &fs.PathError{Op: "write", Path: name, Err: syscall.ENOTDIR}
	}
	if info, err := files.Stat(name); err == nil && info.IsDir() {
		return &fs.PathError{Op: "write", Path: name, Err: syscall.EISDIR}
	}
	return nil
}

// throughFile reports whether a directory on the way to name is a file.
func throughFile(files fstest.MapFS, name string) bool {
	for d := path.Dir(name); d != "."; d = path.Dir(d) {
		if _, isFile := files[d]; isFile {
			return true
		}
	}
	return false
}

// Package fsys holds the files of one project: a directory on disk, or files in memory.
package fsys

import "io/fs"

// FS is a project's files, by slash-separated paths relative to its root.
type FS interface {
	fs.FS
	// Lock makes the caller the only writer of the project until unlock is called.
	Lock() (unlock func(), err error)
	// Write writes every file, all together, with the directories they need, and removes the
	// files whose content is nil. A refused write changes nothing.
	Write(files map[string][]byte) error
}

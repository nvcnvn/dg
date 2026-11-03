package server

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// settle is how long Watch waits after a change for more changes before reloading, so a save
// that writes several files, or writes one file in steps, reloads once.
const settle = 20 * time.Millisecond

// Watch watches every directory of the project, except hidden ones and node_modules, and
// reloads after each change until ctx is done (FR-13). Directories created later are watched
// too. Errors from the watcher are passed to report.
func (s *Server) Watch(ctx context.Context, report func(error)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := watchTree(w, s.dir); err != nil {
		return err
	}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-w.Events:
			if skipped(ev.Name) {
				continue
			}
			if ev.Has(fsnotify.Create) {
				_ = watchTree(w, ev.Name)
			}
			timer.Reset(settle)
		case err := <-w.Errors:
			report(err)
		case <-timer.C:
			s.Reload(ctx)
		}
	}
}

// watchTree adds root and every directory under it that is not skipped; root may be a file.
func watchTree(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != root && skipped(path) {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
}

func skipped(path string) bool {
	base := filepath.Base(path)
	return base == "node_modules" || strings.HasPrefix(base, ".") && base != "." && base != ".."
}

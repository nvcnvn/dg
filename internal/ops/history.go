package ops

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/project"
)

// History is the undo log of an editing session (spec FR-10): the changes applied, as their
// files before and after, and the changes undone, which redo applies again.
type History struct {
	done, undone []*Change
}

// Record adds an applied change, which clears what can be redone.
func (h *History) Record(c *Change) {
	if len(c.Files) == 0 {
		return
	}
	h.done = append(h.done, c)
	h.undone = nil
}

// CanUndo reports whether there is a change to undo.
func (h *History) CanUndo() bool { return len(h.done) > 0 }

// CanRedo reports whether there is a change to redo.
func (h *History) CanRedo() bool { return len(h.undone) > 0 }

// Undo restores the files of the last change to what they were before it, and sess to its
// state before it. A file changed since the change was applied keeps those other changes: the
// change is taken out of it by a three-way merge. If that conflicts or would add validation
// errors, or a file the change created or removed was changed since, the change is dropped and
// Undo fails; on other failures it is kept.
func (h *History) Undo(dir string, sess *Session) (*Change, error) {
	return replay(dir, sess, &h.done, &h.undone, "undo", 0)
}

// Redo applies the last undone change again, under the same conditions as Undo.
func (h *History) Redo(dir string, sess *Session) (*Change, error) {
	return replay(dir, sess, &h.undone, &h.done, "redo", 1)
}

func replay(dir string, sess *Session, from, to *[]*Change, verb string, side int) (*Change, error) {
	if len(*from) == 0 {
		return nil, fmt.Errorf("nothing to %s", verb)
	}
	content := func(fc FileChange, s int) []byte {
		if s == 0 {
			return fc.Before
		}
		return fc.After
	}
	c := (*from)[len(*from)-1]
	unlock, err := lockProject(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	p, before, err := project.Load(dir)
	if err != nil {
		return nil, err
	}
	if p.ConfigInvalid {
		return nil, fmt.Errorf("cannot %s: dg.hcl has errors, so read_only may be incomplete", verb)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	drop := func(format string, args ...any) (*Change, error) {
		*from = (*from)[:len(*from)-1]
		return nil, fmt.Errorf("cannot %s: %s; the change is dropped", verb, fmt.Sprintf(format, args...))
	}
	f := newFiles(root.FS())
	merges := false
	for _, fc := range c.Files {
		if p.IsReadOnly(fc.Path) {
			return nil, fmt.Errorf("cannot %s: %s is read-only (read_only in dg.hcl)", verb, fc.Path)
		}
		got, err := fs.ReadFile(root.FS(), fc.Path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		was, want := content(fc, 1-side), content(fc, side)
		merged := want
		switch {
		case was == nil && err == nil:
			return drop("%s, which the change removed, exists again", fc.Path)
		case was != nil && err != nil:
			return drop("%s was removed since", fc.Path)
		case was == nil || bytes.Equal(got, was):
		case want == nil:
			return drop("%s, which the change created, was changed since", fc.Path)
		default:
			var ok bool
			if merged, ok = merge3(was, want, got); !ok {
				return drop("%s was changed since in the same place", fc.Path)
			}
			merges = true
		}
		if merged == nil {
			f.remove(fc.Path)
		} else {
			f.put(fc.Path, merged)
		}
	}
	// A merge can be clean line by line and still make the project invalid, such as a key
	// added twice; like an operation (FR-12), it may not add errors.
	if merges {
		_, after, err := project.LoadFS(f, dir)
		if err != nil {
			return nil, err
		}
		if errs := added(before, after, diag.Error); len(errs) > 0 {
			return drop("merging it with the changes made since would add errors: %s", errs[0].String())
		}
	}
	out := f.changed
	if err := commit(dir, c.Files, func(fc FileChange) []byte { return out[fc.Path] }); err != nil {
		return nil, err
	}
	*from = (*from)[:len(*from)-1]
	*to = append(*to, c)
	if sess != nil {
		sess.kindIDs = maps.Clone(c.sessions[side])
		if sess.kindIDs == nil {
			sess.kindIDs = map[string]bool{}
		}
	}
	return c, nil
}

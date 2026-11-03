package ops

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"

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
// state before it. If a file no longer holds what the change wrote, the change is dropped and
// Undo fails; on other failures it is kept.
func (h *History) Undo(dir string, sess *Session) (*Change, error) {
	return replay(dir, sess, &h.done, &h.undone, "undo", 0)
}

// Redo applies the last undone change again, under the same conditions as Undo.
func (h *History) Redo(dir string, sess *Session) (*Change, error) {
	return replay(dir, sess, &h.undone, &h.done, "redo", 1)
}

// replay moves the last change of from to to, writing its files' contents on side to of
// before (0) and after (1).
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
	p, _, err := project.Load(dir)
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
	for _, fc := range c.Files {
		if p.IsReadOnly(fc.Path) {
			return nil, fmt.Errorf("cannot %s: %s is read-only (read_only in dg.hcl)", verb, fc.Path)
		}
		got, err := fs.ReadFile(root.FS(), fc.Path)
		w := content(fc, 1-side)
		if w == nil && !errors.Is(err, fs.ErrNotExist) || w != nil && (err != nil || !bytes.Equal(got, w)) {
			*from = (*from)[:len(*from)-1]
			return nil, fmt.Errorf("cannot %s: %s changed since; the change is dropped", verb, fc.Path)
		}
	}
	if err := commit(dir, c.Files, func(fc FileChange) []byte { return content(fc, side) }); err != nil {
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

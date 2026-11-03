package ops

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

// Session is what operations share within one editing session: the nodes created with an ID
// derived from their kind, which take a new ID from their first label (spec 6.3). Each dg op
// command and MCP call is a session; dg serve keeps one for its lifetime.
type Session struct {
	kindIDs map[string]bool
}

// NewSession returns an empty session.
func NewSession() *Session {
	return &Session{kindIDs: map[string]bool{}}
}

// Change is what an applied operation did: its results, the warnings it added, and every file
// it wrote with its content before and after (nil when the file does not exist), which is the
// operation's inverse.
type Change struct {
	Results  []Result         `json:"results"`
	Warnings diag.Diagnostics `json:"warnings,omitempty"`
	Files    []FileChange     `json:"-"`
	// sessions are the session's kind-derived IDs before and after the change, which undo and
	// redo restore.
	sessions [2]map[string]bool
}

// FileChange is a file an operation wrote; Path is project-relative.
type FileChange struct {
	Path   string
	Before []byte
	After  []byte
}

// Paths returns the project-relative paths of the files c wrote.
func (c *Change) Paths() []string {
	out := []string{}
	for _, f := range c.Files {
		out = append(out, f.Path)
	}
	return out
}

// Result is the result of one operation: the ID of the element it created or changed (after a
// rename or re-slug, the new ID) and, for move and resize, the values stored after snapping to
// the grid.
type Result struct {
	Op string `json:"op"`
	ID string `json:"id,omitempty"`
	X  *int   `json:"x,omitempty"`
	Y  *int   `json:"y,omitempty"`
	W  *int   `json:"w,omitempty"`
	H  *int   `json:"h,omitempty"`
}

// RefusedError is an operation refused because it would add validation errors (spec FR-2);
// the diagnostics are those errors, positioned in the files as they would be written.
type RefusedError struct {
	Diagnostics diag.Diagnostics
}

func (e *RefusedError) Error() string {
	var b strings.Builder
	b.WriteString("refused: the operation would add these errors:")
	for _, d := range e.Diagnostics {
		b.WriteString("\n  " + d.String())
	}
	return b.String()
}

// beforeWrite, when set by a test, runs between reading the project and checking that its files
// are unchanged.
var beforeWrite func()

const attempts = 3

// Apply applies op to the project in dir and writes the files it changes, all together. If a
// file changes between reading and writing, it reloads and applies op again (spec FR-12).
func Apply(dir string, op Op, sess *Session) (*Change, error) {
	unlock, err := lockProject(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for range attempts {
		trial := &Session{kindIDs: maps.Clone(sess.kindIDs)}
		c, f, err := plan(root.FS(), dir, op, trial)
		if err != nil {
			return nil, err
		}
		if beforeWrite != nil {
			beforeWrite()
		}
		if !unchanged(root.FS(), f.read) {
			continue
		}
		if err := commit(dir, c.Files, func(fc FileChange) []byte { return fc.After }); err != nil {
			return nil, err
		}
		c.sessions = [2]map[string]bool{maps.Clone(sess.kindIDs), maps.Clone(trial.kindIDs)}
		*sess = *trial
		return c, nil
	}
	return nil, errors.New("the project's files kept changing while the operation was applied; try again")
}

// unchanged reports whether every file in read still has the content read (nil: absent).
func unchanged(fsys fs.FS, read map[string][]byte) bool {
	for name, want := range read {
		got, err := fs.ReadFile(fsys, name)
		if want == nil && !errors.Is(err, fs.ErrNotExist) || want != nil && (err != nil || !bytes.Equal(got, want)) {
			return false
		}
	}
	return true
}

// commit writes content(f) to each file, or removes it when that is nil, all together. Paths
// that lead out of the project through a symbolic link are refused before anything is written,
// and directories created for new files are removed again if the write fails.
func commit(dir string, changes []FileChange, content func(FileChange) []byte) error {
	realRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	out := map[string][]byte{}
	var missing []string
	for _, fc := range changes {
		p := filepath.Join(dir, filepath.FromSlash(fc.Path))
		existing := p
		for {
			if _, err := os.Lstat(existing); err == nil {
				break
			}
			if content(fc) != nil && existing != p && !slices.Contains(missing, existing) {
				missing = append(missing, existing)
			}
			existing = filepath.Dir(existing)
		}
		real, err := filepath.EvalSymlinks(existing)
		if err != nil {
			return err
		}
		if rel, err := filepath.Rel(realRoot, real); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s links outside the project", fc.Path)
		}
		out[p] = content(fc)
	}
	slices.Sort(missing)
	var created []string
	for _, d := range missing {
		if err = os.Mkdir(d, 0o755); err != nil {
			break
		}
		created = append(created, d)
	}
	if err == nil {
		err = WriteFiles(out)
	}
	if err != nil {
		for _, d := range slices.Backward(created) {
			os.Remove(d)
		}
	}
	return err
}

// state is a project while an operation is applied to it.
type state struct {
	dir       string
	layoutDir string
	viewsPath string
	f         *files
	p         *project.Project // the project as the files now are; nil after a model edit until reloaded
	before    diag.Diagnostics
	sess      *Session
	out       []Result
}

// Plan applies op to the project whose files fsys holds, as Apply would but writing nothing, and
// returns the change it would make. dir is recorded as the project's directory.
func Plan(fsys fs.FS, dir string, op Op) (*Change, error) {
	c, _, err := plan(fsys, dir, op, NewSession())
	return c, err
}

// plan applies op to the project in base without writing, returning the change and the files
// it read.
func plan(base fs.FS, dir string, op Op, sess *Session) (*Change, *files, error) {
	f := newFiles(base)
	p, before, err := project.LoadFS(f, dir)
	if err != nil {
		return nil, nil, err
	}
	if p.ConfigInvalid {
		return nil, nil, errors.New("dg.hcl has errors; fix them before applying operations, since read_only may be incomplete")
	}
	if len(p.Unparsed) > 0 {
		return nil, nil, fmt.Errorf("%s does not parse; fix it before applying operations", strings.Join(p.Unparsed, ", "))
	}
	s := &state{dir: dir, layoutDir: p.LayoutDir, viewsPath: p.ViewsFile, f: f, p: p, before: before, sess: sess}
	if err := s.apply(op); err != nil {
		return nil, nil, err
	}

	c := &Change{Results: s.out}
	for _, name := range slices.Sorted(maps.Keys(f.changed)) {
		after := f.changed[name]
		if prev := f.read[name]; bytes.Equal(prev, after) && (prev == nil) == (after == nil) {
			continue
		}
		if p.IsReadOnly(name) {
			return nil, nil, fmt.Errorf("%s is read-only (read_only in dg.hcl); the operation would change it", name)
		}
		c.Files = append(c.Files, FileChange{Path: name, Before: f.read[name], After: after})
	}
	if len(c.Files) == 0 {
		return c, f, nil
	}
	_, after, err := project.LoadFS(f, dir)
	if err != nil {
		return nil, nil, err
	}
	// A diagnostic is new when the operation adds one with its summary. Details are compared only
	// to say which ones: they name IDs, which a rename changes.
	added := func(sev diag.Severity) diag.Diagnostics {
		count := map[string]int{}
		exact := map[[2]string]int{}
		for _, d := range before {
			if d.Severity == sev {
				count[d.Summary]++
				exact[[2]string{d.Summary, d.Detail}]++
			}
		}
		for _, d := range after {
			if d.Severity == sev {
				count[d.Summary]--
			}
		}
		var out diag.Diagnostics
		for _, d := range after {
			k := [2]string{d.Summary, d.Detail}
			if d.Severity != sev || count[d.Summary] >= 0 {
				continue
			}
			if exact[k] > 0 {
				exact[k]--
				continue
			}
			out = append(out, d)
		}
		return out
	}
	if errs := added(diag.Error); len(errs) > 0 {
		return nil, nil, &RefusedError{Diagnostics: errs}
	}
	c.Warnings = added(diag.Warning)
	return c, f, nil
}

func (s *state) project() (*project.Project, error) {
	if s.p == nil {
		p, _, err := project.LoadFS(s.f, s.dir)
		if err != nil {
			return nil, err
		}
		s.p = p
	}
	return s.p, nil
}

func (s *state) result(r Result) {
	s.out = append(s.out, r)
}

// hclFile parses file name for editing; a file that does not exist is empty.
func (s *state) hclFile(name string) (*hclwrite.File, error) {
	src, _ := s.f.get(name)
	return parse(name, src)
}

func (s *state) putHCL(name string, f *hclwrite.File) {
	s.f.put(name, source(f))
	s.p = nil
}

// block returns the block of node or edge id, on several lines, with its file.
func (s *state) block(typ, id string) (string, *hclwrite.File, *hclwrite.Block, error) {
	p, err := s.project()
	if err != nil {
		return "", nil, nil, err
	}
	var rng hcl.Range
	switch {
	case typ == "node" && p.Model.Nodes[id] != nil:
		rng = p.Model.Nodes[id].Range
	case typ == "edge" && p.Model.Edges[id] != nil:
		rng = p.Model.Edges[id].Range
	default:
		return "", nil, nil, fmt.Errorf("%s %q does not exist", typ, id)
	}
	f, err := s.hclFile(rng.Filename)
	if err != nil {
		return "", nil, nil, err
	}
	b := findBlock(f.Body(), typ, []string{id})
	if b == nil {
		return "", nil, nil, fmt.Errorf("%s %q is not a block of %s", typ, id, rng.Filename)
	}
	f, b, err = expand(f, b)
	return rng.Filename, f, b, err
}

// modelFile returns the file a new block goes in: near is a node whose file is preferred if it
// is writable, else the first writable model file, else model.hcl.
func (s *state) modelFile(near string) string {
	p := s.p
	if n := p.Model.Nodes[near]; n != nil && !p.IsReadOnly(n.Range.Filename) {
		return n.Range.Filename
	}
	for _, f := range p.Files {
		if f.Role == project.RoleModel && !p.IsReadOnly(f.Path) {
			return f.Path
		}
	}
	return "model.hcl"
}

func (s *state) layoutPath(id string) string {
	return path.Join(s.layoutDir, id+".layout.yaml")
}

// layout returns the layout of view id for editing; a layout file with errors is refused,
// since what does not decode would be lost.
func (s *state) layout(id string) (*view.Layout, error) {
	name := s.layoutPath(id)
	for _, d := range s.before {
		if d.File == name && d.Severity == diag.Error && d.Summary != "Layout without view" {
			return nil, fmt.Errorf("%s has errors; fix them before applying operations to view %q", name, id)
		}
	}
	p, err := s.project()
	if err != nil {
		return nil, err
	}
	if l := p.Layouts[id]; l != nil {
		return l, nil
	}
	return &view.Layout{Place: map[string]*view.Place{}, EdgeHints: map[string]*view.Hint{}, LiftHints: map[[2]string]*view.Hint{}}, nil
}

// putLayout regenerates the layout file of view id, without entries that hold nothing,
// removing the file when l is empty.
func (s *state) putLayout(id string, l *view.Layout) {
	if s.p != nil {
		s.p.Layouts[id] = l
	}
	maps.DeleteFunc(l.Place, func(_ string, pl *view.Place) bool {
		return !pins(pl) && len(pl.Ports) == 0 && len(pl.Order) == 0
	})
	if len(l.Place) == 0 && len(l.EdgeHints) == 0 && len(l.LiftHints) == 0 && len(l.Decorations) == 0 {
		s.f.remove(s.layoutPath(id))
		return
	}
	s.f.put(s.layoutPath(id), l.Encode())
}

// editLayouts calls edit for the layout of every view, writing those it reports changed.
func (s *state) editLayouts(edit func(l *view.Layout) bool) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	for _, id := range slices.Sorted(maps.Keys(p.Layouts)) {
		if p.Views[id] == nil {
			continue
		}
		l, err := s.layout(id)
		if err != nil {
			return err
		}
		if edit(l) {
			s.putLayout(id, l)
		}
	}
	return nil
}

// viewsFile returns views.hcl for editing. Unless the project defines a view, the default view
// is written out first (spec 4.5), when withDefault.
func (s *state) viewsFile(withDefault bool) (string, *hclwrite.File, error) {
	p, err := s.project()
	if err != nil {
		return "", nil, err
	}
	f, err := s.hclFile(s.viewsPath)
	if err != nil {
		return "", nil, err
	}
	defined := slices.ContainsFunc(slices.Collect(maps.Values(p.Views)), func(v *view.View) bool { return v.Range.Filename != "" })
	if !defined && withDefault {
		b := hclwrite.NewBlock("view", []string{view.DefaultView})
		b.Body().SetAttributeRaw("include", hclwrite.TokensForTuple([]hclwrite.Tokens{hclwrite.TokensForTraversal(ref("all"))}))
		appendBlock(f, b)
	}
	return s.viewsPath, f, nil
}

// Outcome is an operation's outcome as dg op --json prints it and the MCP op tool returns it:
// its results, the files written and the warnings it added, or why it failed.
type Outcome struct {
	Results     []Result         `json:"results,omitempty"`
	Files       []string         `json:"files,omitempty"`
	Warnings    diag.Diagnostics `json:"warnings,omitempty"`
	Error       string           `json:"error,omitempty"`
	Diagnostics diag.Diagnostics `json:"diagnostics,omitempty"`
}

// NewOutcome returns the outcome of an operation that returned c and err.
func NewOutcome(c *Change, err error) Outcome {
	if err != nil {
		o := Outcome{Error: err.Error()}
		var refused *RefusedError
		if errors.As(err, &refused) {
			o.Diagnostics = refused.Diagnostics
		}
		return o
	}
	return Outcome{Results: c.Results, Files: c.Paths(), Warnings: c.Warnings}
}

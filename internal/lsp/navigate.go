package lsp

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/nvcnvn/dg/internal/ops"
)

func (s *Server) hover(l *loaded, rel string, pos Position) (any, error) {
	o, ok := l.index().at(rel, pos)
	n := s.parsed(l).Model.Nodes[o.id]
	if !ok || o.typ != symNode || n == nil {
		return nil, nil
	}
	value := fmt.Sprintf("node %q, kind %q\n\n%s", n.ID, n.Kind, n.Label)
	return hover{Contents: markupContent{Kind: "plaintext", Value: value}, Range: o.rng}, nil
}

func (s *Server) definition(l *loaded, rel string, pos Position) (any, error) {
	o, ok := l.index().at(rel, pos)
	if !ok {
		return nil, nil
	}
	for _, d := range l.index().of(o.typ, o.id) {
		if d.def {
			return Location{URI: s.uri(d.file), Range: d.rng}, nil
		}
	}
	return nil, nil
}

func (s *Server) references(l *loaded, rel string, pos Position, withDef bool) (any, error) {
	out := []Location{}
	o, ok := l.index().at(rel, pos)
	if !ok {
		return out, nil
	}
	for _, r := range l.index().of(o.typ, o.id) {
		if withDef || !r.def {
			out = append(out, Location{URI: s.uri(r.file), Range: r.rng})
		}
	}
	return out, nil
}

// renamable returns the node, edge or view ID at pos, which rename_id renames, unless it is
// defined in a read-only file.
func renamable(l *loaded, rel string, pos Position) (occurrence, error) {
	o, ok := l.index().at(rel, pos)
	if !ok || o.typ != symNode && o.typ != symEdge && o.typ != symView {
		return occurrence{}, &rpcError{codeRequestFailed, "dg renames node, edge and view IDs; there is none here"}
	}
	for _, d := range l.index().of(o.typ, o.id) {
		if d.def && l.p.IsReadOnly(d.file) {
			return occurrence{}, &rpcError{codeRequestFailed, fmt.Sprintf("%s %q is defined in %s, which is read-only (read_only in dg.hcl)", o.typ, o.id, d.file)}
		}
	}
	return o, nil
}

func (s *Server) prepareRename(l *loaded, rel string, pos Position) (any, error) {
	o, err := renamable(l, rel, pos)
	if err != nil {
		return nil, err
	}
	return prepareRenameResult{Range: o.rng, Placeholder: o.id}, nil
}

// rename plans rename_id on the buffers over the files and returns what it would write as one
// workspace edit; refused as the operation is, for a read-only file among them.
func (s *Server) rename(l *loaded, rel string, pos Position, newID string) (any, error) {
	o, err := renamable(l, rel, pos)
	if err != nil {
		return nil, err
	}
	fsys, done, err := s.fsys()
	if err != nil {
		return nil, err
	}
	defer done()
	c, err := ops.Plan(fsys, s.dir, ops.Op{Op: "rename_id", Type: o.typ, Old: o.id, New: newID})
	if err != nil {
		return nil, err
	}
	edit := workspaceEdit{DocumentChanges: []any{}}
	renamed := map[string]bool{}
	for _, f := range c.Files {
		if f.After != nil {
			continue
		}
		for _, g := range c.Files {
			if g.Before == nil && bytes.Equal(g.After, f.Before) && !renamed[g.Path] {
				edit.DocumentChanges = append(edit.DocumentChanges, fileOperation{Kind: "rename", OldURI: s.uri(f.Path), NewURI: s.uri(g.Path)})
				renamed[f.Path], renamed[g.Path] = true, true
				break
			}
		}
	}
	for _, f := range c.Files {
		switch {
		case renamed[f.Path]:
		case f.After == nil:
			edit.DocumentChanges = append(edit.DocumentChanges, fileOperation{Kind: "delete", URI: s.uri(f.Path)})
		case f.Before == nil:
			edit.DocumentChanges = append(edit.DocumentChanges, fileOperation{Kind: "create", URI: s.uri(f.Path)},
				s.documentEdit(f.Path, "", string(f.After)))
		default:
			edit.DocumentChanges = append(edit.DocumentChanges, s.documentEdit(f.Path, string(f.Before), string(f.After)))
		}
	}
	return edit, nil
}

func (s *Server) documentEdit(rel, before, after string) textDocumentEdit {
	var e textDocumentEdit
	e.TextDocument.URI = s.uri(rel)
	if d := s.docs[rel]; d != nil {
		v := d.version
		e.TextDocument.Version = &v
	}
	e.Edits = edits(before, after)
	return e
}

// format formats a project file through dg fmt's code: nothing for a read-only file, a file the
// project does not load, or one that does not parse, and an error while dg.hcl has errors, as
// dg fmt refuses then.
func (s *Server) format(l *loaded, rel string) (any, error) {
	if l.p.ConfigInvalid {
		return nil, ops.ErrConfigInvalid
	}
	out := []TextEdit{}
	for _, f := range l.p.Files {
		if f.Path != rel || l.p.IsReadOnly(rel) {
			continue
		}
		text := l.texts[rel]
		if formatted, _ := ops.FormatFile(f, []byte(text)); formatted != nil {
			out = edits(text, string(formatted))
		}
	}
	return out, nil
}

// edits returns the edits that turn before into after: one per changed line when both have
// the same lines, as a rename leaves them, else one around everything that changed.
func edits(before, after string) []TextEdit {
	out := []TextEdit{}
	ls := newLines(before)
	bl, al := strings.SplitAfter(before, "\n"), strings.SplitAfter(after, "\n")
	if len(bl) != len(al) {
		bl, al = []string{before}, []string{after}
	}
	off := 0
	for i := range bl {
		b, a := bl[i], al[i]
		if b != a {
			p := commonPrefix(b, a)
			q := commonSuffix(b[p:], a[p:])
			out = append(out, TextEdit{
				Range:   ls.rng(off+p, off+len(b)-q),
				NewText: a[p : len(a)-q],
			})
		}
		off += len(b)
	}
	return out
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	for n > 0 && n < len(a) && !utf8Start(a[n]) {
		n--
	}
	return n
}

func commonSuffix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}
	for n > 0 && n < len(a) && !utf8Start(a[len(a)-n]) {
		n--
	}
	return n
}

func utf8Start(b byte) bool {
	return b&0xC0 != 0x80
}

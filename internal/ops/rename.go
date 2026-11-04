package ops

import (
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"

	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/view"
)

func (s *state) renameID(op Op) error {
	switch op.Type {
	case "node", "edge", "view":
	default:
		return fmt.Errorf("rename_id: type is node, edge or view, not %q", op.Type)
	}
	if err := s.rename(op.Type, op.Old, op.New); err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.New})
	return nil
}

// rename renames node, edge or view old to id everywhere (spec 6.4).
func (s *state) rename(typ, old, id string) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	switch typ {
	case "node":
		if p.Model.Nodes[old] == nil {
			return fmt.Errorf("node %q does not exist", old)
		}
		if err := checkNewID("node", id, p.Model.Nodes[id] != nil); err != nil {
			return err
		}
		delete(s.sess.kindIDs, old)
		if err := s.editLayouts(func(l *view.Layout) bool { return renameNode(l, old, id) }); err != nil {
			return err
		}
		if err := s.renameInViews([]string{"node", "children", "descendants"}, "style", "node.", old, id); err != nil {
			return err
		}
		if err := s.renameInViews(nil, "node", "", old, id); err != nil {
			return err
		}
		return s.renameInModel(p, "node", []string{"node"}, old, id)
	case "edge":
		if p.Model.Edges[old] == nil {
			return fmt.Errorf("edge %q does not exist", old)
		}
		if err := checkNewID("edge", id, p.Model.Edges[id] != nil); err != nil {
			return err
		}
		err := s.editLayouts(func(l *view.Layout) bool {
			h := l.EdgeHints[old]
			if h == nil {
				return false
			}
			delete(l.EdgeHints, old)
			l.EdgeHints[id] = h
			return true
		})
		if err != nil {
			return err
		}
		if err := s.renameInViews([]string{"edge"}, "edge_style", "edge.", old, id); err != nil {
			return err
		}
		return s.renameInModel(p, "edge", nil, old, id)
	}
	if p.Views[old] == nil {
		return fmt.Errorf("view %q does not exist", old)
	}
	if d := spec.CheckSimple("view", id, hcl.Range{}); d.HasErrors() {
		return fmt.Errorf("%s", d[0].Detail)
	}
	if p.Views[id] != nil {
		return fmt.Errorf("view %q already exists", id)
	}
	name, f, err := s.viewsFile(true)
	if err != nil {
		return err
	}
	b := findBlock(f.Body(), "view", []string{old})
	if b == nil {
		return fmt.Errorf("view %q is not a block of %s", old, name)
	}
	setLabel(b, id)
	s.putHCL(name, f)
	from, to := s.layoutPath(old), s.layoutPath(id)
	if data, ok := s.f.get(from); ok {
		if _, exists := s.f.get(to); exists {
			return fmt.Errorf("%s already exists", to)
		}
		s.f.put(to, data)
		s.f.remove(from)
	}
	return nil
}

// renameNode renames node old to id in the keys and values of l.
func renameNode(l *view.Layout, old, id string) bool {
	changed := false
	if pl := l.Place[old]; pl != nil {
		delete(l.Place, old)
		l.Place[id] = pl
		changed = true
	}
	for _, pl := range l.Place {
		if pl.In == old {
			pl.In = id
			changed = true
		}
		for i, o := range pl.Order {
			if o == old {
				pl.Order[i] = id
				changed = true
			}
		}
	}
	for _, k := range slices.Collect(maps.Keys(l.LiftHints)) {
		nk := k
		for i := range nk {
			if nk[i] == old {
				nk[i] = id
			}
		}
		if nk != k {
			l.LiftHints[nk] = l.LiftHints[k]
			delete(l.LiftHints, k)
			changed = true
		}
	}
	for _, d := range l.Decorations {
		for _, ref := range []*string{&d.In, &d.Attach} {
			if *ref == old {
				*ref = id
				changed = true
			}
		}
	}
	return changed
}

// renameInViews renames references under roots, and the rules of type rule labeled prefix+old,
// in views.hcl.
func (s *state) renameInViews(roots []string, rule, prefix, old, id string) error {
	name, f, err := s.viewsFile(false)
	if err != nil {
		return err
	}
	changed := false
	for _, b := range blocks(f.Body(), "view") {
		changed = renameIn(b.Body(), roots, old, id) || changed
		for _, r := range blocks(b.Body(), rule) {
			if labels := labelsOf(r); len(labels) == 1 && labels[0] == prefix+old {
				setLabel(r, prefix+id)
				changed = true
			}
		}
	}
	if changed {
		s.putHCL(name, f)
	}
	return nil
}

// renameInModel renames the block of type typ labeled old, and references under roots, in every
// model file. It fails when no block was renamed.
func (s *state) renameInModel(p *project.Project, typ string, roots []string, old, id string) error {
	renamed := false
	for _, mf := range p.Files {
		if mf.Role != project.RoleModel {
			continue
		}
		f, err := s.hclFile(mf.Path)
		if err != nil {
			return err
		}
		changed := false
		for _, b := range f.Body().Blocks() {
			bc := false
			if labels := labelsOf(b); b.Type() == typ && len(labels) == 1 && labels[0] == old {
				setLabel(b, id)
				bc, renamed = true, true
			}
			if len(roots) > 0 && renameIn(b.Body(), roots, old, id) {
				bc = true
			}
			changed = changed || bc
		}
		if changed {
			s.putHCL(mf.Path, f)
		}
	}
	if !renamed {
		return fmt.Errorf("the block of %s %q was not found to rename; it is written in a way dg cannot edit", typ, old)
	}
	return nil
}

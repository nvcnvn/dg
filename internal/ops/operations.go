package ops

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/view"
)

// maxCoord bounds coordinates and sizes, far beyond any drawing, so arithmetic cannot overflow.
const maxCoord = 1_000_000_000

var pinSides = []string{"top", "right", "bottom", "left"}

func (s *state) apply(op Op) error {
	switch op.Op {
	case "batch":
		for i, o := range op.Ops {
			if err := s.apply(o); err != nil {
				return fmt.Errorf("operation %d (%s): %w", i+1, o.Op, err)
			}
		}
		return nil
	case "create_node":
		return s.createNode(op)
	case "delete_node":
		return s.deleteNode(op)
	case "set_label":
		return s.setLabel(op)
	case "set_parent":
		return s.setParent(op)
	case "set_kind":
		return s.setKind(op)
	case "set_tags":
		return s.setTags(op)
	case "set_props":
		return s.setProps(op)
	case "add_port":
		return s.addPort(op)
	case "remove_port":
		return s.removePort(op)
	case "set_rows":
		return s.setRows(op)
	case "connect":
		return s.connect(op)
	case "disconnect":
		return s.disconnect(op)
	case "set_edge_labels":
		return s.setEdgeLabels(op)
	case "move":
		return s.move(op)
	case "resize":
		return s.resize(op)
	case "reorder":
		return s.reorder(op)
	case "unpin":
		return s.unpin(op)
	case "pin_port":
		return s.pinPort(op)
	case "set_edge_hint":
		return s.setEdgeHint(op)
	case "rename_id":
		return s.renameID(op)
	case "create_view":
		return s.createView(op)
	case "update_view":
		return s.updateView(op)
	case "delete_view":
		return s.deleteView(op)
	case "show_node":
		return s.showExisting(op)
	case "hide_node":
		return s.hideNode(op)
	case "add_decoration":
		return s.addDecoration(op)
	case "update_decoration":
		return s.updateDecoration(op)
	case "delete_decoration":
		return s.deleteDecoration(op)
	case "promote_decoration":
		return s.promoteDecoration(op)
	}
	return fmt.Errorf("unknown operation %q", op.Op)
}

// checkNewID reports why id cannot be the ID of a new node or edge.
func checkNewID(typ, id string, exists bool) error {
	if d := spec.CheckID(typ, id, hcl.Range{}); d.HasErrors() {
		return fmt.Errorf("%s", d[0].Detail)
	}
	if exists {
		return fmt.Errorf("%s %q already exists", typ, id)
	}
	return nil
}

func (s *state) createNode(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	if op.Kind == "" {
		return fmt.Errorf("create_node needs a kind")
	}
	if (op.X == nil) != (op.Y == nil) || op.X != nil && op.View == "" {
		return fmt.Errorf("create_node takes x and y together, with view")
	}
	if op.View != "" && p.Views[op.View] == nil {
		return fmt.Errorf("view %q does not exist", op.View)
	}
	label := ""
	if op.Label != nil {
		label = *op.Label
	}
	id := op.ID
	if id != "" {
		if err := checkNewID("node", id, p.Model.Nodes[id] != nil); err != nil {
			return err
		}
	} else {
		var fromKind bool
		id, fromKind = nodeID(op.Kind, label, func(x string) bool { return p.Model.Nodes[x] != nil })
		if fromKind {
			s.sess.kindIDs[id] = true
		}
	}
	name := s.modelFile(op.Parent)
	f, err := s.hclFile(name)
	if err != nil {
		return err
	}
	b := hclwrite.NewBlock("node", []string{id})
	if op.Kind != spec.RootKind {
		b.Body().SetAttributeValue("kind", cty.StringVal(op.Kind))
	}
	if op.Label != nil {
		b.Body().SetAttributeValue("label", cty.StringVal(label))
	}
	if op.Parent != "" {
		b.Body().SetAttributeTraversal("parent", ref("node", op.Parent))
	}
	appendBlock(f, b)
	s.putHCL(name, f)
	if p, err = s.project(); err != nil {
		return err
	}
	if p.Model.Nodes[id] == nil {
		return fmt.Errorf("%s is not a model file of the project (models in dg.hcl); add one before creating nodes", name)
	}
	r := Result{Op: op.Op, ID: id}
	if op.Before != "" && op.View == "" {
		return fmt.Errorf("create_node takes before with view")
	}
	if op.View != "" {
		if err := s.showNode(op.View, id); err != nil {
			return err
		}
		if op.X != nil {
			if r.X, r.Y, err = s.place(op.View, id, *op.X, *op.Y); err != nil {
				return err
			}
		}
		if err := s.insert(op.View, id, op.Before); err != nil {
			return err
		}
	}
	s.result(r)
	return nil
}

// insert puts node id in the order of the stack or grid container that places it in view v,
// before sibling before, or last when before is empty. A node placed freely takes no before.
func (s *state) insert(v, id, before string) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	vw := p.Views[v]
	visible, _ := vw.Visible(p.Spec, p.Model)
	c := p.PlacedBy(vw, visible, id)
	if c == "" {
		if before != "" {
			return fmt.Errorf("view %q places node %q freely, not in a stack or grid container's order; before does not apply", v, id)
		}
		return nil
	}
	r, _ := p.Sources.Resolve(v)
	var policy *view.Policy
	if r.Policy != nil && r.Policy.Container == c {
		policy = r.Policy
	}
	for _, n := range r.Nodes {
		if n.ID == c {
			policy = n.Policy
		}
	}
	order := slices.DeleteFunc(slices.Clone(policy.Order), func(o string) bool { return o == id })
	at := len(order)
	if before != "" {
		if at = slices.Index(order, before); at < 0 {
			return fmt.Errorf("node %q is not a sibling of node %q placed by %q in view %q", before, id, c, v)
		}
	}
	l, err := s.layout(v)
	if err != nil {
		return err
	}
	pl := l.Place[c]
	if pl == nil {
		pl = &view.Place{}
		l.Place[c] = pl
	}
	pl.Order = slices.Insert(order, at, id)
	s.putLayout(v, l)
	return nil
}

// showExisting makes view v show existing node id: it removes the selectors naming it, its
// children or its descendants from the view's exclude, then adds node.<id> to its include if
// needed.
func (s *state) showExisting(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	switch {
	case p.Views[op.View] == nil:
		return fmt.Errorf("view %q does not exist", op.View)
	case p.Model.Nodes[op.ID] == nil:
		return fmt.Errorf("node %q does not exist", op.ID)
	}
	if visible, _ := p.Views[op.View].Visible(p.Spec, p.Model); !visible[op.ID] {
		name, f, b, err := s.viewBlock(op.View)
		if err != nil {
			return err
		}
		if dropSelectors(b.Body(), "exclude", func(sel view.Selector) bool {
			return sel.Target == op.ID && slices.Contains([]string{"node", "children", "descendants"}, sel.Form)
		}) {
			if l, ok := splitList(b.Body().GetAttribute("exclude").Expr().BuildTokens(nil)); ok && len(l.items) == 0 {
				removeAttr(b.Body(), "exclude")
			}
			s.putHCL(name, f)
		}
		if err := s.showNode(op.View, op.ID); err != nil {
			return err
		}
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// hideNode makes view v stop showing node id and the nodes inside it: it removes from the view's
// include the selectors naming them and those of id's children and descendants, then adds
// node.<id>, and descendants.<id> if needed, to its exclude for what the view still shows.
func (s *state) hideNode(op Op) error {
	if _, _, err := s.shown(op.View, op.ID); err != nil {
		return err
	}
	p := s.p
	inside := func(id string) bool { return id == op.ID || p.Model.Contains(op.ID, id) }
	name, f, b, err := s.viewBlock(op.View)
	if err != nil {
		return err
	}
	if dropSelectors(b.Body(), "include", func(sel view.Selector) bool {
		return sel.Form == "node" && inside(sel.Target) || (sel.Form == "children" || sel.Form == "descendants") && sel.Target == op.ID
	}) {
		s.putHCL(name, f)
	}
	if p, err = s.project(); err != nil {
		return err
	}
	visible, _ := p.Views[op.View].Visible(p.Spec, p.Model)
	var add []hcl.Traversal
	if visible[op.ID] {
		add = append(add, ref("node", op.ID))
	}
	if slices.ContainsFunc(slices.Sorted(maps.Keys(visible)), func(id string) bool { return id != op.ID && p.Model.Contains(op.ID, id) }) {
		add = append(add, ref("descendants", op.ID))
	}
	for _, sel := range add {
		name, f, b, err := s.viewBlock(op.View)
		if err != nil {
			return err
		}
		if err := addSelector(b.Body(), "exclude", nil, sel); err != nil {
			return fmt.Errorf("view %q: %w", op.View, err)
		}
		s.putHCL(name, f)
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// showNode makes view v show node id, adding node.<id> to its include if needed.
func (s *state) showNode(v, id string) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	vw := p.Views[v]
	if visible, _ := vw.Visible(p.Spec, p.Model); visible[id] {
		return nil
	}
	name, f, err := s.viewsFile(true)
	if err != nil {
		return err
	}
	b := findBlock(f.Body(), "view", []string{v})
	if b == nil {
		return fmt.Errorf("view %q is not a block of %s", v, name)
	}
	if f, b, err = expand(f, b); err != nil {
		return err
	}
	def := []hcl.Traversal{ref("all")}
	if vw.Of != nil {
		def = []hcl.Traversal{ref("descendants", vw.Of.Target)}
	}
	if err := addSelector(b.Body(), "include", def, ref("node", id)); err != nil {
		return fmt.Errorf("view %q: %w", v, err)
	}
	s.putHCL(name, f)
	if p, err = s.project(); err != nil {
		return err
	}
	if visible, _ := p.Views[v].Visible(p.Spec, p.Model); !visible[id] {
		return fmt.Errorf("view %q does not show node %q even with node.%s in its include: it is excluded or inside a collapsed container", v, id, id)
	}
	return nil
}

func (s *state) deleteNode(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	n := p.Model.Nodes[op.ID]
	if n == nil {
		return fmt.Errorf("node %q does not exist", op.ID)
	}
	del := map[string]bool{op.ID: true}
	var children []string
	for _, id := range slices.Sorted(maps.Keys(p.Model.Nodes)) {
		switch c := p.Model.Nodes[id]; {
		case op.Recursive && p.Model.Contains(op.ID, id):
			del[id] = true
		case c.Parent != nil && c.Parent.Node == op.ID:
			children = append(children, id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Views)) {
		if v := p.Views[id]; v.Of != nil && del[v.Of.Target] {
			return fmt.Errorf("view %q has of = node.%s; change or delete the view first", id, v.Of.Target)
		}
	}
	edges := map[string]bool{}
	for id, e := range p.Model.Edges {
		if del[e.From.Node] || del[e.To.Node] {
			edges[id] = true
		}
	}
	err = s.editLayouts(func(l *view.Layout) bool {
		changed := false
		for id, pl := range l.Place {
			switch {
			case del[id] || slices.Contains(children, id):
				delete(l.Place, id)
				changed = true
			case slices.ContainsFunc(pl.Order, func(o string) bool { return del[o] }):
				pl.Order = slices.DeleteFunc(pl.Order, func(o string) bool { return del[o] })
				changed = true
			}
		}
		return dropHints(l, edges, del) || changed
	})
	if err != nil {
		return err
	}
	if err := s.dropViewRefs(del, edges); err != nil {
		return err
	}
	var parent hcl.Traversal
	if n.Parent != nil {
		parent = ref("node", n.Parent.Node)
	}
	if err := s.editModel(p, del, edges, func(b *hclwrite.Block) bool {
		if b.Type() != "node" || len(labelsOf(b)) != 1 || !slices.Contains(children, labelsOf(b)[0]) {
			return false
		}
		if parent == nil {
			removeAttr(b.Body(), "parent")
		} else {
			setTraversal(b.Body(), "parent", parent)
		}
		return true
	}); err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// dropHints removes from l the hints of edges and the decorations and lifted-edge hints that
// name a node of nodes.
func dropHints(l *view.Layout, edges, nodes map[string]bool) bool {
	changed := false
	for id := range l.EdgeHints {
		if edges[id] {
			delete(l.EdgeHints, id)
			changed = true
		}
	}
	for k := range l.LiftHints {
		if nodes[k[0]] || nodes[k[1]] {
			delete(l.LiftHints, k)
			changed = true
		}
	}
	n := len(l.Decorations)
	l.Decorations = slices.DeleteFunc(l.Decorations, func(d *view.Decoration) bool { return nodes[d.In] || nodes[d.Attach] })
	return changed || len(l.Decorations) != n
}

// editModel removes the blocks of nodes and edges from the model files and calls edit, if set,
// on every other top-level block, formatting those it reports changed.
func (s *state) editModel(p *project.Project, nodes, edges map[string]bool, edit func(*hclwrite.Block) bool) error {
	for _, mf := range p.Files {
		if mf.Role != project.RoleModel {
			continue
		}
		f, err := s.hclFile(mf.Path)
		if err != nil {
			return err
		}
		var rm []*hclwrite.Block
		changed := false
		for _, b := range f.Body().Blocks() {
			if len(labelsOf(b)) != 1 {
				continue
			}
			id := labelsOf(b)[0]
			switch {
			case b.Type() == "node" && nodes[id], b.Type() == "edge" && edges[id]:
				rm = append(rm, b)
			case edit != nil && edit(b):
				changed = true
			}
		}
		if len(rm) > 0 {
			if f, err = removeBlocks(f, rm); err != nil {
				return err
			}
			changed = true
		}
		if changed {
			s.putHCL(mf.Path, f)
		}
	}
	return nil
}

// dropViewRefs removes from views.hcl the selectors and view style rules naming nodes or edges.
func (s *state) dropViewRefs(nodes, edges map[string]bool) error {
	name, f, err := s.viewsFile(false)
	if err != nil {
		return err
	}
	dropNode := func(sel view.Selector) bool {
		return sel.Form != "kind" && sel.Form != "tag" && sel.Form != "all" && nodes[sel.Target]
	}
	changed := false
	var rules []*hclwrite.Block
	for _, b := range blocks(f.Body(), "view") {
		for _, attr := range []string{"include", "exclude", "collapse"} {
			changed = dropSelectors(b.Body(), attr, dropNode) || changed
		}
		changed = dropSelectors(b.Body(), "edges", func(sel view.Selector) bool { return sel.Form == "edge" && edges[sel.Target] }) || changed
		for _, o := range blocks(b.Body(), "overlay") {
			changed = dropSelectors(o.Body(), "members", dropNode) || changed
		}
		for _, r := range b.Body().Blocks() {
			labels := labelsOf(r)
			if len(labels) != 1 {
				continue
			}
			id, isNode := strings.CutPrefix(labels[0], "node.")
			id2, isEdge := strings.CutPrefix(labels[0], "edge.")
			if r.Type() == "style" && isNode && nodes[id] || r.Type() == "edge_style" && isEdge && edges[id2] {
				rules = append(rules, r)
			}
		}
	}
	if len(rules) > 0 {
		if f, err = removeBlocks(f, rules); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		s.putHCL(name, f)
	}
	return nil
}

// editBlock calls edit on the body of the block of node or edge id and writes its file, with
// the attributes edit added before the block's nested blocks.
func (s *state) editBlock(typ, id string, edit func(*hclwrite.Body) error) error {
	name, f, b, err := s.block(typ, id)
	if err != nil {
		return err
	}
	had := sortedAttrs(b.Body())
	if err := edit(b.Body()); err != nil {
		return err
	}
	added := slices.DeleteFunc(sortedAttrs(b.Body()), func(a string) bool { return slices.Contains(had, a) })
	if f, err = attrsBeforeBlocks(f, typ, []string{id}, added); err != nil {
		return err
	}
	s.putHCL(name, f)
	return nil
}

func (s *state) setLabel(op Op) error {
	label := ""
	if op.Label != nil {
		label = *op.Label
	}
	err := s.editBlock("node", op.ID, func(b *hclwrite.Body) error {
		setValue(b, "label", cty.StringVal(label))
		return nil
	})
	if err != nil {
		return err
	}
	id := op.ID
	if s.sess.kindIDs[id] && slug(label) != "" {
		delete(s.sess.kindIDs, id)
		p, err := s.project()
		if err != nil {
			return err
		}
		if next := newID(slug(label), func(x string) bool { return x != id && p.Model.Nodes[x] != nil }); next != id {
			if err := s.rename("node", id, next); err != nil {
				return err
			}
			id = next
		}
	}
	s.result(Result{Op: op.Op, ID: id})
	return nil
}

func (s *state) setParent(op Op) error {
	err := s.editBlock("node", op.ID, func(b *hclwrite.Body) error {
		if op.Parent == "" {
			removeAttr(b, "parent")
		} else {
			setTraversal(b, "parent", ref("node", op.Parent))
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func elementType(op Op) (string, error) {
	switch op.Type {
	case "", "node":
		return "node", nil
	case "edge":
		return "edge", nil
	}
	return "", fmt.Errorf("%s: type is node or edge, not %q", op.Op, op.Type)
}

func (s *state) setKind(op Op) error {
	typ, err := elementType(op)
	if err != nil {
		return err
	}
	if op.Kind == "" {
		return fmt.Errorf("set_kind needs a kind")
	}
	if err := s.editBlock(typ, op.ID, func(b *hclwrite.Body) error {
		setValue(b, "kind", cty.StringVal(op.Kind))
		return nil
	}); err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func (s *state) setTags(op Op) error {
	typ, err := elementType(op)
	if err != nil {
		return err
	}
	if err := s.editBlock(typ, op.ID, func(b *hclwrite.Body) error {
		if len(op.Tags) == 0 {
			removeAttr(b, "tags")
			return nil
		}
		var vals []cty.Value
		for _, t := range op.Tags {
			vals = append(vals, cty.StringVal(t))
		}
		setValue(b, "tags", cty.TupleVal(vals))
		return nil
	}); err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// propValue converts a property value from JSON.
func propValue(name string, v any) (cty.Value, error) {
	switch v := v.(type) {
	case string:
		return cty.StringVal(v), nil
	case bool:
		return cty.BoolVal(v), nil
	case json.Number:
		n, err := cty.ParseNumberVal(v.String())
		if err == nil && !n.IsKnown() {
			err = fmt.Errorf("not a number")
		}
		return n, err
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			break
		}
		return cty.NumberFloatVal(v), nil
	case int:
		return cty.NumberIntVal(int64(v)), nil
	}
	return cty.NilVal, fmt.Errorf("property %q: a value is a string, number or boolean", name)
}

func (s *state) setProps(op Op) error {
	props := map[string]cty.Value{}
	for _, k := range slices.Sorted(maps.Keys(op.Props)) {
		v, err := propValue(k, op.Props[k])
		if err != nil {
			return err
		}
		props[k] = v
	}
	if err := s.editBlock("node", op.ID, func(b *hclwrite.Body) error {
		if len(props) == 0 {
			removeAttr(b, "props")
		} else {
			setValue(b, "props", cty.ObjectVal(props))
		}
		return nil
	}); err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func (s *state) addPort(op Op) error {
	if problem := spec.NameProblem(op.Name); problem != "" {
		return fmt.Errorf("the port name %q %s", op.Name, problem)
	}
	if err := s.editBlock("node", op.ID, func(b *hclwrite.Body) error {
		port := hclwrite.NewBlock("port", []string{op.Name})
		if op.Side != "" {
			port.Body().SetAttributeValue("side", cty.StringVal(op.Side))
		}
		appendNested(b, port)
		return nil
	}); err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func (s *state) removePort(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	n := p.Model.Nodes[op.ID]
	if n == nil {
		return fmt.Errorf("node %q does not exist", op.ID)
	}
	err = s.editLayouts(func(l *view.Layout) bool {
		if pl := l.Place[op.ID]; pl != nil && pl.Ports[op.Name] != nil {
			delete(pl.Ports, op.Name)
			return true
		}
		return false
	})
	if err != nil {
		return err
	}
	name, f, b, err := s.block("node", op.ID)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(blocks(b.Body(), "port"), func(pb *hclwrite.Block) bool { return labelsOf(pb)[0] == op.Name })
	switch {
	case i >= 0:
	case slices.Contains(p.Spec.Ports(n.Kind), op.Name):
		return fmt.Errorf("port %q of node %q comes from its kind %q; it is not an instance port", op.Name, op.ID, n.Kind)
	case slices.ContainsFunc(n.Ports, func(pt model.Port) bool { return pt.Name == op.Name }):
		return fmt.Errorf("port %q of node %q is declared by its ports attribute; edit that by hand", op.Name, op.ID)
	default:
		return fmt.Errorf("node %q has no port %q", op.ID, op.Name)
	}
	if f, err = removeBlocks(f, blocks(b.Body(), "port")[i:i+1]); err != nil {
		return err
	}
	s.putHCL(name, f)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func rowBlock(r view.Row) *hclwrite.Block {
	b := hclwrite.NewBlock("row", []string{r.Name})
	if r.Type != "" {
		b.Body().SetAttributeValue("type", cty.StringVal(r.Type))
	}
	if r.Key != "" {
		b.Body().SetAttributeValue("key", cty.StringVal(r.Key))
	}
	return b
}

func (s *state) setRows(op Op) error {
	for i, c := range op.Compartments {
		if c.Name == "" && len(op.Compartments) > 1 {
			return fmt.Errorf("compartment %d has no name; rows declared directly in the node are the only compartment", i+1)
		}
	}
	name, f, b, err := s.block("node", op.ID)
	if err != nil {
		return err
	}
	old := slices.DeleteFunc(slices.Clone(b.Body().Blocks()), func(r *hclwrite.Block) bool { return r.Type() != "row" && r.Type() != "compartment" })
	if f, err = removeBlocks(f, old); err != nil {
		return err
	}
	body := findBlock(f.Body(), "node", []string{op.ID}).Body()
	for _, c := range op.Compartments {
		if c.Name == "" {
			for _, r := range c.Rows {
				appendNested(body, rowBlock(r))
			}
			continue
		}
		cb := hclwrite.NewBlock("compartment", []string{c.Name})
		for _, r := range c.Rows {
			cb.Body().AppendBlock(rowBlock(r))
		}
		appendNested(body, cb)
	}
	s.putHCL(name, f)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// setLabels writes an edge's labels: label for a middle label alone, else labels.
func setLabels(b *hclwrite.Body, l model.Labels) {
	removeAttr(b, "label")
	removeAttr(b, "labels")
	switch {
	case l == model.Labels{}:
	case l.Start == "" && l.End == "":
		setValue(b, "label", cty.StringVal(l.Mid))
	default:
		var attrs []hclwrite.ObjectAttrTokens
		for _, kv := range [][2]string{{"start", l.Start}, {"mid", l.Mid}, {"end", l.End}} {
			if kv[1] != "" {
				attrs = append(attrs, hclwrite.ObjectAttrTokens{Name: hclwrite.TokensForIdentifier(kv[0]), Value: hclwrite.TokensForValue(cty.StringVal(kv[1]))})
			}
		}
		setAttr(b, "labels", hclwrite.TokensForObject(attrs))
	}
}

func (s *state) connect(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	if op.From == nil || op.To == nil {
		return fmt.Errorf("connect needs from and to")
	}
	for _, e := range []*view.End{op.From, op.To} {
		if e.Port != "" && e.Row != "" {
			return fmt.Errorf("an edge end names a port or a row, not both")
		}
		if p.Model.Nodes[e.Node] == nil {
			return fmt.Errorf("node %q does not exist", e.Node)
		}
	}
	if op.View != "" && p.Views[op.View] == nil {
		return fmt.Errorf("view %q does not exist", op.View)
	}
	id := op.ID
	if id != "" {
		if err := checkNewID("edge", id, p.Model.Edges[id] != nil); err != nil {
			return err
		}
	} else {
		id = edgeID(op.From.Node, op.To.Node, func(x string) bool { return p.Model.Edges[x] != nil })
	}
	name := s.modelFile(op.From.Node)
	f, err := s.hclFile(name)
	if err != nil {
		return err
	}
	b := hclwrite.NewBlock("edge", []string{id})
	b.Body().SetAttributeTraversal("from", endRef(*op.From))
	b.Body().SetAttributeTraversal("to", endRef(*op.To))
	if op.Kind != "" {
		b.Body().SetAttributeValue("kind", cty.StringVal(op.Kind))
	}
	if op.Labels != nil {
		setLabels(b.Body(), *op.Labels)
	}
	if len(op.Tags) > 0 {
		var vals []cty.Value
		for _, t := range op.Tags {
			vals = append(vals, cty.StringVal(t))
		}
		b.Body().SetAttributeValue("tags", cty.TupleVal(vals))
	}
	appendBlock(f, b)
	s.putHCL(name, f)
	if p, err = s.project(); err != nil {
		return err
	}
	if p.Model.Edges[id] == nil {
		return fmt.Errorf("%s is not a model file of the project (models in dg.hcl); add one before connecting nodes", name)
	}
	if op.View != "" {
		if err := s.showEdge(op.View, id); err != nil {
			return err
		}
	}
	s.result(Result{Op: op.Op, ID: id})
	return nil
}

// showEdge makes view v show edge id, adding edge.<id> to its edges filter if needed.
func (s *state) showEdge(v, id string) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	vw := p.Views[v]
	visible, _ := vw.Visible(p.Spec, p.Model)
	e := p.Model.Edges[id]
	for _, n := range []string{e.From.Node, e.To.Node} {
		if !visible[n] {
			return fmt.Errorf("view %q does not show node %q, an end of edge %q", v, n, id)
		}
	}
	if vw.EdgesIn(p.Spec, p.Model)[id] {
		return nil
	}
	name, f, err := s.viewsFile(true)
	if err != nil {
		return err
	}
	b := findBlock(f.Body(), "view", []string{v})
	if b == nil {
		return fmt.Errorf("view %q is not a block of %s", v, name)
	}
	if f, b, err = expand(f, b); err != nil {
		return err
	}
	if err := addSelector(b.Body(), "edges", nil, ref("edge", id)); err != nil {
		return fmt.Errorf("view %q: %w", v, err)
	}
	s.putHCL(name, f)
	return nil
}

func (s *state) disconnect(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	if p.Model.Edges[op.ID] == nil {
		return fmt.Errorf("edge %q does not exist", op.ID)
	}
	edges := map[string]bool{op.ID: true}
	if err := s.editLayouts(func(l *view.Layout) bool { return dropHints(l, edges, nil) }); err != nil {
		return err
	}
	if err := s.dropViewRefs(nil, edges); err != nil {
		return err
	}
	if err := s.editModel(p, nil, edges, nil); err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func (s *state) setEdgeLabels(op Op) error {
	var l model.Labels
	if op.Labels != nil {
		l = *op.Labels
	}
	if err := s.editBlock("edge", op.ID, func(b *hclwrite.Body) error {
		setLabels(b, l)
		return nil
	}); err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// tokensOf returns the tokens of expression src.
func tokensOf(src string) (hclwrite.Tokens, error) {
	f, diags := hclwrite.ParseConfig([]byte("x = "+src+"\n"), "", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%q: %s", src, diags.Error())
	}
	return f.Body().GetAttribute("x").Expr().BuildTokens(nil), nil
}

func snap(v, grid int) int {
	return int(math.Round(float64(v)/float64(grid))) * grid
}

// size checks that a width or height is positive and rounds it to the grid, to at least one grid
// unit.
func size(v, grid int) (int, error) {
	if v <= 0 {
		return 0, fmt.Errorf("w and h are positive")
	}
	if err := checkCoords("w and h", v); err != nil {
		return 0, err
	}
	return max(grid, snap(v, grid)), nil
}

func checkCoords(names string, vs ...int) error {
	for _, v := range vs {
		if v < -maxCoord || v > maxCoord {
			return fmt.Errorf("%s must lie within ±%d", names, maxCoord)
		}
	}
	return nil
}

// shown returns view v and whether it shows node id.
func (s *state) shown(v, id string) (*view.View, map[string]bool, error) {
	p, err := s.project()
	if err != nil {
		return nil, nil, err
	}
	vw := p.Views[v]
	switch {
	case vw == nil:
		return nil, nil, fmt.Errorf("view %q does not exist", v)
	case p.Model.Nodes[id] == nil:
		return nil, nil, fmt.Errorf("node %q does not exist", id)
	}
	visible, _ := vw.Visible(p.Spec, p.Model)
	if !visible[id] {
		return nil, nil, fmt.Errorf("view %q does not show node %q", v, id)
	}
	return vw, visible, nil
}

// place pins node id in view v at x, y in its frame, snapped to the grid.
func (s *state) place(v, id string, x, y int) (*int, *int, error) {
	vw, visible, err := s.shown(v, id)
	if err != nil {
		return nil, nil, err
	}
	if err := checkCoords("x and y", x, y); err != nil {
		return nil, nil, err
	}
	p := s.p
	frame := view.Frame(p.Model, visible, id)
	if c := p.PlacedBy(vw, visible, id); c != "" {
		return nil, nil, fmt.Errorf("node %q is placed in order by %q, a %s container; reorder it instead", id, c, p.NodeStyle(vw, p.Model.Nodes[c])["layout"].Value.AsString())
	}
	l, err := s.layout(v)
	if err != nil {
		return nil, nil, err
	}
	pl := l.Place[id]
	if pl == nil {
		pl = &view.Place{}
		l.Place[id] = pl
	}
	x, y = snap(x, p.Grid), snap(y, p.Grid)
	pl.In, pl.X, pl.Y = frame, &x, &y
	s.putLayout(v, l)
	return &x, &y, nil
}

func (s *state) move(op Op) error {
	if op.X == nil || op.Y == nil {
		return fmt.Errorf("move needs x and y")
	}
	x, y, err := s.place(op.View, op.ID, *op.X, *op.Y)
	if err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: op.ID, X: x, Y: y})
	return nil
}

// pinned returns the layout of view v and node id's entry in it, which pins the node when it
// holds a position or size. A child of a stack or grid container has no position to pin, so its
// entry is made when it has none.
func (s *state) pinned(v, id string) (*view.Layout, *view.Place, error) {
	vw, visible, err := s.shown(v, id)
	if err != nil {
		return nil, nil, err
	}
	l, err := s.layout(v)
	if err != nil {
		return nil, nil, err
	}
	pl := l.Place[id]
	if s.p.PlacedBy(vw, visible, id) != "" {
		if pl == nil {
			pl = &view.Place{}
			l.Place[id] = pl
		}
		return l, pl, nil
	}
	if pl == nil || !pins(pl) {
		return nil, nil, fmt.Errorf("node %q is not pinned in view %q; move it first (dg op has no geometry: read drawn positions with dg export --format json)", id, v)
	}
	return l, pl, nil
}

// pins reports whether a layout entry pins its node: whether it holds a position or size,
// rather than only a container's order.
func pins(pl *view.Place) bool {
	return pl.X != nil || pl.Y != nil || pl.W != nil || pl.H != nil
}

func (s *state) resize(op Op) error {
	if op.W == nil || op.H == nil {
		return fmt.Errorf("resize needs w and h")
	}
	l, pl, err := s.pinned(op.View, op.ID)
	if err != nil {
		return err
	}
	w, err := size(*op.W, s.p.Grid)
	if err != nil {
		return err
	}
	h, err := size(*op.H, s.p.Grid)
	if err != nil {
		return err
	}
	pl.W, pl.H = &w, &h
	s.putLayout(op.View, l)
	s.result(Result{Op: op.Op, ID: op.ID, W: &w, H: &h})
	return nil
}

func (s *state) unpin(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	if p.Views[op.View] == nil {
		return fmt.Errorf("view %q does not exist", op.View)
	}
	l, err := s.layout(op.View)
	if err != nil {
		return err
	}
	pl := l.Place[op.ID]
	if pl == nil || !pins(pl) && len(pl.Ports) == 0 {
		return fmt.Errorf("node %q is not pinned in view %q", op.ID, op.View)
	}
	l.Place[op.ID] = &view.Place{Order: pl.Order}
	s.putLayout(op.View, l)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// reorder sets the order list of a stack or grid container in a view: one drawn expanded, or
// the view's of, whose policy places its children on the canvas. An empty order removes the
// list from any node, so a stale one can be cleared.
func (s *state) reorder(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	vw := p.Views[op.View]
	switch {
	case vw == nil:
		return fmt.Errorf("view %q does not exist", op.View)
	case p.Model.Nodes[op.Container] == nil:
		return fmt.Errorf("node %q does not exist", op.Container)
	}
	visible, collapsed := vw.Visible(p.Spec, p.Model)
	c := op.Container
	switch {
	case len(op.Order) == 0:
	case !visible[c] && p.PolicyContainer(vw, visible, "") == c:
	case !visible[c]:
		return fmt.Errorf("view %q does not show node %q", op.View, c)
	case collapsed[c]:
		return fmt.Errorf("view %q draws node %q collapsed", op.View, c)
	case p.PolicyContainer(vw, visible, c) == "":
		return fmt.Errorf("node %q is not a stack or grid container in view %q", c, op.View)
	}
	seen := map[string]bool{}
	for _, id := range op.Order {
		switch {
		case seen[id]:
			return fmt.Errorf("node %q is in the order twice", id)
		case !visible[id] || p.PlacedBy(vw, visible, id) != c:
			return fmt.Errorf("node %q is not a child of %q drawn in view %q", id, c, op.View)
		}
		seen[id] = true
	}
	l, err := s.layout(op.View)
	if err != nil {
		return err
	}
	s.result(Result{Op: op.Op, ID: c})
	pl := l.Place[c]
	if pl == nil {
		if len(op.Order) == 0 {
			return nil
		}
		pl = &view.Place{}
		l.Place[c] = pl
	}
	pl.Order = slices.Clone(op.Order)
	s.putLayout(op.View, l)
	return nil
}

// fraction checks that v is between 0 and 1 and keeps it to two decimals, as layout files do.
func fraction(name string, v *float64) (*float64, error) {
	if v == nil {
		return nil, nil
	}
	if !(*v >= 0 && *v <= 1) {
		return nil, fmt.Errorf("%s is between 0 and 1", name)
	}
	r := math.Round(*v*100) / 100
	return &r, nil
}

func (s *state) pinPort(op Op) error {
	if !slices.Contains(pinSides, op.Side) {
		return fmt.Errorf("side is one of %s", strings.Join(pinSides, ", "))
	}
	offset, err := fraction("offset", op.Offset)
	if err != nil {
		return err
	}
	l, pl, err := s.pinned(op.View, op.ID)
	if err != nil {
		return err
	}
	if !slices.Contains(s.p.Model.Ports(s.p.Spec, s.p.Model.Nodes[op.ID]), op.Port) {
		return fmt.Errorf("node %q has no port %q", op.ID, op.Port)
	}
	if pl.Ports == nil {
		pl.Ports = map[string]*view.PortPin{}
	}
	pl.Ports[op.Port] = &view.PortPin{Side: op.Side, Offset: offset}
	s.putLayout(op.View, l)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func (s *state) setEdgeHint(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	vw := p.Views[op.View]
	if vw == nil {
		return fmt.Errorf("view %q does not exist", op.View)
	}
	var h view.Hint
	if op.Hint != nil {
		h = view.Hint{FromSide: op.Hint.FromSide, ToSide: op.Hint.ToSide}
		for _, side := range []string{h.FromSide, h.ToSide} {
			if side != "" && !slices.Contains(pinSides, side) {
				return fmt.Errorf("a side is one of %s", strings.Join(pinSides, ", "))
			}
		}
		if h.LabelPos, err = fraction("label_pos", op.Hint.LabelPos); err != nil {
			return err
		}
		for _, pt := range op.Hint.Via {
			if err := checkCoords("via points", pt[0], pt[1]); err != nil {
				return err
			}
			h.Via = append(h.Via, [2]int{snap(pt[0], p.Grid), snap(pt[1], p.Grid)})
		}
	}
	l, err := s.layout(op.View)
	if err != nil {
		return err
	}
	empty := h.FromSide == "" && h.ToSide == "" && len(h.Via) == 0 && h.LabelPos == nil
	if k, isLift, err := view.ParseLiftKey(op.ID); isLift {
		if err != nil {
			return fmt.Errorf("%s: %w", op.ID, err)
		}
		for _, id := range k {
			if p.Model.Nodes[id] == nil {
				return fmt.Errorf("node %q does not exist", id)
			}
		}
		visible, collapsed := vw.Visible(p.Spec, p.Model)
		drawn := slices.ContainsFunc(vw.DrawnEdges(p.Spec, p.Model, visible, collapsed), func(e view.DrawnEdge) bool { return e.Lifted && e.From == k[0] && e.To == k[1] })
		switch {
		case empty:
			delete(l.LiftHints, k)
		case !drawn:
			return fmt.Errorf("view %q draws no lifted edge from %q to %q", op.View, k[0], k[1])
		default:
			l.LiftHints[k] = &h
		}
	} else {
		if p.Model.Edges[op.ID] == nil {
			return fmt.Errorf("edge %q does not exist", op.ID)
		}
		if !vw.EdgesIn(p.Spec, p.Model)[op.ID] {
			return fmt.Errorf("view %q does not show edge %q", op.View, op.ID)
		}
		if empty {
			delete(l.EdgeHints, op.ID)
		} else {
			l.EdgeHints[op.ID] = &h
		}
	}
	s.putLayout(op.View, l)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

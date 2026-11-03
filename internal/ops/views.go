package ops

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/view"
)

// OverlayArgs is an overlay of create_view and update_view: members are selectors as in
// views.hcl.
type OverlayArgs struct {
	ID      string   `json:"id"`
	Members []string `json:"members"`
	Style   string   `json:"style,omitempty"`
	Label   string   `json:"label,omitempty"`
}

// StyleArgs is a view style rule of create_view and update_view: type style (the default) or
// edge_style, its label, and its properties.
type StyleArgs struct {
	Type  string         `json:"type,omitempty"`
	Label string         `json:"label"`
	Props map[string]any `json:"props"`
}

// DecorationArgs are the fields of a decoration (spec 5.7) as add_decoration and
// update_decoration take them; a field left out is not changed.
type DecorationArgs struct {
	Type        *string   `json:"type,omitempty"`
	In          *string   `json:"in,omitempty"`
	Attach      *string   `json:"attach,omitempty"`
	X           *int      `json:"x,omitempty"`
	Y           *int      `json:"y,omitempty"`
	DX          *int      `json:"dx,omitempty"`
	DY          *int      `json:"dy,omitempty"`
	W           *int      `json:"w,omitempty"`
	H           *int      `json:"h,omitempty"`
	Z           *int      `json:"z,omitempty"`
	Text        *string   `json:"text,omitempty"`
	Src         *string   `json:"src,omitempty"`
	Points      [][2]int  `json:"points,omitzero"`
	Path        *string   `json:"path,omitempty"`
	Fill        *string   `json:"fill,omitempty"`
	Stroke      *string   `json:"stroke,omitempty"`
	StrokeWidth *float64  `json:"stroke_width,omitempty"`
	StrokeDash  []float64 `json:"stroke_dash,omitzero"`
	Opacity     *float64  `json:"opacity,omitempty"`
	TextColor   *string   `json:"text_color,omitempty"`
	FontSize    *float64  `json:"font_size,omitempty"`
}

// viewAttrs are the attributes update_view sets and unset names, besides overlays.
var viewAttrs = []string{"title", "include", "exclude", "collapse", "edges", "of", "direction"}

// decorationFields are the fields update_decoration's unset may name.
var decorationFields = []string{"in", "attach", "x", "y", "dx", "dy", "w", "h", "z", "text", "src", "points", "path", "fill", "stroke", "stroke_width", "stroke_dash", "opacity", "text_color", "font_size"}

// selectorTokens returns the list of selectors sels, as views.hcl writes them, for attribute
// name.
func selectorTokens(name string, sels []string) (hclwrite.Tokens, error) {
	var items []hclwrite.Tokens
	for _, src := range sels {
		expr, diags := hclsyntax.ParseExpression([]byte(src), "", hcl.InitialPos)
		if !diags.HasErrors() {
			_, diags = view.ParseSelector(expr, name == "edges")
		}
		if diags.HasErrors() {
			return nil, fmt.Errorf("%s: %q is not a selector: %s", name, src, diags[0].Detail)
		}
		toks, err := tokensOf(src)
		if err != nil {
			return nil, err
		}
		items = append(items, toks)
	}
	return hclwrite.TokensForTuple(items), nil
}

// setViewAttrs sets the attributes of a view block that op gives.
func setViewAttrs(body *hclwrite.Body, op Op) error {
	if op.Title != "" {
		setValue(body, "title", cty.StringVal(op.Title))
	}
	if op.Of != "" {
		setTraversal(body, "of", ref("node", op.Of))
	}
	if op.Direction != "" {
		if !slices.Contains(view.Directions, op.Direction) {
			return fmt.Errorf("direction is one of %s", strings.Join(view.Directions, ", "))
		}
		setValue(body, "direction", cty.StringVal(op.Direction))
	}
	for _, attr := range []struct {
		name string
		sels []string
	}{{"include", op.Include}, {"exclude", op.Exclude}, {"collapse", op.Collapse}, {"edges", op.Edges}} {
		if attr.sels == nil {
			continue
		}
		toks, err := selectorTokens(attr.name, attr.sels)
		if err != nil {
			return err
		}
		setAttr(body, attr.name, toks)
	}
	return nil
}

// overlayBlock returns the block of overlay o.
func overlayBlock(o OverlayArgs) (*hclwrite.Block, error) {
	if d := spec.CheckSimple("overlay", o.ID, hcl.Range{}); d.HasErrors() {
		return nil, fmt.Errorf("%s", d[0].Detail)
	}
	b := hclwrite.NewBlock("overlay", []string{o.ID})
	toks, err := selectorTokens("members", o.Members)
	if err != nil {
		return nil, fmt.Errorf("overlay %q: %w", o.ID, err)
	}
	b.Body().SetAttributeRaw("members", toks)
	if o.Style != "" {
		b.Body().SetAttributeValue("style", cty.StringVal(o.Style))
	}
	if o.Label != "" {
		b.Body().SetAttributeValue("label", cty.StringVal(o.Label))
	}
	return b, nil
}

// styleBlock returns the block of view style rule r.
func styleBlock(r StyleArgs) (*hclwrite.Block, error) {
	typ := cmp.Or(r.Type, "style")
	if typ != "style" && typ != "edge_style" {
		return nil, fmt.Errorf("a style rule's type is style or edge_style, not %q", r.Type)
	}
	if r.Label == "" {
		return nil, fmt.Errorf("a style rule needs a label")
	}
	b := hclwrite.NewBlock(typ, []string{r.Label})
	for _, name := range slices.Sorted(maps.Keys(r.Props)) {
		v, err := styleValue(name, r.Props[name])
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", typ, r.Label, err)
		}
		b.Body().SetAttributeValue(name, v)
	}
	return b, nil
}

// styleValue converts a style property value from JSON: a string, number or boolean, or a list
// of numbers.
func styleValue(name string, v any) (cty.Value, error) {
	list, ok := v.([]any)
	if !ok {
		return propValue(name, v)
	}
	vals := []cty.Value{}
	for _, item := range list {
		iv, err := propValue(name, item)
		if err != nil {
			return cty.NilVal, err
		}
		vals = append(vals, iv)
	}
	return cty.TupleVal(vals), nil
}

// blockKey names a nested block by its type and labels.
func blockKey(b *hclwrite.Block) string {
	return b.Type() + " " + strings.Join(labelsOf(b), " ")
}

// replaceBlocks makes the nested blocks of the given types of view id the blocks want, in
// order: a block whose content is unchanged stays as it is written, the others are removed, and
// the new ones are appended to the view.
func replaceBlocks(f *hclwrite.File, id string, types []string, want []*hclwrite.Block) (*hclwrite.File, error) {
	wanted := map[string][]byte{}
	for _, w := range want {
		if wanted[blockKey(w)] != nil {
			return nil, fmt.Errorf("%s is given twice", blockKey(w))
		}
		wanted[blockKey(w)] = canonical(w)
	}
	b := findBlock(f.Body(), "view", []string{id})
	var rm []*hclwrite.Block
	for _, nb := range b.Body().Blocks() {
		if !slices.Contains(types, nb.Type()) {
			continue
		}
		if bytes.Equal(wanted[blockKey(nb)], canonical(nb)) {
			want = slices.DeleteFunc(want, func(w *hclwrite.Block) bool { return blockKey(w) == blockKey(nb) })
			continue
		}
		rm = append(rm, nb)
	}
	if len(rm) > 0 {
		var err error
		if f, err = removeBlocks(f, rm); err != nil {
			return nil, err
		}
		b = findBlock(f.Body(), "view", []string{id})
	}
	for _, w := range want {
		appendNested(b.Body(), w)
	}
	return f, nil
}

// viewBlocks returns the overlay and style rule blocks op gives.
func viewBlocks(op Op) (overlays, styles []*hclwrite.Block, err error) {
	for _, o := range op.Overlays {
		ob, err := overlayBlock(o)
		if err != nil {
			return nil, nil, err
		}
		overlays = append(overlays, ob)
	}
	for _, r := range op.Styles {
		sb, err := styleBlock(r)
		if err != nil {
			return nil, nil, err
		}
		styles = append(styles, sb)
	}
	return overlays, styles, nil
}

// canonical returns b formatted without comments, for comparing blocks whatever their spacing
// and comments.
func canonical(b *hclwrite.Block) []byte {
	var toks hclwrite.Tokens
	for _, t := range b.BuildTokens(nil) {
		if t.Type == hclsyntax.TokenComment {
			if bytes.HasSuffix(t.Bytes, []byte("\n")) {
				toks = append(toks, tok(hclsyntax.TokenNewline, "\n"))
			}
			continue
		}
		toks = append(toks, t)
	}
	return bytes.TrimSpace(hclwrite.Format(toks.Bytes()))
}

// viewBlock returns views.hcl and the block of view id in it, on several lines.
func (s *state) viewBlock(id string) (string, *hclwrite.File, *hclwrite.Block, error) {
	p, err := s.project()
	if err != nil {
		return "", nil, nil, err
	}
	if p.Views[id] == nil {
		return "", nil, nil, fmt.Errorf("view %q does not exist", id)
	}
	name, f, err := s.viewsFile(true)
	if err != nil {
		return "", nil, nil, err
	}
	b := findBlock(f.Body(), "view", []string{id})
	if b == nil {
		return "", nil, nil, fmt.Errorf("view %q is not a block of %s", id, name)
	}
	f, b, err = expand(f, b)
	return name, f, b, err
}

func (s *state) createView(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	if d := spec.CheckSimple("view", op.ID, hcl.Range{}); d.HasErrors() {
		return fmt.Errorf("%s", d[0].Detail)
	}
	if v := p.Views[op.ID]; v != nil && v.Range.Filename != "" {
		return fmt.Errorf("view %q already exists", op.ID)
	}
	b := hclwrite.NewBlock("view", []string{op.ID})
	if err := setViewAttrs(b.Body(), op); err != nil {
		return err
	}
	overlays, styles, err := viewBlocks(op)
	if err != nil {
		return err
	}
	for _, nb := range append(overlays, styles...) {
		b.Body().AppendBlock(nb)
	}
	name, f, err := s.viewsFile(op.ID != view.DefaultView)
	if err != nil {
		return err
	}
	appendBlock(f, b)
	s.putHCL(name, f)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func (s *state) updateView(op Op) error {
	for _, u := range op.Unset {
		if !slices.Contains(viewAttrs, u) && u != "overlays" && u != "styles" {
			return fmt.Errorf("unset names attributes of a view: %s, overlays or styles, not %q", strings.Join(viewAttrs, ", "), u)
		}
	}
	given := map[string]bool{"title": op.Title != "", "of": op.Of != "", "direction": op.Direction != "", "include": op.Include != nil, "exclude": op.Exclude != nil, "collapse": op.Collapse != nil, "edges": op.Edges != nil, "overlays": op.Overlays != nil, "styles": op.Styles != nil}
	for _, u := range op.Unset {
		if given[u] {
			return fmt.Errorf("%s is both set and unset", u)
		}
	}
	name, f, _, err := s.viewBlock(op.ID)
	if err != nil {
		return err
	}
	overlays, styles, err := viewBlocks(op)
	if err != nil {
		return err
	}
	if op.Overlays != nil || slices.Contains(op.Unset, "overlays") {
		if f, err = replaceBlocks(f, op.ID, []string{"overlay"}, overlays); err != nil {
			return err
		}
	}
	if op.Styles != nil || slices.Contains(op.Unset, "styles") {
		if f, err = replaceBlocks(f, op.ID, []string{"style", "edge_style"}, styles); err != nil {
			return err
		}
	}
	b := findBlock(f.Body(), "view", []string{op.ID})
	for _, u := range op.Unset {
		removeAttr(b.Body(), u)
	}
	var added []string
	for _, a := range viewAttrs {
		if given[a] && b.Body().GetAttribute(a) == nil {
			added = append(added, a)
		}
	}
	if err := setViewAttrs(b.Body(), op); err != nil {
		return err
	}
	if f, err = attrsBeforeBlocks(f, "view", []string{op.ID}, added); err != nil {
		return err
	}
	s.putHCL(name, f)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func (s *state) deleteView(op Op) error {
	name, f, b, err := s.viewBlock(op.ID)
	if err != nil {
		return err
	}
	if s.p.Views[op.ID].Range.Filename == "" {
		return fmt.Errorf("view %q is the default view, which views.hcl does not define", op.ID)
	}
	if f, err = removeBlocks(f, []*hclwrite.Block{b}); err != nil {
		return err
	}
	s.putHCL(name, f)
	if _, ok := s.f.get(s.layoutPath(op.ID)); ok {
		s.f.remove(s.layoutPath(op.ID))
	}
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// decoration returns the layout of view v and the index of its decoration id.
func (s *state) decoration(v, id string) (*view.Layout, int, error) {
	p, err := s.project()
	if err != nil {
		return nil, 0, err
	}
	if p.Views[v] == nil {
		return nil, 0, fmt.Errorf("view %q does not exist", v)
	}
	l, err := s.layout(v)
	if err != nil {
		return nil, 0, err
	}
	i := slices.IndexFunc(l.Decorations, func(d *view.Decoration) bool { return d.ID == id })
	if i < 0 {
		return nil, 0, fmt.Errorf("view %q has no decoration %q", v, id)
	}
	return l, i, nil
}

// setDecoration copies the fields args gives into d, coordinates and sizes snapped to the grid.
func setDecoration(d *view.Decoration, a *DecorationArgs, grid int) error {
	if a == nil {
		return nil
	}
	snapped := func(v *int) (*int, error) {
		if v == nil {
			return nil, nil
		}
		if err := checkCoords("coordinates and sizes", *v); err != nil {
			return nil, err
		}
		r := snap(*v, grid)
		return &r, nil
	}
	for _, f := range []struct {
		dst **int
		src *int
	}{{&d.X, a.X}, {&d.Y, a.Y}, {&d.DX, a.DX}, {&d.DY, a.DY}} {
		v, err := snapped(f.src)
		if err != nil {
			return err
		}
		if v != nil {
			*f.dst = v
		}
	}
	for _, f := range []struct {
		dst **int
		src *int
	}{{&d.W, a.W}, {&d.H, a.H}} {
		if f.src == nil {
			continue
		}
		v, err := size(*f.src, grid)
		if err != nil {
			return err
		}
		*f.dst = &v
	}
	if a.Z != nil {
		d.Z = a.Z
	}
	for _, f := range []struct {
		dst *string
		src *string
	}{{&d.Type, a.Type}, {&d.In, a.In}, {&d.Attach, a.Attach}, {&d.Text, a.Text}, {&d.Src, a.Src}, {&d.Path, a.Path}, {&d.Fill, a.Fill}, {&d.Stroke, a.Stroke}, {&d.TextColor, a.TextColor}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if a.Points != nil {
		d.Points = nil
		for _, pt := range a.Points {
			if err := checkCoords("points", pt[0], pt[1]); err != nil {
				return err
			}
			d.Points = append(d.Points, [2]int{snap(pt[0], grid), snap(pt[1], grid)})
		}
	}
	for _, f := range []struct {
		dst **float64
		src *float64
	}{{&d.StrokeWidth, a.StrokeWidth}, {&d.Opacity, a.Opacity}, {&d.FontSize, a.FontSize}} {
		if f.src != nil {
			if math.IsInf(*f.src, 0) || math.IsNaN(*f.src) {
				return fmt.Errorf("a number is finite")
			}
			v := *f.src
			*f.dst = &v
		}
	}
	if a.StrokeDash != nil {
		d.StrokeDash = slices.Clone(a.StrokeDash)
	}
	return nil
}

// checkRefs refuses a decoration in or attached to a node that does not exist.
func (s *state) checkRefs(d *view.Decoration) error {
	for _, id := range []string{d.In, d.Attach} {
		if id != "" && s.p.Model.Nodes[id] == nil {
			return fmt.Errorf("node %q does not exist", id)
		}
	}
	return nil
}

// unsetDecoration clears the fields of d that names lists.
func unsetDecoration(d *view.Decoration, names []string) error {
	for _, n := range names {
		switch n {
		case "in":
			d.In = ""
		case "attach":
			d.Attach = ""
		case "x":
			d.X = nil
		case "y":
			d.Y = nil
		case "dx":
			d.DX = nil
		case "dy":
			d.DY = nil
		case "w":
			d.W = nil
		case "h":
			d.H = nil
		case "z":
			d.Z = nil
		case "text":
			d.Text = ""
		case "src":
			d.Src = ""
		case "points":
			d.Points = nil
		case "path":
			d.Path = ""
		case "fill":
			d.Fill = ""
		case "stroke":
			d.Stroke = ""
		case "stroke_width":
			d.StrokeWidth = nil
		case "stroke_dash":
			d.StrokeDash = nil
		case "opacity":
			d.Opacity = nil
		case "text_color":
			d.TextColor = ""
		case "font_size":
			d.FontSize = nil
		default:
			return fmt.Errorf("unset names decoration fields: %s, not %q", strings.Join(decorationFields, ", "), n)
		}
	}
	return nil
}

func (s *state) addDecoration(op Op) error {
	p, err := s.project()
	if err != nil {
		return err
	}
	if p.Views[op.View] == nil {
		return fmt.Errorf("view %q does not exist", op.View)
	}
	if op.Decoration == nil || op.Decoration.Type == nil {
		return fmt.Errorf("add_decoration needs a decoration with a type")
	}
	l, err := s.layout(op.View)
	if err != nil {
		return err
	}
	taken := func(id string) bool {
		return slices.ContainsFunc(l.Decorations, func(d *view.Decoration) bool { return d.ID == id })
	}
	id := op.ID
	if id == "" {
		id = newID(slug(*op.Decoration.Type), taken)
	} else if d := spec.CheckSimple("decoration", id, hcl.Range{}); d.HasErrors() {
		return fmt.Errorf("%s", d[0].Detail)
	} else if taken(id) {
		return fmt.Errorf("view %q already has a decoration %q", op.View, id)
	}
	d := &view.Decoration{ID: id}
	if err := setDecoration(d, op.Decoration, p.Grid); err != nil {
		return err
	}
	if err := s.checkRefs(d); err != nil {
		return err
	}
	l.Decorations = append(l.Decorations, d)
	s.putLayout(op.View, l)
	s.result(Result{Op: op.Op, ID: id})
	return nil
}

func (s *state) updateDecoration(op Op) error {
	l, i, err := s.decoration(op.View, op.ID)
	if err != nil {
		return err
	}
	d := l.Decorations[i]
	if err := unsetDecoration(d, op.Unset); err != nil {
		return err
	}
	if err := setDecoration(d, op.Decoration, s.p.Grid); err != nil {
		return err
	}
	if err := s.checkRefs(d); err != nil {
		return err
	}
	s.putLayout(op.View, l)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

func (s *state) deleteDecoration(op Op) error {
	l, i, err := s.decoration(op.View, op.ID)
	if err != nil {
		return err
	}
	l.Decorations = slices.Delete(l.Decorations, i, i+1)
	s.putLayout(op.View, l)
	s.result(Result{Op: op.Op, ID: op.ID})
	return nil
}

// promoteDecoration replaces a decoration with a node of kind, pinned where the decoration is
// and, for one with a size, at that size. A decoration in a frame becomes a child of that frame;
// an attached one, an arrow and a freehand line have no position Go can convert, so they are
// refused.
func (s *state) promoteDecoration(op Op) error {
	l, i, err := s.decoration(op.View, op.ID)
	if err != nil {
		return err
	}
	d := l.Decorations[i]
	switch {
	case d.Attach != "":
		return fmt.Errorf("decoration %q is attached to node %q; detach it first (update_decoration with x and y, unset attach, dx and dy)", d.ID, d.Attach)
	case d.Type == "arrow" || d.Type == "freehand":
		return fmt.Errorf("decoration %q is an %s, which has no box to become a node", d.ID, d.Type)
	case d.In != "":
		p := s.p
		n := p.Model.Nodes[d.In]
		if n == nil || !p.Spec.IsContainer(n.Kind) {
			return fmt.Errorf("decoration %q is in node %q, which is not a container, so the new node cannot be placed in it; move the decoration out first (update_decoration, unset in)", d.ID, d.In)
		}
		if visible, collapsed := p.Views[op.View].Visible(p.Spec, p.Model); !visible[d.In] || collapsed[d.In] {
			return fmt.Errorf("decoration %q is in container %q, which view %q does not draw expanded, so the new node would be hidden or placed elsewhere", d.ID, d.In, op.View)
		}
	}
	l.Decorations = slices.Delete(l.Decorations, i, i+1)
	s.putLayout(op.View, l)
	label := op.Label
	if label == nil && d.Type == "text" {
		label = &d.Text
	}
	x, y := 0, 0
	if d.X != nil {
		x = *d.X
	}
	if d.Y != nil {
		y = *d.Y
	}
	if err := s.createNode(Op{Op: op.Op, Kind: op.Kind, ID: op.New, Label: label, Parent: d.In, View: op.View, X: &x, Y: &y}); err != nil {
		return err
	}
	r := s.out[len(s.out)-1]
	if d.W != nil && d.H != nil {
		l, pl, err := s.pinned(op.View, r.ID)
		if err != nil {
			return err
		}
		pl.W, pl.H = d.W, d.H
		s.putLayout(op.View, l)
		r.W, r.H = d.W, d.H
	}
	s.out[len(s.out)-1] = r
	return nil
}

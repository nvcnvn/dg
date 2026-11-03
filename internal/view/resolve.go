// Package view holds view definitions and layout files, and resolves views: which elements a
// view shows and how they are styled (FR-4). It computes no geometry.
package view

import (
	"cmp"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"

	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/style"
)

// Sources is what validating and resolving views needs from a loaded project.
type Sources struct {
	Spec    *spec.Spec
	Styles  *style.Styles
	Model   *model.Model
	Views   Views
	Layouts map[string]*Layout
	Grid    int
}

// Resolved is the structure of one view, sent to the browser to draw.
type Resolved struct {
	View   string                  `json:"view"`
	Grid   int                     `json:"grid"`
	Font   *Font                   `json:"font,omitempty"`
	Nodes  []Node                  `json:"nodes"`
	Edges  []Edge                  `json:"edges"`
	Shapes map[string]*style.Shape `json:"shapes,omitempty"`
}

// Font is the project font: the family name an exported SVG names, and the font file.
type Font struct {
	Family string `json:"family"`
	Data   []byte `json:"data"`
}

// Node is a node drawn in a view.
type Node struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Frame     string `json:"frame,omitempty"`
	Container bool   `json:"container,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	Ports     []Port `json:"ports,omitempty"`
	// Compartments holds the node's rows; rows outside compartments are in one named "".
	Compartments []Compartment `json:"compartments,omitempty"`
	Pin          *Pin          `json:"pin,omitempty"`
	Style        style.Node    `json:"style"`
}

// Compartment is a named group of a node's rows, in declaration order.
type Compartment struct {
	Name string `json:"name"`
	Rows []Row  `json:"rows"`
}

// Row is a row of a table or class node.
type Row struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Key  string `json:"key,omitempty"`
}

// Port is a port of a node with its declared side, in declaration order.
type Port struct {
	Name string `json:"name"`
	Side string `json:"side"`
}

// Pin is a node's entry in the layout file. X and Y are relative to the content origin of its
// frame; In is set, to the stored frame ("" for the canvas), only when that differs from the
// node's frame and the browser must convert the coordinates (spec 7.1).
type Pin struct {
	In    *string               `json:"in,omitempty"`
	X     int                   `json:"x"`
	Y     int                   `json:"y"`
	W     *int                  `json:"w,omitempty"`
	H     *int                  `json:"h,omitempty"`
	Ports map[string]PinnedPort `json:"ports,omitempty"`
}

// PinnedPort is a pinned port position.
type PinnedPort struct {
	Side   string   `json:"side"`
	Offset *float64 `json:"offset,omitempty"`
}

// Edge is an edge drawn in a view.
type Edge struct {
	ID       string       `json:"id"`
	Kind     string       `json:"kind"`
	From     End          `json:"from"`
	To       End          `json:"to"`
	Labels   model.Labels `json:"labels"`
	ReadOnly bool         `json:"read_only,omitempty"`
	Hint     *EdgeHint    `json:"hint,omitempty"`
	Style    style.Edge   `json:"style"`
}

// End is the drawn endpoint of an edge.
type End struct {
	Node string `json:"node"`
	Port string `json:"port,omitempty"`
	Row  string `json:"row,omitempty"`
}

// EdgeHint is an edge's routing hint from the layout file; Via is in canvas coordinates.
type EdgeHint struct {
	FromSide string   `json:"from_side,omitempty"`
	ToSide   string   `json:"to_side,omitempty"`
	Via      [][2]int `json:"via,omitempty"`
	LabelPos *float64 `json:"label_pos,omitempty"`
}

// Resolve returns the resolved view id, with nodes and edges sorted by ID. Features the
// renderer does not draw yet are refused with errors naming the milestone that adds them.
func (s *Sources) Resolve(id string) (*Resolved, hcl.Diagnostics) {
	v := s.Views[id]
	if v == nil {
		return nil, hcl.Diagnostics{{Severity: hcl.DiagError, Summary: "Undefined view", Detail: "View \"" + id + "\" is not defined."}}
	}
	var diags hcl.Diagnostics
	notYet := func(rng hcl.Range, milestone int, what string) {
		d := spec.Errorf(rng, "Not supported yet", "%s drawn from M%d.", what, milestone)
		if !slices.ContainsFunc(diags, func(o *hcl.Diagnostic) bool { return o.Subject.String() == rng.String() && o.Detail == d.Detail }) {
			diags = append(diags, d)
		}
	}
	if len(v.Collapse) > 0 {
		notYet(v.Collapse[0].Range, 7, "Collapse is")
	}
	if len(v.Overlays) > 0 {
		notYet(v.Overlays[0].Range, 7, "Overlays are")
	}
	for _, r := range append(slices.Clone(v.Styles), v.EdgeStyles...) {
		notYet(r.Range, 7, "View style rules are")
	}
	l := s.Layouts[id]
	if l == nil {
		l = &Layout{}
	}
	if len(l.Decorations) > 0 {
		notYet(l.Decorations[0].Range, 7, "Decorations are")
	}

	m := s.Model
	visible, _ := v.Visible(s.Spec, m)
	r := &Resolved{View: id, Grid: s.Grid, Nodes: []Node{}, Edges: []Edge{}}
	for _, nid := range slices.Sorted(maps.Keys(visible)) {
		n := m.Nodes[nid]
		props := s.Styles.ForNode(s.Spec, n.Kind, n.Tags)
		s.refuseUndrawn(props, style.NodeProps, notYet)
		node := Node{
			ID:        n.ID,
			Kind:      n.Kind,
			Label:     n.Label,
			Frame:     Frame(m, visible, nid),
			Container: s.Spec.IsContainer(n.Kind),
			Ports:     s.ports(n),
			Style:     style.DrawnNode(props),
		}
		for _, c := range n.Compartments {
			rc := Compartment{Name: c.Name, Rows: []Row{}}
			for _, row := range c.Rows {
				rc.Rows = append(rc.Rows, Row{Name: row.Name, Type: row.Type, Key: row.Key})
			}
			node.Compartments = append(node.Compartments, rc)
		}
		if p := l.Place[nid]; p != nil {
			if len(p.Order) > 0 {
				notYet(p.Range, 8, "Order lists are")
			}
			node.Pin = pin(m, visible, node, p)
		}
		if sh := s.Styles.Shapes[node.Style.Shape]; sh != nil {
			if r.Shapes == nil {
				r.Shapes = map[string]*style.Shape{}
			}
			r.Shapes[sh.Name] = sh
		}
		r.Nodes = append(r.Nodes, node)
	}
	for _, eid := range slices.Sorted(maps.Keys(v.EdgesIn(s.Spec, m, visible))) {
		e := m.Edges[eid]
		props := s.Styles.ForEdge(s.Spec, e.Kind, e.Tags)
		s.refuseUndrawn(props, style.EdgeProps, notYet)
		edge := Edge{
			ID:     e.ID,
			Kind:   e.Kind,
			From:   End{Node: e.From.Node, Port: e.From.Port, Row: e.From.Row},
			To:     End{Node: e.To.Node, Port: e.To.Port, Row: e.To.Row},
			Labels: e.Labels,
			Style:  style.DrawnEdge(props),
		}
		if h := l.EdgeHints[eid]; h != nil {
			edge.Hint = &EdgeHint{FromSide: h.FromSide, ToSide: h.ToSide, Via: h.Via, LabelPos: h.LabelPos}
		}
		r.Edges = append(r.Edges, edge)
	}
	slices.SortFunc(diags, func(a, b *hcl.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Subject.Filename, b.Subject.Filename), cmp.Compare(a.Subject.Start.Line, b.Subject.Start.Line), cmp.Compare(a.Subject.Start.Column, b.Subject.Start.Column), cmp.Compare(a.Detail, b.Detail))
	})
	return r, diags
}

// ports returns a node's ports, its kind's first, with their declared sides.
func (s *Sources) ports(n *model.Node) []Port {
	def := s.Spec.DefaultPortSide(n.Kind)
	var out []Port
	for _, name := range s.Spec.Ports(n.Kind) {
		out = append(out, Port{Name: name, Side: def})
	}
	for _, p := range n.Ports {
		out = append(out, Port{Name: p.Name, Side: cmp.Or(p.Side, def)})
	}
	return out
}

// pin returns node's pin data from its layout entry, or nil when the entry's stored frame is
// not drawn in this view or lies inside the node itself, so the node is unpinned (spec 7.1).
func pin(m *model.Model, visible map[string]bool, node Node, p *Place) *Pin {
	if p.In != node.Frame && (p.In != "" && !visible[p.In] || p.In == node.ID || m.Contains(node.ID, p.In)) {
		return nil
	}
	out := &Pin{W: p.W, H: p.H}
	if p.In != node.Frame {
		out.In = &p.In
	}
	if p.X != nil {
		out.X = *p.X
	}
	if p.Y != nil {
		out.Y = *p.Y
	}
	for name, pp := range p.Ports {
		if !slices.ContainsFunc(node.Ports, func(port Port) bool { return port.Name == name }) {
			continue
		}
		if out.Ports == nil {
			out.Ports = map[string]PinnedPort{}
		}
		out.Ports[name] = PinnedPort{Side: pp.Side, Offset: pp.Offset}
	}
	return out
}

func (s *Sources) refuseUndrawn(props map[string]style.Setting, defs map[string]style.Prop, notYet func(hcl.Range, int, string)) {
	for _, name := range slices.Sorted(maps.Keys(props)) {
		set := props[name]
		if m := style.DrawnFrom(defs, name, set.Value); m > 0 {
			notYet(set.Range, m, "The value of "+name+" set here is")
		}
	}
}

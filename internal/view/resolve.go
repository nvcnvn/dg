// Package view holds view definitions and layout files, and resolves views: which elements a
// view shows and how they are styled (FR-4). It computes no geometry.
package view

import (
	"cmp"
	"fmt"
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
	View      string `json:"view"`
	Grid      int    `json:"grid"`
	Direction string `json:"direction,omitempty"`
	// Breadcrumbs are the levels of views this view leads back to, outermost first.
	Breadcrumbs [][]string              `json:"breadcrumbs,omitempty"`
	Font        *Font                   `json:"font,omitempty"`
	Nodes       []Node                  `json:"nodes"`
	Edges       []Edge                  `json:"edges"`
	Overlays    []ResolvedOverlay       `json:"overlays,omitempty"`
	Decorations []ResolvedDecoration    `json:"decorations,omitempty"`
	Shapes      map[string]*style.Shape `json:"shapes,omitempty"`
}

// ResolvedOverlay is an overlay with its drawn members and its style.
type ResolvedOverlay struct {
	ID      string        `json:"id"`
	Label   string        `json:"label,omitempty"`
	Members []string      `json:"members"`
	Style   style.Overlay `json:"style"`
}

// ResolvedDecoration is a decoration a view draws, with every style property set; X and Y are
// its dx and dy when it is attached (spec 5.7).
type ResolvedDecoration struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	In          string    `json:"in,omitempty"`
	Attach      string    `json:"attach,omitempty"`
	X           int       `json:"x"`
	Y           int       `json:"y"`
	W           int       `json:"w,omitempty"`
	H           int       `json:"h,omitempty"`
	Z           int       `json:"z"`
	Text        string    `json:"text,omitempty"`
	Image       string    `json:"image,omitempty"`
	Points      [][2]int  `json:"points,omitempty"`
	Path        string    `json:"path,omitempty"`
	Fill        string    `json:"fill"`
	Stroke      string    `json:"stroke"`
	StrokeWidth float64   `json:"stroke_width"`
	StrokeDash  []float64 `json:"stroke_dash"`
	Opacity     float64   `json:"opacity"`
	TextColor   string    `json:"text_color"`
	FontSize    float64   `json:"font_size"`
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
	// Collapsed is set for a collapsed container, which is drawn as a leaf (Container unset).
	Collapsed bool `json:"collapsed,omitempty"`
	// Drill lists the other views whose of is this node, sorted.
	Drill    []string `json:"drill,omitempty"`
	ReadOnly bool     `json:"read_only,omitempty"`
	Ports    []Port   `json:"ports,omitempty"`
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

// Edge is an edge drawn in a view. A lifted edge's ID is its lift:<from>:<to> key and Edges are
// the model edges it stands for.
type Edge struct {
	ID       string       `json:"id"`
	Edges    []string     `json:"edges,omitempty"`
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
	l := s.Layouts[id]
	if l == nil {
		l = &Layout{}
	}

	m := s.Model
	visible, collapsed := v.Visible(s.Spec, m)
	r := &Resolved{View: id, Grid: s.Grid, Direction: v.Direction, Breadcrumbs: s.breadcrumbs(v), Nodes: []Node{}, Edges: []Edge{}}
	for _, nid := range slices.Sorted(maps.Keys(visible)) {
		n := m.Nodes[nid]
		props := s.NodeStyle(v, n)
		s.refuseUndrawn(props, style.NodeProps, notYet)
		node := Node{
			ID:        n.ID,
			Kind:      n.Kind,
			Label:     n.Label,
			Frame:     Frame(m, visible, nid),
			Container: s.Spec.IsContainer(n.Kind) && !collapsed[nid],
			Collapsed: collapsed[nid],
			Drill:     s.drill(id, nid),
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
	for _, de := range v.DrawnEdges(s.Spec, m, visible, collapsed) {
		r.Edges = append(r.Edges, s.edge(v, l, de, notYet))
	}
	for _, o := range v.Overlays {
		members := v.OverlayMembers(s.Spec, m, visible, collapsed, o)
		if len(members) == 0 {
			continue
		}
		props := style.Cascade(s.Styles.Overlay, []string{"hull", cmp.Or(o.Style, "hull")})
		r.Overlays = append(r.Overlays, ResolvedOverlay{ID: o.ID, Label: o.Label, Members: members, Style: style.DrawnOverlay(props)})
	}
	for _, d := range l.Decorations {
		if d.Attach != "" && !visible[d.Attach] || d.In != "" && !visible[d.In] {
			continue
		}
		r.Decorations = append(r.Decorations, resolveDecoration(d))
	}
	slices.SortFunc(diags, func(a, b *hcl.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Subject.Filename, b.Subject.Filename), cmp.Compare(a.Subject.Start.Line, b.Subject.Start.Line), cmp.Compare(a.Subject.Start.Column, b.Subject.Start.Column), cmp.Compare(a.Detail, b.Detail))
	})
	return r, diags
}

// mergedLabel is the middle label of N > 1 lifted edges merged into one.
func mergedLabel(n int) string {
	return fmt.Sprintf("×%d", n)
}

// edge resolves a drawn edge. A lifted edge standing for one model edge keeps its labels and
// style; one standing for several is labeled ×N and styled by their kind if they share one,
// else by default. A lifted end has no port or row; the other end keeps those its edges share.
func (s *Sources) edge(v *View, l *Layout, de DrawnEdge, notYet func(hcl.Range, int, string)) Edge {
	m := s.Model
	e := m.Edges[de.Edges[0]]
	if !de.Lifted {
		props := s.EdgeStyle(v, e.Kind, e.Tags, e.ID)
		s.refuseUndrawn(props, style.EdgeProps, notYet)
		edge := Edge{
			ID:     e.ID,
			Kind:   e.Kind,
			From:   End{Node: e.From.Node, Port: e.From.Port, Row: e.From.Row},
			To:     End{Node: e.To.Node, Port: e.To.Port, Row: e.To.Row},
			Labels: e.Labels,
			Style:  style.DrawnEdge(props),
		}
		if h := l.EdgeHints[e.ID]; h != nil {
			edge.Hint = drawnHint(h)
		}
		return edge
	}
	from, to := End{Node: de.From}, End{Node: de.To}
	if e.From.Node == de.From {
		from = End{Node: de.From, Port: e.From.Port, Row: e.From.Row}
	}
	if e.To.Node == de.To {
		to = End{Node: de.To, Port: e.To.Port, Row: e.To.Row}
	}
	kind, labels, props := e.Kind, e.Labels, s.EdgeStyle(v, e.Kind, e.Tags, e.ID)
	if len(de.Edges) > 1 {
		for _, id := range de.Edges[1:] {
			o := m.Edges[id]
			if o.Kind != kind {
				kind = "default"
			}
			if (End{Node: o.From.Node, Port: o.From.Port, Row: o.From.Row}) != from {
				from = End{Node: de.From}
			}
			if (End{Node: o.To.Node, Port: o.To.Port, Row: o.To.Row}) != to {
				to = End{Node: de.To}
			}
		}
		labels = model.Labels{Mid: mergedLabel(len(de.Edges))}
		props = s.EdgeStyle(v, kind, nil, "")
	}
	s.refuseUndrawn(props, style.EdgeProps, notYet)
	edge := Edge{ID: de.Key, Edges: de.Edges, Kind: kind, From: from, To: to, Labels: labels, Style: style.DrawnEdge(props)}
	if h := l.LiftHints[[2]string{de.From, de.To}]; h != nil {
		edge.Hint = drawnHint(h)
	}
	return edge
}

func drawnHint(h *Hint) *EdgeHint {
	return &EdgeHint{FromSide: h.FromSide, ToSide: h.ToSide, Via: h.Via, LabelPos: h.LabelPos}
}

// drill returns the views other than current whose of is node id, sorted.
func (s *Sources) drill(current, id string) []string {
	var out []string
	for _, vid := range slices.Sorted(maps.Keys(s.Views)) {
		if o := s.Views[vid].Of; vid != current && o != nil && o.Target == id {
			out = append(out, vid)
		}
	}
	return out
}

// breadcrumbs returns the levels a view with of = node.X leads back to, outermost first, each
// level's views sorted by ID (spec 8.2). The level above a view with of = node.Y is the views
// with of on the nearest ancestor of Y that a view targets, only those that draw Y when some
// do; when no ancestor is targeted, it is the views without of that draw Y, and is the last.
func (s *Sources) breadcrumbs(v *View) [][]string {
	var levels [][]string
	seen := map[string]bool{}
	for y := v.Of; y != nil && !seen[y.Target]; {
		seen[y.Target] = true
		var level []string
		level, y = s.levelAbove(v.ID, y.Target)
		if len(level) == 0 {
			break
		}
		levels = append([][]string{level}, levels...)
	}
	return levels
}

// levelAbove returns the views one level above a view of node y, and their of, nil for views
// without one.
func (s *Sources) levelAbove(current, y string) ([]string, *Selector) {
	draws := func(ids []string) []string {
		return slices.DeleteFunc(slices.Clone(ids), func(id string) bool {
			visible, _ := s.Views[id].Visible(s.Spec, s.Model)
			return !visible[y]
		})
	}
	seen := map[string]bool{}
	for n := s.Model.Nodes[y]; n != nil && n.Parent != nil && !seen[n.ID]; n = s.Model.Nodes[n.Parent.Node] {
		seen[n.ID] = true
		if views := s.drill(current, n.Parent.Node); len(views) > 0 {
			if d := draws(views); len(d) > 0 {
				views = d
			}
			return views, s.Views[views[0]].Of
		}
	}
	var roots []string
	for _, id := range slices.Sorted(maps.Keys(s.Views)) {
		if id != current && s.Views[id].Of == nil {
			roots = append(roots, id)
		}
	}
	return draws(roots), nil
}

// Decoration defaults by type: fill, stroke, stroke_width.
var decorationPaint = map[string]struct {
	fill, stroke string
	width        float64
}{
	"text":     {"none", "none", 0},
	"rect":     {"none", "#5F6368", 1.5},
	"ellipse":  {"none", "#5F6368", 1.5},
	"band":     {"#F1F3F4", "none", 0},
	"image":    {"none", "none", 0},
	"arrow":    {"none", "#5F6368", 1.5},
	"freehand": {"none", "#5F6368", 1.5},
}

// resolveDecoration fills in a decoration's defaults (spec 5.7).
func resolveDecoration(d *Decoration) ResolvedDecoration {
	paint := decorationPaint[d.Type]
	z := 1
	if d.Type == "band" {
		z = -1
	}
	val := func(p *int, def int) int {
		if p != nil {
			return *p
		}
		return def
	}
	fval := func(p *float64, def float64) float64 {
		if p != nil {
			return *p
		}
		return def
	}
	x, y := val(d.X, 0), val(d.Y, 0)
	if d.Attach != "" {
		x, y = val(d.DX, 0), val(d.DY, 0)
	}
	dash := d.StrokeDash
	if dash == nil {
		dash = []float64{}
	}
	return ResolvedDecoration{
		ID: d.ID, Type: d.Type, In: d.In, Attach: d.Attach,
		X: x, Y: y, W: val(d.W, 0), H: val(d.H, 0), Z: val(d.Z, z),
		Text: d.Text, Image: d.Picture, Points: d.Points, Path: d.Path,
		Fill:        cmp.Or(d.Fill, paint.fill),
		Stroke:      cmp.Or(d.Stroke, paint.stroke),
		StrokeWidth: fval(d.StrokeWidth, paint.width),
		StrokeDash:  dash,
		Opacity:     fval(d.Opacity, 1),
		TextColor:   cmp.Or(d.TextColor, "#202124"),
		FontSize:    fval(d.FontSize, 14),
	}
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

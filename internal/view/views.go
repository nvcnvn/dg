package view

import (
	"cmp"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/style"
)

// DefaultView is the view a project without view definitions has.
const DefaultView = "main"

// Selector is one selector of a view (spec 4.5): Form is all, node, kind, tag, children,
// descendants, edge_kind or edge.
type Selector struct {
	Form   string
	Target string
	Range  hcl.Range
}

// View is a view block.
type View struct {
	ID        string
	Title     string
	Include   []Selector
	Exclude   []Selector
	Collapse  []Selector
	Edges     []Selector
	EdgesSet  bool
	Of        *Selector
	Direction string
	Overlays  []Overlay
	// NodeLabels are the labels this view draws nodes with instead of the model's, by node ID.
	NodeLabels map[string]NodeLabel
	Styles     []style.Rule
	EdgeStyles []style.Rule
	Range      hcl.Range
}

// NodeLabel is a node block of a view.
type NodeLabel struct {
	Label string
	Range hcl.Range
}

// Overlay is an overlay block of a view.
type Overlay struct {
	ID      string
	Members []Selector
	Style   string
	Label   string
	Range   hcl.Range
}

// Views holds the view definitions of a project, keyed by ID.
type Views map[string]*View

type viewBlock struct {
	ID        string         `hcl:"id,label"`
	Title     *string        `hcl:"title,optional"`
	Include   *hcl.Attribute `hcl:"include,optional"`
	Exclude   *hcl.Attribute `hcl:"exclude,optional"`
	Collapse  *hcl.Attribute `hcl:"collapse,optional"`
	Edges     *hcl.Attribute `hcl:"edges,optional"`
	Of        *hcl.Attribute `hcl:"of,optional"`
	Direction *hcl.Attribute `hcl:"direction,optional"`
	Overlays  []struct {
		ID      string         `hcl:"id,label"`
		Members *hcl.Attribute `hcl:"members"`
		Style   *string        `hcl:"style,optional"`
		Label   *string        `hcl:"label,optional"`
		Range   hcl.Range      `hcl:",def_range"`
	} `hcl:"overlay,block"`
	Nodes []struct {
		ID    string    `hcl:"id,label"`
		Label string    `hcl:"label"`
		Range hcl.Range `hcl:",def_range"`
	} `hcl:"node,block"`
	Styles     []style.Block `hcl:"style,block"`
	EdgeStyles []style.Block `hcl:"edge_style,block"`
	Range      hcl.Range     `hcl:",def_range"`
}

var (
	nodeForms = []string{"all", "node", "kind", "tag", "children", "descendants"}
	edgeForms = []string{"edge_kind", "tag", "edge"}
	// Directions are the values of a view's direction: the way the layers of its first layout run.
	Directions = []string{"down", "right", "up", "left"}
)

// Decode adds the view blocks of a views.hcl body to vs.
func (vs Views) Decode(body hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics {
	var f struct {
		Views []viewBlock `hcl:"view,block"`
	}
	diags := gohcl.DecodeBody(body, ctx, &f)
	for _, b := range f.Views {
		diags = append(diags, spec.CheckSimple("view", b.ID, b.Range)...)
		if prev := vs[b.ID]; prev != nil {
			diags = append(diags, spec.Errorf(b.Range, "Duplicate ID", "The view ID %q is already defined at line %d.", b.ID, prev.Range.Start.Line))
			continue
		}
		v := &View{ID: b.ID, Range: b.Range}
		if b.Title != nil {
			v.Title = *b.Title
		}
		selectors := func(attr *hcl.Attribute, forms []string) []Selector {
			if attr == nil {
				return nil
			}
			s, d := parseSelectors(attr.Expr, forms)
			diags = append(diags, d...)
			return s
		}
		v.Include = selectors(b.Include, nodeForms)
		v.Exclude = selectors(b.Exclude, nodeForms)
		v.Collapse = selectors(b.Collapse, nodeForms)
		v.Edges, v.EdgesSet = selectors(b.Edges, edgeForms), b.Edges != nil
		if b.Of != nil {
			s, d := parseSelector(b.Of.Expr, []string{"node"})
			diags = append(diags, d...)
			if !d.HasErrors() {
				v.Of = &s
			}
		}
		if b.Direction != nil {
			d, dd := spec.DecodeEnum(b.Direction.Expr, ctx, Directions)
			diags = append(diags, dd...)
			v.Direction = d
		}
		if b.Include == nil {
			v.Include = []Selector{{Form: "all", Range: b.Range}}
			if v.Of != nil {
				v.Include = []Selector{{Form: "descendants", Target: v.Of.Target, Range: v.Of.Range}}
			}
		}
		overlays := map[string]bool{}
		for _, o := range b.Overlays {
			diags = append(diags, spec.CheckSimple("overlay", o.ID, o.Range)...)
			if overlays[o.ID] {
				diags = append(diags, spec.Errorf(o.Range, "Duplicate ID", "View %q has two overlays %q.", b.ID, o.ID))
				continue
			}
			overlays[o.ID] = true
			ov := Overlay{ID: o.ID, Members: selectors(o.Members, nodeForms), Range: o.Range}
			if o.Style != nil {
				ov.Style = *o.Style
			}
			if o.Label != nil {
				ov.Label = *o.Label
			}
			v.Overlays = append(v.Overlays, ov)
		}
		for _, n := range b.Nodes {
			if prev, ok := v.NodeLabels[n.ID]; ok {
				diags = append(diags, spec.Errorf(n.Range, "Duplicate ID", "View %q has two node blocks %q; the first is at line %d.", b.ID, n.ID, prev.Range.Start.Line))
				continue
			}
			if v.NodeLabels == nil {
				v.NodeLabels = map[string]NodeLabel{}
			}
			v.NodeLabels[n.ID] = NodeLabel{Label: n.Label, Range: n.Range}
		}
		var d hcl.Diagnostics
		v.Styles, d = style.DecodeRules("style", b.Styles, style.NodeProps, ctx)
		diags = append(diags, d...)
		v.EdgeStyles, d = style.DecodeRules("edge_style", b.EdgeStyles, style.EdgeProps, ctx)
		diags = append(diags, d...)
		vs[b.ID] = v
	}
	return diags
}

func parseSelectors(expr hcl.Expression, forms []string) ([]Selector, hcl.Diagnostics) {
	items, diags := hcl.ExprList(expr)
	var out []Selector
	for _, item := range items {
		s, d := parseSelector(item, forms)
		diags = append(diags, d...)
		if !d.HasErrors() {
			out = append(out, s)
		}
	}
	return out, diags
}

// ParseSelector parses one selector of a node list (include, exclude, collapse, members, of) or,
// with edges, of an edges list.
func ParseSelector(expr hcl.Expression, edges bool) (Selector, hcl.Diagnostics) {
	if edges {
		return parseSelector(expr, edgeForms)
	}
	return parseSelector(expr, nodeForms)
}

func parseSelector(expr hcl.Expression, forms []string) (Selector, hcl.Diagnostics) {
	tr, diags := hcl.AbsTraversalForExpr(expr)
	s := Selector{Range: expr.Range()}
	ok := !diags.HasErrors()
	if ok {
		s.Form = tr.RootName()
		ok = slices.Contains(forms, s.Form)
	}
	switch {
	case !ok:
	case s.Form == "all":
		ok = len(tr) == 1
	default:
		ok = len(tr) == 2
		if ok {
			s.Target, ok = spec.StepName(tr[1])
		}
	}
	if !ok {
		want := make([]string, len(forms))
		for i, f := range forms {
			want[i] = f + ".<id>"
			if f == "all" {
				want[i] = f
			}
		}
		return s, hcl.Diagnostics{spec.Errorf(expr.Range(), "Invalid selector", "Expected one of %s.", spec.QuoteAll(want))}
	}
	return s, nil
}

// Validate reports undefined selector targets, selectors matching nothing (warnings), of and
// collapse naming non-containers, undefined overlay styles and invalid view style rules.
func (vs Views) Validate(sp *spec.Spec, st *style.Styles, m *model.Model) hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, id := range slices.Sorted(maps.Keys(vs)) {
		v := vs[id]
		if v.Range.Filename == "" {
			continue
		}
		check := func(sels []Selector) {
			for _, s := range sels {
				diags = append(diags, checkSelector(s, false, sp, m)...)
			}
		}
		check(v.Include)
		check(v.Exclude)
		check(v.Collapse)
		for _, s := range v.Edges {
			diags = append(diags, checkSelector(s, true, sp, m)...)
		}
		if v.Of != nil {
			check([]Selector{*v.Of})
			if n := m.Nodes[v.Of.Target]; n != nil && !sp.IsContainer(n.Kind) {
				diags = append(diags, spec.Errorf(v.Of.Range, "Not a container", "View %q has of = node %q, which is not a container.", id, n.ID))
			}
		}
		for _, s := range v.Collapse {
			switch {
			case s.Form == "node" && m.Nodes[s.Target] != nil && !sp.IsContainer(m.Nodes[s.Target].Kind):
				diags = append(diags, spec.Errorf(s.Range, "Not a container", "View %q collapses node %q, which is not a container.", id, s.Target))
			case s.Form == "kind" && sp.Kinds[s.Target] != nil && !sp.IsContainer(s.Target):
				diags = append(diags, spec.Errorf(s.Range, "Not a container", "View %q collapses kind %q, which is not a container kind.", id, s.Target))
			}
		}
		for _, o := range v.Overlays {
			check(o.Members)
			if o.Style != "" && !slices.ContainsFunc(st.Overlay, func(r style.Rule) bool { return r.Label == o.Style }) {
				diags = append(diags, spec.Errorf(o.Range, "Undefined overlay style", "Overlay %q uses undefined overlay style %q.", o.ID, o.Style))
			}
		}
		for _, nid := range slices.Sorted(maps.Keys(v.NodeLabels)) {
			if m.Nodes[nid] == nil {
				diags = append(diags, spec.Errorf(v.NodeLabels[nid].Range, "Undefined node", "View %q labels undefined node %q.", id, nid))
			}
		}
		nodeExists := func(id string) bool { return m.Nodes[id] != nil }
		edgeExists := func(id string) bool { return m.Edges[id] != nil }
		for _, r := range v.Styles {
			diags = append(diags, style.CheckLabel(r, false, sp, nodeExists)...)
			diags = append(diags, st.CheckShape(r)...)
		}
		for _, r := range v.EdgeStyles {
			diags = append(diags, style.CheckLabel(r, true, sp, edgeExists)...)
		}
	}
	return diags
}

func checkSelector(s Selector, edges bool, sp *spec.Spec, m *model.Model) hcl.Diagnostics {
	switch s.Form {
	case "tag":
		if d := spec.CheckSimple("tag", s.Target, s.Range); d != nil {
			return d
		}
	case "node", "children", "descendants":
		if m.Nodes[s.Target] == nil {
			return hcl.Diagnostics{spec.Errorf(s.Range, "Undefined node", "The selector names undefined node %q.", s.Target)}
		}
	case "kind":
		if sp.Kinds[s.Target] == nil {
			return hcl.Diagnostics{spec.Errorf(s.Range, "Undefined kind", "The selector names undefined kind %q.", s.Target)}
		}
	case "edge_kind":
		if sp.EdgeKinds[s.Target] == nil {
			return hcl.Diagnostics{spec.Errorf(s.Range, "Undefined edge kind", "The selector names undefined edge kind %q.", s.Target)}
		}
	case "edge":
		if m.Edges[s.Target] == nil {
			return hcl.Diagnostics{spec.Errorf(s.Range, "Undefined edge", "The selector names undefined edge %q.", s.Target)}
		}
	}
	matches := nodeMatches(s, sp, m)
	if edges {
		matches = edgeMatches(s, sp, m)
	}
	if len(matches) == 0 {
		return hcl.Diagnostics{spec.Warnf(s.Range, "Selector matches nothing", "No element matches this selector.")}
	}
	return nil
}

// nodeMatches returns the nodes a selector matches.
func nodeMatches(s Selector, sp *spec.Spec, m *model.Model) map[string]bool {
	out := map[string]bool{}
	for id, n := range m.Nodes {
		var match bool
		switch s.Form {
		case "all":
			match = true
		case "node":
			match = id == s.Target
		case "kind":
			match = sp.IsA(n.Kind, s.Target)
		case "tag":
			match = slices.Contains(n.Tags, s.Target)
		case "children":
			match = n.Parent != nil && n.Parent.Node == s.Target
		case "descendants":
			match = m.Contains(s.Target, id)
		}
		if match {
			out[id] = true
		}
	}
	return out
}

// edgeMatches returns the edges a selector matches.
func edgeMatches(s Selector, sp *spec.Spec, m *model.Model) map[string]bool {
	out := map[string]bool{}
	for id, e := range m.Edges {
		var match bool
		switch s.Form {
		case "edge_kind":
			match = slices.ContainsFunc(sp.EdgeLineage(e.Kind), func(k *spec.EdgeKind) bool { return k.Name == s.Target })
		case "tag":
			match = slices.Contains(e.Tags, s.Target)
		case "edge":
			match = id == s.Target
		}
		if match {
			out[id] = true
		}
	}
	return out
}

// Visible returns the nodes in v (spec 8.1) and the containers it collapses.
func (v *View) Visible(sp *spec.Spec, m *model.Model) (visible, collapsed map[string]bool) {
	visible = map[string]bool{}
	for _, s := range v.Include {
		maps.Copy(visible, nodeMatches(s, sp, m))
	}
	for _, s := range v.Exclude {
		for id := range nodeMatches(s, sp, m) {
			delete(visible, id)
		}
	}
	collapsed = map[string]bool{}
	for _, s := range v.Collapse {
		for id := range nodeMatches(s, sp, m) {
			if sp.IsContainer(m.Nodes[id].Kind) {
				collapsed[id] = true
			}
		}
	}
	for id := range visible {
		for c := range collapsed {
			if m.Contains(c, id) {
				delete(visible, id)
			}
		}
	}
	return visible, collapsed
}

// Frame returns the nearest visible ancestor of node id, or "" for the canvas (spec 7.1).
func Frame(m *model.Model, visible map[string]bool, id string) string {
	seen := map[string]bool{}
	for n := m.Nodes[id]; n != nil && n.Parent != nil && !seen[n.ID]; n = m.Nodes[n.Parent.Node] {
		seen[n.ID] = true
		if visible[n.Parent.Node] {
			return n.Parent.Node
		}
	}
	return ""
}

// DrawnEdge is an edge as a view draws it (spec 8.3): a model edge with both ends visible, keyed
// by its ID, or the lifted edges between the same two drawn nodes merged into one, keyed
// lift:<from>:<to>. Edges are the model edges it stands for, sorted.
type DrawnEdge struct {
	Key    string
	From   string
	To     string
	Edges  []string
	Lifted bool
}

// drawnAs returns the node that draws node id in a view: id when it is visible, else the
// outermost collapsed container holding it when that is visible, else "".
func drawnAs(m *model.Model, visible, collapsed map[string]bool, id string) string {
	if visible[id] {
		return id
	}
	out := ""
	for c := range collapsed {
		if m.Contains(c, id) && (out == "" || m.Contains(c, out)) {
			out = c
		}
	}
	if !visible[out] {
		return ""
	}
	return out
}

// DrawnEdges returns the edges v draws, sorted by key, given its visible and collapsed nodes
// (Visible): each model edge that passes its edges filter, with an end hidden inside a
// collapsed container lifted to the outermost collapsed container holding it, unless that is
// not visible; a lifted edge whose two drawn ends are the same collapsed container is hidden.
func (v *View) DrawnEdges(sp *spec.Spec, m *model.Model, visible, collapsed map[string]bool) []DrawnEdge {
	filter := map[string]bool{}
	for _, s := range v.Edges {
		maps.Copy(filter, edgeMatches(s, sp, m))
	}
	byKey := map[string]*DrawnEdge{}
	var plain []DrawnEdge
	for _, id := range slices.Sorted(maps.Keys(m.Edges)) {
		e := m.Edges[id]
		from, to := drawnAs(m, visible, collapsed, e.From.Node), drawnAs(m, visible, collapsed, e.To.Node)
		if from == "" || to == "" || v.EdgesSet && !filter[id] {
			continue
		}
		if from == e.From.Node && to == e.To.Node {
			plain = append(plain, DrawnEdge{Key: id, From: from, To: to, Edges: []string{id}})
			continue
		}
		if from == to {
			continue
		}
		key := liftKey(from, to)
		if byKey[key] == nil {
			byKey[key] = &DrawnEdge{Key: key, From: from, To: to, Lifted: true}
		}
		byKey[key].Edges = append(byKey[key].Edges, id)
	}
	for _, k := range slices.Sorted(maps.Keys(byKey)) {
		plain = append(plain, *byKey[k])
	}
	slices.SortFunc(plain, func(a, b DrawnEdge) int { return cmp.Compare(a.Key, b.Key) })
	return plain
}

// EdgesIn returns the model edges v draws as themselves, not lifted.
func (v *View) EdgesIn(sp *spec.Spec, m *model.Model) map[string]bool {
	visible, collapsed := v.Visible(sp, m)
	out := map[string]bool{}
	for _, e := range v.DrawnEdges(sp, m, visible, collapsed) {
		if !e.Lifted {
			out[e.Key] = true
		}
	}
	return out
}

// OverlayMembers returns the drawn members of overlay o of v, sorted, given its visible and
// collapsed nodes: each node its selectors match as it is drawn (spec 8.3).
func (v *View) OverlayMembers(sp *spec.Spec, m *model.Model, visible, collapsed map[string]bool, o Overlay) []string {
	members := map[string]bool{}
	for _, s := range o.Members {
		for id := range nodeMatches(s, sp, m) {
			if d := drawnAs(m, visible, collapsed, id); d != "" {
				members[d] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(members))
}

// ValidateLayouts reports layout files without a view (errors), and entries naming IDs that no
// longer exist, stale frames and positions of stack or grid children (warnings).
func (s *Sources) ValidateLayouts() hcl.Diagnostics {
	var diags hcl.Diagnostics
	m, sp := s.Model, s.Spec
	orphan := func(rng hcl.Range, what, id string) {
		diags = append(diags, spec.Warnf(rng, "Orphan layout entry", "%s %q does not exist.", what, id))
	}
	for _, id := range slices.Sorted(maps.Keys(s.Layouts)) {
		l := s.Layouts[id]
		v := s.Views[id]
		if v == nil {
			diags = append(diags, spec.Errorf(l.Range, "Layout without view", "No view %q is defined for this layout file.", id))
			continue
		}
		visible, collapsed := v.Visible(sp, m)
		for _, nid := range slices.Sorted(maps.Keys(l.Place)) {
			p := l.Place[nid]
			n := m.Nodes[nid]
			if n == nil {
				orphan(p.Range, "Node", nid)
				continue
			}
			if p.In != "" && m.Nodes[p.In] == nil {
				orphan(p.Range, "Frame node", p.In)
			}
			if len(p.Order) > 0 && s.NodeStyle(v, n)["layout"].Value.AsString() == "free" {
				diags = append(diags, spec.Warnf(p.Range, "Ignored order", "Node %q is not a stack or grid container, so its order is ignored.", nid))
			}
			seen := map[string]bool{}
			for _, o := range p.Order {
				switch on := m.Nodes[o]; {
				case on == nil:
					orphan(p.Range, "Ordered node", o)
				case seen[o]:
					diags = append(diags, spec.Warnf(p.Range, "Ignored order entry", "Node %q is in the order of %q twice.", o, nid))
				case on.Parent == nil || on.Parent.Node != nid:
					diags = append(diags, spec.Warnf(p.Range, "Ignored order entry", "Node %q in the order of %q is not its child.", o, nid))
				}
				seen[o] = true
			}
			ports := m.Ports(sp, n)
			for _, name := range slices.Sorted(maps.Keys(p.Ports)) {
				if !slices.Contains(ports, name) {
					orphan(p.Ports[name].Range, "Port", name)
				}
			}
			if !visible[nid] {
				continue
			}
			frame := Frame(m, visible, nid)
			c := s.PlacedBy(v, visible, nid)
			switch {
			case p.X == nil && p.Y == nil:
			case c != "":
				layout := s.NodeStyle(v, m.Nodes[c])["layout"].Value.AsString()
				diags = append(diags, spec.Warnf(p.Range, "Ignored position", "Node %q is placed in order by %q, a %s container, so its x and y are ignored.", nid, c, layout))
			case p.In != frame:
				diags = append(diags, spec.Warnf(p.Range, "Stale frame", "Node %q is stored relative to %s, but its frame in this view is %s.", nid, frameName(p.In), frameName(frame)))
			}
		}
		for _, eid := range slices.Sorted(maps.Keys(l.EdgeHints)) {
			if m.Edges[eid] == nil {
				orphan(l.EdgeHints[eid].Range, "Edge", eid)
			}
		}
		lifts := slices.SortedFunc(maps.Keys(l.LiftHints), func(a, b [2]string) int {
			return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
		})
		drawn := map[string]bool{}
		for _, e := range v.DrawnEdges(sp, m, visible, collapsed) {
			drawn[e.Key] = e.Lifted
		}
		for _, k := range lifts {
			missing := false
			for _, nid := range k {
				if m.Nodes[nid] == nil {
					orphan(l.LiftHints[k].Range, "Node", nid)
					missing = true
				}
			}
			if !missing && !drawn[liftKey(k[0], k[1])] {
				diags = append(diags, spec.Warnf(l.LiftHints[k].Range, "Orphan layout entry", "View %q draws no lifted edge from %q to %q.", id, k[0], k[1]))
			}
		}
		for _, d := range l.Decorations {
			for _, nid := range []string{d.In, d.Attach} {
				if nid != "" && m.Nodes[nid] == nil {
					orphan(d.Range, "Node", nid)
				}
			}
		}
	}
	return diags
}

func frameName(id string) string {
	if id == "" {
		return "the canvas"
	}
	return "node \"" + id + "\""
}

// NodeStyle returns the settings of node n in view v: the cascade of spec 8.4, ending with v's
// kind, tag and node rules.
func (s *Sources) NodeStyle(v *View, n *model.Node) map[string]style.Setting {
	props := s.Styles.ForNode(s.Spec, n.Kind, n.Tags)
	var labels []string
	for _, k := range s.Spec.Lineage(n.Kind) {
		labels = append(labels, k.Name)
	}
	for _, t := range n.Tags {
		labels = append(labels, "tag."+t)
	}
	maps.Copy(props, style.Cascade(v.Styles, append(labels, "node."+n.ID)))
	return props
}

// EdgeStyle returns the settings of an edge of kind with tags in view v, id naming it for v's
// edge.<id> rules ("" for a merged lifted edge, which has no tags either).
func (s *Sources) EdgeStyle(v *View, kind string, tags []string, id string) map[string]style.Setting {
	props := s.Styles.ForEdge(s.Spec, kind, tags)
	var labels []string
	for _, k := range s.Spec.EdgeLineage(kind) {
		labels = append(labels, k.Name)
	}
	for _, t := range tags {
		labels = append(labels, "tag."+t)
	}
	if id != "" {
		labels = append(labels, "edge."+id)
	}
	maps.Copy(props, style.Cascade(v.EdgeStyles, labels))
	return props
}

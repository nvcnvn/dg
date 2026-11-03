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
	View  string `json:"view"`
	Grid  int    `json:"grid"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node is a node drawn in a view.
type Node struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Label     string     `json:"label"`
	Frame     string     `json:"frame,omitempty"`
	Container bool       `json:"container,omitempty"`
	Style     style.Node `json:"style"`
}

// Edge is an edge drawn in a view.
type Edge struct {
	ID     string     `json:"id"`
	Kind   string     `json:"kind"`
	From   End        `json:"from"`
	To     End        `json:"to"`
	Labels Labels     `json:"labels"`
	Style  style.Edge `json:"style"`
}

// End is the drawn endpoint of an edge.
type End struct {
	Node string `json:"node"`
}

// Labels are an edge's labels.
type Labels struct {
	Mid string `json:"mid,omitempty"`
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
	if l := s.Layouts[id]; l != nil {
		if len(l.Place)+len(l.EdgeHints)+len(l.LiftHints) > 0 {
			notYet(l.Range, 3, "Pinned positions and edge hints are")
		}
		if len(l.Decorations) > 0 {
			notYet(l.Decorations[0].Range, 7, "Decorations are")
		}
	}

	m := s.Model
	visible, _ := v.Visible(s.Spec, m)
	r := &Resolved{View: id, Grid: s.Grid, Nodes: []Node{}, Edges: []Edge{}}
	for _, nid := range slices.Sorted(maps.Keys(visible)) {
		n := m.Nodes[nid]
		props := s.Styles.ForNode(s.Spec, n.Kind, n.Tags)
		s.refuseUndrawn(props, style.NodeProps, notYet)
		if len(n.Compartments) > 0 {
			notYet(n.Compartments[0].Range, 4, "Rows are")
		}
		r.Nodes = append(r.Nodes, Node{
			ID:        n.ID,
			Kind:      n.Kind,
			Label:     n.Label,
			Frame:     Frame(m, visible, nid),
			Container: s.Spec.IsContainer(n.Kind),
			Style:     style.DrawnNode(props),
		})
	}
	for _, eid := range slices.Sorted(maps.Keys(v.EdgesIn(s.Spec, m, visible))) {
		e := m.Edges[eid]
		props := s.Styles.ForEdge(s.Spec, e.Kind, e.Tags)
		s.refuseUndrawn(props, style.EdgeProps, notYet)
		switch {
		case e.From.Node == e.To.Node:
			notYet(e.Range, 4, "Self-loops are")
		case m.Contains(e.From.Node, e.To.Node) || m.Contains(e.To.Node, e.From.Node):
			notYet(e.Range, 3, "Edges between a container and its own descendant are")
		}
		for _, r := range []model.Ref{e.From, e.To} {
			if r.Port != "" {
				notYet(r.Range, 3, "Edges ending at ports are")
			}
			if r.Row != "" {
				notYet(r.Range, 4, "Edges ending at rows are")
			}
		}
		if e.Labels.Start != "" || e.Labels.End != "" {
			notYet(e.Range, 4, "Start and end labels are")
		}
		r.Edges = append(r.Edges, Edge{
			ID:     e.ID,
			Kind:   e.Kind,
			From:   End{Node: e.From.Node},
			To:     End{Node: e.To.Node},
			Labels: Labels{Mid: e.Labels.Mid},
			Style:  style.DrawnEdge(props),
		})
	}
	slices.SortFunc(diags, func(a, b *hcl.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Subject.Filename, b.Subject.Filename), cmp.Compare(a.Subject.Start.Line, b.Subject.Start.Line), cmp.Compare(a.Subject.Start.Column, b.Subject.Start.Column), cmp.Compare(a.Detail, b.Detail))
	})
	return r, diags
}

func (s *Sources) refuseUndrawn(props map[string]style.Setting, defs map[string]style.Prop, notYet func(hcl.Range, int, string)) {
	for _, name := range slices.Sorted(maps.Keys(props)) {
		set := props[name]
		if m := style.DrawnFrom(defs, name, set.Value); m > 0 {
			notYet(set.Range, m, "The value of "+name+" set here is")
		}
	}
}

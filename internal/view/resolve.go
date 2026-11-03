// Package view resolves views: which elements a view shows and how they are
// styled (FR-4). It computes no geometry.
package view

import (
	"fmt"
	"maps"
	"slices"

	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/style"
)

// DefaultView is the view a project without views.hcl has.
const DefaultView = "main"

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

// Resolve returns the resolved view id of p. Nodes and edges are sorted by ID.
func Resolve(p *project.Project, id string) (*Resolved, error) {
	if id != DefaultView {
		return nil, fmt.Errorf("view %q is not defined", id)
	}
	r := &Resolved{View: id, Grid: project.Grid, Nodes: []Node{}, Edges: []Edge{}}
	for _, nid := range slices.Sorted(maps.Keys(p.Model.Nodes)) {
		n := p.Model.Nodes[nid]
		rn := Node{
			ID:    n.ID,
			Kind:  n.Kind,
			Label: n.Label,
			Style: p.Styles.ForNode(p.Spec, n.Kind),
		}
		if n.Parent != nil {
			rn.Frame = n.Parent.Node
		}
		rn.Container = p.Spec.IsContainer(n.Kind)
		r.Nodes = append(r.Nodes, rn)
	}
	for _, eid := range slices.Sorted(maps.Keys(p.Model.Edges)) {
		e := p.Model.Edges[eid]
		r.Edges = append(r.Edges, Edge{
			ID:     e.ID,
			Kind:   e.Kind,
			From:   End{Node: e.From.Node},
			To:     End{Node: e.To.Node},
			Labels: Labels{Mid: e.Label},
			Style:  p.Styles.ForEdge(p.Spec, e.Kind),
		})
	}
	return r, nil
}

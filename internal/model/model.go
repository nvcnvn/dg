// Package model holds nodes and edges and validates them against the spec.
package model

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"unicode"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/spec"
)

// Node is a node instance.
type Node struct {
	ID     string
	Kind   string
	Label  string
	Parent *Ref
	Range  hcl.Range
}

// Edge is an edge instance.
type Edge struct {
	ID    string
	Kind  string
	Label string
	From  Ref
	To    Ref
	Range hcl.Range
}

// Ref is a reference to a node.
type Ref struct {
	Node  string
	Range hcl.Range
}

// Model is every node and edge of a project, keyed by ID.
type Model struct {
	Nodes map[string]*Node
	Edges map[string]*Edge
}

// New returns an empty Model.
func New() *Model {
	return &Model{Nodes: map[string]*Node{}, Edges: map[string]*Edge{}}
}

type file struct {
	Nodes []struct {
		ID     string         `hcl:"id,label"`
		Kind   *string        `hcl:"kind,optional"`
		Label  *string        `hcl:"label,optional"`
		Parent *hcl.Attribute `hcl:"parent,optional"`
		Range  hcl.Range      `hcl:",def_range"`
	} `hcl:"node,block"`
	Edges []struct {
		ID    string         `hcl:"id,label"`
		From  hcl.Expression `hcl:"from"`
		To    hcl.Expression `hcl:"to"`
		Kind  *string        `hcl:"kind,optional"`
		Label *string        `hcl:"label,optional"`
		Range hcl.Range      `hcl:",def_range"`
	} `hcl:"edge,block"`
}

// Decode adds the node and edge blocks of body to m.
func (m *Model) Decode(body hcl.Body) hcl.Diagnostics {
	var f file
	diags := gohcl.DecodeBody(body, nil, &f)
	for _, b := range f.Nodes {
		n := &Node{ID: b.ID, Kind: or(b.Kind, spec.RootKind), Label: or(b.Label, b.ID), Range: b.Range}
		if b.Parent != nil {
			ref, d := nodeRef(b.Parent.Expr)
			diags = append(diags, d...)
			if !d.HasErrors() {
				n.Parent = &ref
			}
		}
		diags = append(diags, checkID("node", b.ID, b.Range)...)
		if prev := m.Nodes[b.ID]; prev != nil {
			diags = append(diags, duplicate("node", b.ID, b.Range, prev.Range))
			continue
		}
		m.Nodes[b.ID] = n
	}
	for _, b := range f.Edges {
		e := &Edge{ID: b.ID, Kind: or(b.Kind, spec.DefaultEdgeKind), Label: or(b.Label, ""), Range: b.Range}
		from, d1 := requiredRef("from", b.From, b.Range)
		to, d2 := requiredRef("to", b.To, b.Range)
		diags = append(diags, d1...)
		diags = append(diags, d2...)
		e.From, e.To = from, to
		diags = append(diags, checkID("edge", b.ID, b.Range)...)
		if prev := m.Edges[b.ID]; prev != nil {
			diags = append(diags, duplicate("edge", b.ID, b.Range, prev.Range))
			continue
		}
		if !d1.HasErrors() && !d2.HasErrors() {
			m.Edges[b.ID] = e
		}
	}
	return diags
}

func or(v *string, def string) string {
	if v == nil {
		return def
	}
	return *v
}

// requiredRef decodes a required reference; gohcl gives a missing
// hcl.Expression attribute a null value instead of reporting it.
func requiredRef(name string, expr hcl.Expression, block hcl.Range) (Ref, hcl.Diagnostics) {
	if v, d := expr.Value(nil); !d.HasErrors() && v.IsNull() {
		return Ref{}, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Missing required argument",
			Detail:   fmt.Sprintf("The argument %q is required.", name),
			Subject:  block.Ptr(),
		}}
	}
	return nodeRef(expr)
}

func nodeRef(expr hcl.Expression) (Ref, hcl.Diagnostics) {
	bad := hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  "Invalid node reference",
		Detail:   `Expected node.<id> or node["<id>"]. Port and row references are not supported yet.`,
		Subject:  expr.Range().Ptr(),
	}}
	tr, diags := hcl.AbsTraversalForExpr(expr)
	if diags.HasErrors() || tr.RootName() != "node" || len(tr) != 2 {
		return Ref{}, bad
	}
	ref := Ref{Range: expr.Range()}
	switch step := tr[1].(type) {
	case hcl.TraverseAttr:
		ref.Node = step.Name
	case hcl.TraverseIndex:
		if step.Key.Type() != cty.String || !step.Key.IsKnown() || step.Key.IsNull() {
			return Ref{}, bad
		}
		ref.Node = step.Key.AsString()
	default:
		return Ref{}, bad
	}
	return ref, nil
}

var (
	simpleID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	reserved = map[string]bool{
		"all": true, "node": true, "kind": true, "tag": true, "children": true,
		"descendants": true, "edge_kind": true, "port": true, "row": true, "lift": true,
	}
)

func checkID(what, id string, rng hcl.Range) hcl.Diagnostics {
	var problem string
	switch {
	case reserved[id]:
		problem = "is reserved"
	case simpleID.MatchString(id):
		return nil
	case id == "":
		problem = "is empty"
	case len([]rune(id)) > 200:
		problem = "is longer than 200 characters"
	case slices.ContainsFunc([]rune(id), unicode.IsControl):
		problem = "contains a control character"
	default:
		return nil
	}
	return hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  "Invalid ID",
		Detail:   fmt.Sprintf("The %s ID %q %s.", what, id, problem),
		Subject:  rng.Ptr(),
	}}
}

func duplicate(what, id string, rng, prev hcl.Range) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Duplicate ID",
		Detail:   fmt.Sprintf("The %s ID %q is already defined at %s:%d:%d.", what, id, prev.Filename, prev.Start.Line, prev.Start.Column),
		Subject:  rng.Ptr(),
	}
}

// Validate reports undefined kinds and nodes, non-container parents, containment cycles and unsupported edges.
func (m *Model) Validate(sp *spec.Spec) hcl.Diagnostics {
	var diags hcl.Diagnostics
	errorf := func(rng hcl.Range, summary, format string, args ...any) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  summary,
			Detail:   fmt.Sprintf(format, args...),
			Subject:  rng.Ptr(),
		})
	}
	for _, id := range slices.Sorted(maps.Keys(m.Nodes)) {
		n := m.Nodes[id]
		if sp.Kinds[n.Kind] == nil {
			errorf(n.Range, "Undefined kind", "Node %q has undefined kind %q.", id, n.Kind)
		}
		if n.Parent == nil {
			continue
		}
		p := m.Nodes[n.Parent.Node]
		switch {
		case p == nil:
			errorf(n.Parent.Range, "Undefined node", "Parent %q of node %q is not defined.", n.Parent.Node, id)
		case sp.Kinds[p.Kind] != nil && !sp.IsContainer(p.Kind):
			errorf(n.Parent.Range, "Parent is not a container", "Parent %q of node %q has kind %q, which is not a container.", p.ID, id, p.Kind)
		case m.inCycle(id):
			errorf(n.Parent.Range, "Containment cycle", "Node %q contains itself through its parents.", id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(m.Edges)) {
		e := m.Edges[id]
		if sp.EdgeKinds[e.Kind] == nil {
			errorf(e.Range, "Undefined edge kind", "Edge %q has undefined edge kind %q.", id, e.Kind)
		}
		missing := false
		for _, r := range []Ref{e.From, e.To} {
			if m.Nodes[r.Node] == nil {
				errorf(r.Range, "Undefined node", "Edge %q references undefined node %q.", id, r.Node)
				missing = true
			}
		}
		switch {
		case missing:
		case e.From.Node == e.To.Node:
			errorf(e.Range, "Unsupported edge", "Edge %q starts and ends at %q; self-loops are not supported yet.", id, e.From.Node)
		case m.contains(e.From.Node, e.To.Node) || m.contains(e.To.Node, e.From.Node):
			errorf(e.Range, "Unsupported edge", "Edge %q connects a container to its own descendant; such edges are not supported yet.", id)
		}
	}
	return diags
}

// contains reports whether node outer is an ancestor of node inner.
func (m *Model) contains(outer, inner string) bool {
	seen := map[string]bool{}
	for n := m.Nodes[inner]; n != nil && n.Parent != nil && !seen[n.ID]; n = m.Nodes[n.Parent.Node] {
		seen[n.ID] = true
		if n.Parent.Node == outer {
			return true
		}
	}
	return false
}

func (m *Model) inCycle(id string) bool {
	return m.contains(id, id)
}

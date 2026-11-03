// Package model holds nodes and edges and validates them against the spec.
package model

import (
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/spec"
)

// Node is a node instance.
type Node struct {
	ID           string
	Kind         string
	Label        string
	Parent       *Ref
	Tags         []string
	Props        []Prop
	Ports        []Port
	Compartments []Compartment
	Range        hcl.Range
}

// Prop is a property value set on a node.
type Prop struct {
	Name  string
	Value cty.Value
	Range hcl.Range
}

// Port is an instance port; Side is "" when the kind's default side applies.
type Port struct {
	Name  string
	Side  string
	Range hcl.Range
}

// Compartment is a named group of rows; rows declared outside a compartment are in one named "".
type Compartment struct {
	Name  string
	Rows  []Row
	Range hcl.Range
}

// Row is a row of a table or class node.
type Row struct {
	Name  string
	Type  string
	Key   string
	Range hcl.Range
}

// Edge is an edge instance.
type Edge struct {
	ID     string
	Kind   string
	Labels Labels
	Tags   []string
	From   Ref
	To     Ref
	Range  hcl.Range
}

// Labels are the labels at an edge's start, middle and end.
type Labels struct {
	Start string `json:"start,omitempty"`
	Mid   string `json:"mid,omitempty"`
	End   string `json:"end,omitempty"`
}

// Ref is a reference to a node, or to one of its ports or rows.
type Ref struct {
	Node  string
	Port  string
	Row   string
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

type rowBlock struct {
	Name  string    `hcl:"name,label"`
	Type  *string   `hcl:"type,optional"`
	Key   *string   `hcl:"key,optional"`
	Range hcl.Range `hcl:",def_range"`
}

type nodeBlock struct {
	ID         string         `hcl:"id,label"`
	Kind       *hcl.Attribute `hcl:"kind,optional"`
	Label      *hcl.Attribute `hcl:"label,optional"`
	Parent     *hcl.Attribute `hcl:"parent,optional"`
	Tags       *hcl.Attribute `hcl:"tags,optional"`
	Props      *hcl.Attribute `hcl:"props,optional"`
	Ports      *hcl.Attribute `hcl:"ports,optional"`
	PortBlocks []struct {
		Name  string         `hcl:"name,label"`
		Side  *hcl.Attribute `hcl:"side,optional"`
		Range hcl.Range      `hcl:",def_range"`
	} `hcl:"port,block"`
	Rows         []rowBlock `hcl:"row,block"`
	Compartments []struct {
		Name  string     `hcl:"name,label"`
		Rows  []rowBlock `hcl:"row,block"`
		Range hcl.Range  `hcl:",def_range"`
	} `hcl:"compartment,block"`
	Range hcl.Range `hcl:",def_range"`
}

type edgeBlock struct {
	ID     string         `hcl:"id,label"`
	From   hcl.Expression `hcl:"from"`
	To     hcl.Expression `hcl:"to"`
	Kind   *hcl.Attribute `hcl:"kind,optional"`
	Label  *hcl.Attribute `hcl:"label,optional"`
	Labels *hcl.Attribute `hcl:"labels,optional"`
	Tags   *hcl.Attribute `hcl:"tags,optional"`
	Range  hcl.Range      `hcl:",def_range"`
}

type file struct {
	Nodes []nodeBlock `hcl:"node,block"`
	Edges []edgeBlock `hcl:"edge,block"`
}

// Decode adds the node and edge blocks of body to m.
func (m *Model) Decode(body hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics {
	var f file
	diags := gohcl.DecodeBody(body, ctx, &f)
	for _, b := range f.Nodes {
		n, d := decodeNode(b, ctx)
		diags = append(diags, d...)
		diags = append(diags, spec.CheckID("node", b.ID, b.Range)...)
		if prev := m.Nodes[b.ID]; prev != nil {
			diags = append(diags, duplicate("node", b.ID, b.Range, prev.Range))
			continue
		}
		m.Nodes[b.ID] = n
	}
	for _, b := range f.Edges {
		e, d := decodeEdge(b, ctx)
		diags = append(diags, d...)
		diags = append(diags, spec.CheckID("edge", b.ID, b.Range)...)
		if prev := m.Edges[b.ID]; prev != nil {
			diags = append(diags, duplicate("edge", b.ID, b.Range, prev.Range))
			continue
		}
		m.Edges[b.ID] = e
	}
	return diags
}

func decodeNode(b nodeBlock, ctx *hcl.EvalContext) (*Node, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	n := &Node{ID: b.ID, Kind: str(b.Kind, ctx, spec.RootKind, &diags), Label: str(b.Label, ctx, b.ID, &diags), Range: b.Range}
	if b.Parent != nil {
		ref, d := parseRef(b.Parent.Expr, false)
		diags = append(diags, d...)
		if !d.HasErrors() {
			n.Parent = &ref
		}
	}
	n.Tags, diags = decodeTags(b.Tags, ctx, diags)
	if b.Props != nil {
		props, d := decodeProps(b.Props, ctx)
		diags = append(diags, d...)
		n.Props = props
	}
	seen := map[string]bool{}
	addPort := func(p Port) {
		if seen[p.Name] {
			diags = append(diags, spec.Errorf(p.Range, "Duplicate port", "Node %q declares port %q twice.", n.ID, p.Name))
			return
		}
		seen[p.Name] = true
		n.Ports = append(n.Ports, p)
	}
	if b.Ports != nil {
		names, d := spec.DecodeNames("port", b.Ports.Expr, ctx)
		diags = append(diags, d...)
		for _, name := range names {
			addPort(Port{Name: name.Name, Range: name.Range})
		}
	}
	for _, p := range b.PortBlocks {
		port := Port{Name: p.Name, Range: p.Range}
		if problem := spec.NameProblem(p.Name); problem != "" {
			diags = append(diags, spec.Errorf(p.Range, "Invalid name", "The port name %q %s.", p.Name, problem))
			continue
		}
		if p.Side != nil {
			side, d := spec.DecodeEnum(p.Side.Expr, ctx, spec.Sides)
			diags = append(diags, d...)
			port.Side = side
		}
		addPort(port)
	}
	if len(b.Rows) > 0 && len(b.Compartments) > 0 {
		diags = append(diags, spec.Errorf(b.Compartments[0].Range, "Rows outside compartments", "Node %q declares rows both inside and outside compartments.", n.ID))
	}
	rows := map[string]bool{}
	decodeRows := func(blocks []rowBlock) []Row {
		var out []Row
		for _, r := range blocks {
			switch problem := spec.NameProblem(r.Name); {
			case problem != "":
				diags = append(diags, spec.Errorf(r.Range, "Invalid name", "The row name %q %s.", r.Name, problem))
			case rows[r.Name]:
				diags = append(diags, spec.Errorf(r.Range, "Duplicate row", "Node %q declares row %q twice.", n.ID, r.Name))
			default:
				rows[r.Name] = true
				out = append(out, Row{Name: r.Name, Type: or(r.Type, ""), Key: or(r.Key, ""), Range: r.Range})
			}
		}
		return out
	}
	if len(b.Rows) > 0 {
		n.Compartments = append(n.Compartments, Compartment{Rows: decodeRows(b.Rows), Range: b.Rows[0].Range})
	}
	compartments := map[string]bool{}
	for _, c := range b.Compartments {
		switch problem := spec.NameProblem(c.Name); {
		case problem != "":
			diags = append(diags, spec.Errorf(c.Range, "Invalid name", "The compartment name %q %s.", c.Name, problem))
		case compartments[c.Name]:
			diags = append(diags, spec.Errorf(c.Range, "Duplicate compartment", "Node %q declares compartment %q twice.", n.ID, c.Name))
		default:
			compartments[c.Name] = true
			n.Compartments = append(n.Compartments, Compartment{Name: c.Name, Rows: decodeRows(c.Rows), Range: c.Range})
		}
	}
	return n, diags
}

func decodeTags(attr *hcl.Attribute, ctx *hcl.EvalContext, diags hcl.Diagnostics) ([]string, hcl.Diagnostics) {
	if attr == nil {
		return nil, diags
	}
	list, d := spec.DecodeStrings(attr.Expr, ctx)
	diags = append(diags, d...)
	var tags []string
	for _, t := range list {
		diags = append(diags, spec.CheckSimple("tag", t.Name, t.Range)...)
		tags = append(tags, t.Name)
	}
	return tags, diags
}

func decodeProps(attr *hcl.Attribute, ctx *hcl.EvalContext) ([]Prop, hcl.Diagnostics) {
	pairs, diags := hcl.ExprMap(attr.Expr)
	var props []Prop
	seen := map[string]bool{}
	for _, kv := range pairs {
		var name string
		if d := spec.DecodeExpr(kv.Key, ctx, &name); d.HasErrors() {
			diags = append(diags, d...)
			continue
		}
		v, d := kv.Value.Value(ctx)
		diags = append(diags, d...)
		switch {
		case d.HasErrors():
		case seen[name]:
			diags = append(diags, spec.Errorf(kv.Key.Range(), "Duplicate property", "The property %q is set twice.", name))
		default:
			seen[name] = true
			props = append(props, Prop{Name: name, Value: v, Range: kv.Key.Range()})
		}
	}
	return props, diags
}

func decodeEdge(b edgeBlock, ctx *hcl.EvalContext) (*Edge, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	e := &Edge{ID: b.ID, Kind: str(b.Kind, ctx, spec.DefaultEdgeKind, &diags), Labels: Labels{Mid: str(b.Label, ctx, "", &diags)}, Range: b.Range}
	from, d := requiredRef("from", b.From, b.Range)
	diags = append(diags, d...)
	to, d := requiredRef("to", b.To, b.Range)
	diags = append(diags, d...)
	e.From, e.To = from, to
	e.Tags, diags = decodeTags(b.Tags, ctx, diags)
	if b.Labels != nil {
		var labels map[string]string
		d := spec.DecodeExpr(b.Labels.Expr, ctx, &labels)
		diags = append(diags, d...)
		if b.Label != nil {
			diags = append(diags, spec.Errorf(b.Labels.NameRange, "Conflicting arguments", "Edge %q sets both label and labels; label is shorthand for labels = { mid = ... }.", b.ID))
		}
		for _, k := range slices.Sorted(maps.Keys(labels)) {
			switch k {
			case "start":
				e.Labels.Start = labels[k]
			case "mid":
				e.Labels.Mid = labels[k]
			case "end":
				e.Labels.End = labels[k]
			default:
				diags = append(diags, spec.Errorf(b.Labels.Expr.Range(), "Invalid label position", "%q is not start, mid or end.", k))
			}
		}
	}
	return e, diags
}

func or(v *string, def string) string {
	if v == nil {
		return def
	}
	return *v
}

// str decodes an optional string attribute, or returns def.
func str(attr *hcl.Attribute, ctx *hcl.EvalContext, def string, diags *hcl.Diagnostics) string {
	if attr == nil {
		return def
	}
	var v string
	d := spec.DecodeExpr(attr.Expr, ctx, &v)
	*diags = append(*diags, d...)
	if d.HasErrors() {
		return def
	}
	return v
}

// requiredRef decodes a required reference; gohcl gives a missing
// hcl.Expression attribute a null value instead of reporting it.
func requiredRef(name string, expr hcl.Expression, block hcl.Range) (Ref, hcl.Diagnostics) {
	if v, d := expr.Value(nil); !d.HasErrors() && v.IsNull() {
		return Ref{}, hcl.Diagnostics{spec.Errorf(block, "Missing required argument", "The argument %q is required.", name)}
	}
	return parseRef(expr, true)
}

// parseRef parses node.<id>, node["<id>"] and, with ends, a port or row of one:
// node.<id>.port.<port> and node.<id>.row.<row>, each name also in index form.
func parseRef(expr hcl.Expression, ends bool) (Ref, hcl.Diagnostics) {
	want := `node.<id> or node["<id>"]`
	if ends {
		want = `node.<id>, node.<id>.port.<port> or node.<id>.row.<row>, with index form node["<id>"] for IDs that are not simple`
	}
	bad := hcl.Diagnostics{spec.Errorf(expr.Range(), "Invalid node reference", "Expected %s.", want)}
	tr, diags := hcl.AbsTraversalForExpr(expr)
	if diags.HasErrors() || tr.RootName() != "node" || !(len(tr) == 2 || ends && len(tr) == 4) {
		return Ref{}, bad
	}
	ref := Ref{Range: expr.Range()}
	var ok bool
	if ref.Node, ok = spec.StepName(tr[1]); !ok {
		return Ref{}, bad
	}
	if len(tr) == 2 {
		return ref, nil
	}
	what, _ := spec.StepName(tr[2])
	name, ok := spec.StepName(tr[3])
	switch {
	case !ok:
		return Ref{}, bad
	case what == "port":
		ref.Port = name
	case what == "row":
		ref.Row = name
	default:
		return Ref{}, bad
	}
	return ref, nil
}

func duplicate(what, id string, rng, prev hcl.Range) *hcl.Diagnostic {
	return spec.Errorf(rng, "Duplicate ID", "The %s ID %q is already defined at %s:%d:%d.", what, id, prev.Filename, prev.Start.Line, prev.Start.Column)
}

// Ports returns the names of a node's ports: its kind's, then its own.
func (m *Model) Ports(sp *spec.Spec, n *Node) []string {
	ports := sp.Ports(n.Kind)
	for _, p := range n.Ports {
		ports = append(ports, p.Name)
	}
	return ports
}

// HasRow reports whether node n declares row name.
func (n *Node) HasRow(name string) bool {
	for _, c := range n.Compartments {
		for _, r := range c.Rows {
			if r.Name == name {
				return true
			}
		}
	}
	return false
}

// Validate reports undefined kinds, nodes, ports, rows and properties, wrong property types,
// ports that repeat a kind port, rows on a kind without rows, non-container parents and
// containment cycles.
func (m *Model) Validate(sp *spec.Spec) hcl.Diagnostics {
	var diags hcl.Diagnostics
	errorf := func(rng hcl.Range, summary, format string, args ...any) {
		diags = append(diags, spec.Errorf(rng, summary, format, args...))
	}
	for _, id := range slices.Sorted(maps.Keys(m.Nodes)) {
		n := m.Nodes[id]
		if sp.Kinds[n.Kind] == nil {
			errorf(n.Range, "Undefined kind", "Node %q has undefined kind %q.", id, n.Kind)
		} else {
			kindPorts := sp.Ports(n.Kind)
			for _, p := range n.Ports {
				if slices.Contains(kindPorts, p.Name) {
					errorf(p.Range, "Duplicate port", "Node %q declares port %q, which its kind %q already has.", id, p.Name, n.Kind)
				}
			}
			if len(n.Compartments) > 0 && !sp.HasRows(n.Kind) {
				errorf(n.Compartments[0].Range, "Rows not allowed", "Node %q declares rows, but its kind %q does not set rows = true.", id, n.Kind)
			}
			for _, p := range n.Props {
				switch decl := sp.Property(n.Kind, p.Name); {
				case decl == nil:
					errorf(p.Range, "Undefined property", "Kind %q declares no property %q.", n.Kind, p.Name)
				case !p.Value.IsKnown() || p.Value.IsNull() || !p.Value.Type().Equals(decl.Type):
					errorf(p.Range, "Wrong property type", "Property %q of kind %q is a %s.", p.Name, n.Kind, decl.Type.FriendlyName())
				}
			}
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
		for _, r := range []Ref{e.From, e.To} {
			n := m.Nodes[r.Node]
			switch {
			case r.Range == (hcl.Range{}):
			case n == nil:
				errorf(r.Range, "Undefined node", "Edge %q references undefined node %q.", id, r.Node)
			case r.Port != "" && sp.Kinds[n.Kind] != nil && !slices.Contains(m.Ports(sp, n), r.Port):
				errorf(r.Range, "Undefined port", "Edge %q references port %q, which node %q does not have.", id, r.Port, r.Node)
			case r.Row != "" && !n.HasRow(r.Row):
				errorf(r.Range, "Undefined row", "Edge %q references row %q, which node %q does not have.", id, r.Row, r.Node)
			}
		}
	}
	return diags
}

// Contains reports whether node outer is an ancestor of node inner.
func (m *Model) Contains(outer, inner string) bool {
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
	return m.Contains(id, id)
}

// Package spec holds node kinds and edge kinds and their inheritance.
package spec

import (
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// RootKind is the kind every node kind extends.
const RootKind = "node"

// DefaultEdgeKind is the kind of an edge that names none.
const DefaultEdgeKind = "default"

// Sides are the valid values of a port side.
var Sides = []string{"top", "right", "bottom", "left", "auto"}

// Kind is a node kind as declared; inherited fields are read through Spec's methods.
type Kind struct {
	Name            string
	Extends         string
	Container       *bool
	Rows            *bool
	Ports           []Name
	DefaultPortSide string
	Properties      []*Property
	Range           hcl.Range
}

// Name is a declared name and where it is declared.
type Name struct {
	Name  string
	Range hcl.Range
}

// Property is a typed property a kind declares; Default is cty.NilVal when there is none.
type Property struct {
	Name    string
	Type    cty.Type
	Default cty.Value
	Range   hcl.Range
}

// EdgeKind is an edge kind.
type EdgeKind struct {
	Name  string
	Range hcl.Range
}

// Spec is the set of kinds loaded from packs and the project.
type Spec struct {
	Kinds     map[string]*Kind
	EdgeKinds map[string]*EdgeKind
	Overrides []Override
}

// Override records a definition that replaced or overrode one from an earlier file (spec 4.1).
type Override struct {
	What  string
	Name  string
	Range hcl.Range
	Prev  hcl.Range
}

// New returns an empty Spec.
func New() *Spec {
	return &Spec{Kinds: map[string]*Kind{}, EdgeKinds: map[string]*EdgeKind{}}
}

type kindBlock struct {
	Name            string          `hcl:"name,label"`
	Extends         *string         `hcl:"extends,optional"`
	Container       *bool           `hcl:"container,optional"`
	Rows            *bool           `hcl:"rows,optional"`
	Ports           *hcl.Attribute  `hcl:"ports,optional"`
	DefaultPortSide *hcl.Attribute  `hcl:"default_port_side,optional"`
	Properties      []propertyBlock `hcl:"property,block"`
	Range           hcl.Range       `hcl:",def_range"`
}

type propertyBlock struct {
	Name    string         `hcl:"name,label"`
	Type    *hcl.Attribute `hcl:"type,optional"`
	Default *hcl.Attribute `hcl:"default,optional"`
	Range   hcl.Range      `hcl:",def_range"`
}

type file struct {
	Kinds     []kindBlock `hcl:"kind,block"`
	EdgeKinds []struct {
		Name  string    `hcl:"name,label"`
		Range hcl.Range `hcl:",def_range"`
	} `hcl:"edge_kind,block"`
}

var types = map[string]cty.Type{"string": cty.String, "number": cty.Number, "bool": cty.Bool}

// Decode adds the kind and edge_kind blocks of body to s. A definition replaces one from an
// earlier file, reported as information; a name defined twice in one file is an error.
func (s *Spec) Decode(body hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics {
	var f file
	diags := gohcl.DecodeBody(body, ctx, &f)
	kinds := map[string]hcl.Range{}
	for _, b := range f.Kinds {
		if prev, ok := kinds[b.Name]; ok {
			diags = append(diags, duplicate("kind", b.Name, b.Range, prev))
			continue
		}
		kinds[b.Name] = b.Range
		diags = append(diags, CheckSimple("kind", b.Name, b.Range)...)
		k, d := decodeKind(b, ctx)
		diags = append(diags, d...)
		if prev := s.Kinds[b.Name]; prev != nil {
			s.Overrides = append(s.Overrides, Override{"kind", b.Name, b.Range, prev.Range})
		}
		s.Kinds[b.Name] = k
	}
	edgeKinds := map[string]hcl.Range{}
	for _, b := range f.EdgeKinds {
		if prev, ok := edgeKinds[b.Name]; ok {
			diags = append(diags, duplicate("edge kind", b.Name, b.Range, prev))
			continue
		}
		edgeKinds[b.Name] = b.Range
		diags = append(diags, CheckSimple("edge kind", b.Name, b.Range)...)
		if prev := s.EdgeKinds[b.Name]; prev != nil {
			s.Overrides = append(s.Overrides, Override{"edge kind", b.Name, b.Range, prev.Range})
		}
		s.EdgeKinds[b.Name] = &EdgeKind{Name: b.Name, Range: b.Range}
	}
	return diags
}

func decodeKind(b kindBlock, ctx *hcl.EvalContext) (*Kind, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	k := &Kind{Name: b.Name, Container: b.Container, Rows: b.Rows, Range: b.Range}
	if b.Name != RootKind {
		k.Extends = RootKind
	}
	if b.Extends != nil {
		k.Extends = *b.Extends
	}
	if b.Ports != nil {
		names, d := DecodeNames("port", b.Ports.Expr, ctx)
		diags = append(diags, d...)
		k.Ports = names
	}
	if b.DefaultPortSide != nil {
		side, d := DecodeEnum(b.DefaultPortSide.Expr, ctx, Sides)
		diags = append(diags, d...)
		k.DefaultPortSide = side
	}
	declared := map[string]hcl.Range{}
	for _, p := range b.Properties {
		if prev, ok := declared[p.Name]; ok {
			diags = append(diags, duplicate("property", p.Name, p.Range, prev))
			continue
		}
		declared[p.Name] = p.Range
		prop, d := decodeProperty(p, ctx)
		diags = append(diags, d...)
		if prop != nil {
			k.Properties = append(k.Properties, prop)
		}
	}
	return k, diags
}

func decodeProperty(b propertyBlock, ctx *hcl.EvalContext) (*Property, hcl.Diagnostics) {
	if b.Type == nil {
		return nil, hcl.Diagnostics{Errorf(b.Range, "Missing required argument", "The argument \"type\" is required.")}
	}
	typ, ok := types[hcl.ExprAsKeyword(b.Type.Expr)]
	if !ok {
		return nil, hcl.Diagnostics{Errorf(b.Type.Expr.Range(), "Invalid property type", "A property type is string, number or bool.")}
	}
	p := &Property{Name: b.Name, Type: typ, Range: b.Range}
	if b.Default == nil {
		return p, nil
	}
	v, diags := b.Default.Expr.Value(ctx)
	if diags.HasErrors() {
		return p, diags
	}
	if !v.IsKnown() || v.IsNull() || !v.Type().Equals(typ) {
		return p, hcl.Diagnostics{Errorf(b.Default.Expr.Range(), "Wrong property type", "The default of property %q must be a %s.", b.Name, typ.FriendlyName())}
	}
	p.Default = v
	return p, nil
}

// DecodeNames decodes a list of port names, reporting invalid and repeated ones.
func DecodeNames(what string, expr hcl.Expression, ctx *hcl.EvalContext) ([]Name, hcl.Diagnostics) {
	list, diags := DecodeStrings(expr, ctx)
	var names []Name
	seen := map[string]bool{}
	for _, n := range list {
		if problem := NameProblem(n.Name); problem != "" {
			diags = append(diags, Errorf(n.Range, "Invalid name", "The %s name %q %s.", what, n.Name, problem))
			continue
		}
		if seen[n.Name] {
			diags = append(diags, Errorf(n.Range, "Duplicate "+what, "The %s %q is listed twice.", what, n.Name))
			continue
		}
		seen[n.Name] = true
		names = append(names, n)
	}
	return names, diags
}

// DecodeStrings decodes a list of strings, each with the range of its element when the list is
// written out, or of the whole expression when it is computed (as by port_range).
func DecodeStrings(expr hcl.Expression, ctx *hcl.EvalContext) ([]Name, hcl.Diagnostics) {
	if tuple, ok := expr.(*hclsyntax.TupleConsExpr); ok {
		var out []Name
		var diags hcl.Diagnostics
		for _, e := range tuple.Exprs {
			var s string
			d := DecodeExpr(e, ctx, &s)
			diags = append(diags, d...)
			if !d.HasErrors() {
				out = append(out, Name{Name: s, Range: e.Range()})
			}
		}
		return out, diags
	}
	var list []string
	diags := DecodeExpr(expr, ctx, &list)
	var out []Name
	for _, s := range list {
		out = append(out, Name{Name: s, Range: expr.Range()})
	}
	return out, diags
}

// DecodeEnum decodes a string that must be one of values.
func DecodeEnum(expr hcl.Expression, ctx *hcl.EvalContext, values []string) (string, hcl.Diagnostics) {
	var v string
	if diags := DecodeExpr(expr, ctx, &v); diags.HasErrors() {
		return "", diags
	}
	if !slices.Contains(values, v) {
		return "", hcl.Diagnostics{Errorf(expr.Range(), "Invalid value", "%q is not one of %s.", v, QuoteAll(values))}
	}
	return v, nil
}

// QuoteAll returns values quoted and separated by commas.
func QuoteAll(values []string) string {
	s := ""
	for i, v := range values {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%q", v)
	}
	return s
}

func duplicate(what, name string, rng, prev hcl.Range) *hcl.Diagnostic {
	return Errorf(rng, "Duplicate definition", "The %s %q is already defined at line %d of this file.", what, name, prev.Start.Line)
}

// Validate reports undefined parents, extends cycles, and ports or properties a kind redeclares.
func (s *Spec) Validate() hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, name := range slices.Sorted(maps.Keys(s.Kinds)) {
		k := s.Kinds[name]
		if k.Extends == "" {
			continue
		}
		parent := s.Kinds[k.Extends]
		if parent == nil {
			diags = append(diags, Errorf(k.Range, "Undefined kind", "Kind %q extends undefined kind %q.", name, k.Extends))
			continue
		}
		if s.inCycle(name) {
			diags = append(diags, Errorf(k.Range, "Extends cycle", "Kind %q extends itself through its ancestors.", name))
			continue
		}
		inherited := s.Ports(k.Extends)
		for _, p := range k.Ports {
			if slices.Contains(inherited, p.Name) {
				diags = append(diags, Errorf(p.Range, "Redeclared port", "Kind %q redeclares port %q inherited from kind %q.", name, p.Name, k.Extends))
			}
		}
		for _, p := range k.Properties {
			if s.Property(k.Extends, p.Name) != nil {
				diags = append(diags, Errorf(p.Range, "Redeclared property", "Kind %q redeclares property %q inherited from kind %q.", name, p.Name, k.Extends))
			}
		}
	}
	return diags
}

func (s *Spec) inCycle(name string) bool {
	seen := map[string]bool{}
	for k := s.Kinds[name]; k != nil && !seen[k.Name]; k = s.Kinds[k.Extends] {
		seen[k.Name] = true
		if k.Extends == name {
			return true
		}
	}
	return false
}

// IsContainer reports whether kind is a container, as set by the nearest kind in its lineage.
func (s *Spec) IsContainer(kind string) bool {
	return s.nearest(kind, func(k *Kind) *bool { return k.Container })
}

// HasRows reports whether nodes of kind may declare rows, as set by the nearest kind in its lineage.
func (s *Spec) HasRows(kind string) bool {
	return s.nearest(kind, func(k *Kind) *bool { return k.Rows })
}

func (s *Spec) nearest(kind string, field func(*Kind) *bool) bool {
	chain := s.Lineage(kind)
	for i := len(chain) - 1; i >= 0; i-- {
		if v := field(chain[i]); v != nil {
			return *v
		}
	}
	return false
}

// Ports returns the ports of kind, inherited ones first.
func (s *Spec) Ports(kind string) []string {
	var ports []string
	for _, k := range s.Lineage(kind) {
		for _, p := range k.Ports {
			if !slices.Contains(ports, p.Name) {
				ports = append(ports, p.Name)
			}
		}
	}
	return ports
}

// Property returns the property name that kind declares or inherits, or nil.
func (s *Spec) Property(kind, name string) *Property {
	chain := s.Lineage(kind)
	for i := len(chain) - 1; i >= 0; i-- {
		for _, p := range chain[i].Properties {
			if p.Name == name {
				return p
			}
		}
	}
	return nil
}

// IsA reports whether kind is ancestor or one of its sub-kinds.
func (s *Spec) IsA(kind, ancestor string) bool {
	return slices.ContainsFunc(s.Lineage(kind), func(k *Kind) bool { return k.Name == ancestor })
}

// EdgeLineage returns the defined edge kinds from default down to name.
func (s *Spec) EdgeLineage(name string) []*EdgeKind {
	var chain []*EdgeKind
	if name != DefaultEdgeKind && s.EdgeKinds[DefaultEdgeKind] != nil {
		chain = append(chain, s.EdgeKinds[DefaultEdgeKind])
	}
	if k := s.EdgeKinds[name]; k != nil {
		chain = append(chain, k)
	}
	return chain
}

// Lineage returns the defined kinds from the root down to name, stopping at a cycle.
func (s *Spec) Lineage(name string) []*Kind {
	var chain []*Kind
	seen := map[string]bool{}
	for k := s.Kinds[name]; k != nil && !seen[k.Name]; k = s.Kinds[k.Extends] {
		seen[k.Name] = true
		chain = append([]*Kind{k}, chain...)
	}
	return chain
}

// DefaultPortSide returns the side for instance ports of kind that set none: the nearest
// default_port_side in its lineage, or "auto".
func (s *Spec) DefaultPortSide(kind string) string {
	chain := s.Lineage(kind)
	for i := len(chain) - 1; i >= 0; i-- {
		if side := chain[i].DefaultPortSide; side != "" {
			return side
		}
	}
	return "auto"
}

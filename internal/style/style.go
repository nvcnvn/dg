// Package style holds style rules and computes the style cascade (spec 8.4).
package style

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/nvcnvn/dg/internal/spec"
)

// Node is the computed style of a node.
type Node struct {
	Shape  string `json:"shape"`
	Fill   string `json:"fill"`
	Stroke string `json:"stroke"`
}

// Edge is the computed style of an edge.
type Edge struct {
	Stroke   string `json:"stroke"`
	ArrowEnd string `json:"arrow_end"`
}

// prop is a style property and its valid values.
type prop struct {
	name  string
	valid func(string) bool
}

var (
	color     = regexp.MustCompile(`^(#[0-9A-Fa-f]{3}|#[0-9A-Fa-f]{6}|none)$`).MatchString
	nodeProps = []prop{{"shape", oneOf("rect", "rounded")}, {"fill", color}, {"stroke", color}}
	edgeProps = []prop{{"stroke", color}, {"arrow_end", oneOf("none", "arrow")}}
)

func oneOf(values ...string) func(string) bool {
	return func(v string) bool { return slices.Contains(values, v) }
}

type rule struct {
	label string
	rng   hcl.Range
	props map[string]string
}

// Styles is the ordered list of style rules from packs and the project; later rules win.
type Styles struct {
	node []rule
	edge []rule
}

type block struct {
	Label string    `hcl:"label,label"`
	Body  hcl.Body  `hcl:",remain"`
	Range hcl.Range `hcl:",def_range"`
}

type file struct {
	Node []block `hcl:"style,block"`
	Edge []block `hcl:"edge_style,block"`
}

// Decode appends the style and edge_style blocks of body to s.
func (s *Styles) Decode(body hcl.Body) hcl.Diagnostics {
	var f file
	diags := gohcl.DecodeBody(body, nil, &f)
	node, d := decodeRules(f.Node, nodeProps)
	diags = append(diags, d...)
	edge, d := decodeRules(f.Edge, edgeProps)
	diags = append(diags, d...)
	s.node = append(s.node, node...)
	s.edge = append(s.edge, edge...)
	return diags
}

func decodeRules(blocks []block, props []prop) ([]rule, hcl.Diagnostics) {
	schema := &hcl.BodySchema{}
	for _, p := range props {
		schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: p.name})
	}
	var rules []rule
	var diags hcl.Diagnostics
	for _, b := range blocks {
		content, d := b.Body.Content(schema)
		diags = append(diags, d...)
		r := rule{label: b.Label, rng: b.Range, props: map[string]string{}}
		for _, p := range props {
			attr := content.Attributes[p.name]
			if attr == nil {
				continue
			}
			var v string
			if d := gohcl.DecodeExpression(attr.Expr, nil, &v); d.HasErrors() {
				diags = append(diags, d...)
				continue
			}
			if !p.valid(v) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid style value",
					Detail:   fmt.Sprintf("%q is not a valid value for %s.", v, p.name),
					Subject:  attr.Expr.Range().Ptr(),
				})
				continue
			}
			r.props[p.name] = v
		}
		rules = append(rules, r)
	}
	return rules, diags
}

// Validate reports rules whose label is not a kind, the only label form supported so far.
func (s *Styles) Validate(sp *spec.Spec) hcl.Diagnostics {
	var diags hcl.Diagnostics
	check := func(label string, rng hcl.Range, known bool) {
		switch {
		case strings.Contains(label, "."):
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unsupported style label",
				Detail:   fmt.Sprintf("Style label %q: only kind labels are supported.", label),
				Subject:  rng.Ptr(),
			})
		case !known:
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "Undefined kind",
				Detail:   fmt.Sprintf("Style label %q names no defined kind.", label),
				Subject:  rng.Ptr(),
			})
		}
	}
	for _, r := range s.node {
		check(r.label, r.rng, sp.Kinds[r.label] != nil)
	}
	for _, r := range s.edge {
		check(r.label, r.rng, sp.EdgeKinds[r.label] != nil)
	}
	return diags
}

// ForNode returns the style of a node of kind: its kinds' styles from the root down.
func (s *Styles) ForNode(sp *spec.Spec, kind string) Node {
	var names []string
	for _, k := range sp.Lineage(kind) {
		names = append(names, k.Name)
	}
	p := cascade(s.node, names)
	return Node{Shape: p["shape"], Fill: p["fill"], Stroke: p["stroke"]}
}

// ForEdge returns the style of an edge of kind: the default edge kind's style, then its own.
func (s *Styles) ForEdge(sp *spec.Spec, kind string) Edge {
	var names []string
	for _, k := range sp.EdgeLineage(kind) {
		names = append(names, k.Name)
	}
	p := cascade(s.edge, names)
	return Edge{Stroke: p["stroke"], ArrowEnd: p["arrow_end"]}
}

func cascade(rules []rule, labels []string) map[string]string {
	props := map[string]string{}
	for _, label := range labels {
		for _, r := range rules {
			if r.label == label {
				maps.Copy(props, r.props)
			}
		}
	}
	return props
}

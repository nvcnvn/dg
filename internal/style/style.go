// Package style holds style rules and custom shapes and computes the style cascade (spec 8.4).
package style

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/spec"
)

// Node is the computed style of a node, as far as the renderer draws it.
type Node struct {
	Shape        string    `json:"shape"`
	Fill         string    `json:"fill"`
	Stroke       string    `json:"stroke"`
	StrokeWidth  float64   `json:"stroke_width"`
	StrokeDash   []float64 `json:"stroke_dash"`
	CornerRadius float64   `json:"corner_radius"`
	Opacity      float64   `json:"opacity"`
	TextColor    string    `json:"text_color"`
	FontSize     float64   `json:"font_size"`
	FontWeight   float64   `json:"font_weight"`
	Icon         string    `json:"icon,omitempty"`
	Image        string    `json:"image,omitempty"`
	Padding      float64   `json:"padding"`
	MinW         float64   `json:"min_w"`
	MinH         float64   `json:"min_h"`
	MaxW         float64   `json:"max_w"`
	Z            float64   `json:"z"`
}

// Edge is the computed style of an edge, as far as the renderer draws it.
type Edge struct {
	Stroke      string    `json:"stroke"`
	StrokeWidth float64   `json:"stroke_width"`
	StrokeDash  []float64 `json:"stroke_dash"`
	ArrowStart  string    `json:"arrow_start"`
	ArrowEnd    string    `json:"arrow_end"`
	Routing     string    `json:"routing"`
	FontSize    float64   `json:"font_size"`
	Animate     string    `json:"animate,omitempty"`
}

// Overlay is the computed style of an overlay.
type Overlay struct {
	Fill        string    `json:"fill"`
	Stroke      string    `json:"stroke"`
	StrokeWidth float64   `json:"stroke_width"`
	StrokeDash  []float64 `json:"stroke_dash"`
	Opacity     float64   `json:"opacity"`
	Padding     float64   `json:"padding"`
}

// Setting is one property value of a rule and where it is set.
type Setting struct {
	Value cty.Value
	Range hcl.Range
}

// Rule is a style, edge_style or overlay_style block.
type Rule struct {
	Label string
	Range hcl.Range
	Props map[string]Setting
}

// Shape is a custom shape block. TextBox and Anchors are fractions of the drawn box.
type Shape struct {
	Name         string                `json:"-"`
	SVG          string                `json:"svg"`
	TextBox      [4]float64            `json:"text_box"`
	Anchors      map[string][2]float64 `json:"anchors,omitempty"`
	Stretch      string                `json:"stretch"`
	AnchorsRange hcl.Range             `json:"-"`
	Range        hcl.Range             `json:"-"`
}

// Styles is the ordered list of rules from packs and the project; later rules win.
type Styles struct {
	Node      []Rule
	Edge      []Rule
	Overlay   []Rule
	Shapes    map[string]*Shape
	Overrides []spec.Override
}

// New returns empty Styles.
func New() *Styles {
	return &Styles{Shapes: map[string]*Shape{}}
}

// Block is a style block as decoded by gohcl; views.hcl reuses it for view rules.
type Block struct {
	Label string    `hcl:"label,label"`
	Body  hcl.Body  `hcl:",remain"`
	Range hcl.Range `hcl:",def_range"`
}

type shapeBlock struct {
	Name    string         `hcl:"name,label"`
	SVG     *hcl.Attribute `hcl:"svg"`
	TextBox *hcl.Attribute `hcl:"text_box,optional"`
	Anchors *hcl.Attribute `hcl:"anchors,optional"`
	Stretch *hcl.Attribute `hcl:"stretch,optional"`
	Range   hcl.Range      `hcl:",def_range"`
}

type file struct {
	Node    []Block      `hcl:"style,block"`
	Edge    []Block      `hcl:"edge_style,block"`
	Overlay []Block      `hcl:"overlay_style,block"`
	Shapes  []shapeBlock `hcl:"shape,block"`
}

// Decode appends the style, edge_style, overlay_style and shape blocks of body to s. A label
// defined in an earlier file is reported as information; one defined twice in a file is an error.
func (s *Styles) Decode(body hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics {
	var f file
	diags := gohcl.DecodeBody(body, ctx, &f)
	for _, set := range []struct {
		what   string
		blocks []Block
		props  map[string]Prop
		rules  *[]Rule
	}{{"style", f.Node, NodeProps, &s.Node}, {"edge_style", f.Edge, EdgeProps, &s.Edge}, {"overlay_style", f.Overlay, OverlayProps, &s.Overlay}} {
		rules, d := DecodeRules(set.what, set.blocks, set.props, ctx)
		diags = append(diags, d...)
		for _, r := range rules {
			if set.what == "overlay_style" {
				diags = append(diags, spec.CheckSimple("overlay style", r.Label, r.Range)...)
			}
			if i := slices.IndexFunc(*set.rules, func(prev Rule) bool { return prev.Label == r.Label }); i >= 0 {
				s.Overrides = append(s.Overrides, spec.Override{What: set.what, Name: r.Label, Range: r.Range, Prev: (*set.rules)[i].Range})
			}
		}
		*set.rules = append(*set.rules, rules...)
	}
	seen := map[string]hcl.Range{}
	for _, b := range f.Shapes {
		if prev, ok := seen[b.Name]; ok {
			diags = append(diags, duplicate("shape", b.Name, b.Range, prev))
			continue
		}
		seen[b.Name] = b.Range
		sh, d := decodeShape(b, ctx)
		diags = append(diags, d...)
		if prev := s.Shapes[b.Name]; prev != nil {
			s.Overrides = append(s.Overrides, spec.Override{What: "shape", Name: b.Name, Range: b.Range, Prev: prev.Range})
		}
		s.Shapes[b.Name] = sh
	}
	return diags
}

// DecodeRules decodes blocks whose attributes are the properties in props.
func DecodeRules(what string, blocks []Block, props map[string]Prop, ctx *hcl.EvalContext) ([]Rule, hcl.Diagnostics) {
	schema := &hcl.BodySchema{}
	for _, name := range slices.Sorted(maps.Keys(props)) {
		schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: name})
	}
	var rules []Rule
	var diags hcl.Diagnostics
	seen := map[string]hcl.Range{}
	for _, b := range blocks {
		if prev, ok := seen[b.Label]; ok {
			diags = append(diags, duplicate(what, b.Label, b.Range, prev))
			continue
		}
		seen[b.Label] = b.Range
		content, d := b.Body.Content(schema)
		diags = append(diags, d...)
		r := Rule{Label: b.Label, Range: b.Range, Props: map[string]Setting{}}
		for _, name := range slices.Sorted(maps.Keys(content.Attributes)) {
			attr := content.Attributes[name]
			v, d := attr.Expr.Value(ctx)
			if d.HasErrors() {
				diags = append(diags, d...)
				continue
			}
			if problem := props[name].check(v); problem != "" {
				diags = append(diags, spec.Errorf(attr.Expr.Range(), "Invalid style value", "The value of %s %s.", name, problem))
				continue
			}
			r.Props[name] = Setting{Value: v, Range: attr.Expr.Range()}
		}
		rules = append(rules, r)
	}
	return rules, diags
}

func decodeShape(b shapeBlock, ctx *hcl.EvalContext) (*Shape, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	diags = append(diags, spec.CheckSimple("shape", b.Name, b.Range)...)
	if slices.Contains(BuiltinShapes, b.Name) {
		diags = append(diags, spec.Errorf(b.Range, "Invalid ID", "The shape %q is a built-in shape.", b.Name))
	}
	sh := &Shape{Name: b.Name, TextBox: [4]float64{0, 0, 1, 1}, Stretch: "nine-slice", Range: b.Range}
	value := func(attr *hcl.Attribute, valid func(cty.Value) string) (cty.Value, bool) {
		if attr == nil {
			return cty.NilVal, false
		}
		v, d := attr.Expr.Value(ctx)
		diags = append(diags, d...)
		if d.HasErrors() {
			return cty.NilVal, false
		}
		if problem := valid(v); problem != "" {
			diags = append(diags, spec.Errorf(attr.Expr.Range(), "Invalid style value", "The value of %s %s.", attr.Name, problem))
			return cty.NilVal, false
		}
		return v, true
	}
	if v, ok := value(b.SVG, picture); ok {
		sh.SVG = v.AsString()
	}
	if v, ok := value(b.TextBox, numbers(4, 0, 1)); ok {
		copy(sh.TextBox[:], floats(v))
	}
	if v, ok := value(b.Stretch, enum("nine-slice", "uniform", "none")); ok {
		sh.Stretch = v.AsString()
	}
	if b.Anchors != nil {
		sh.AnchorsRange = b.Anchors.Expr.Range()
		v, ok := value(b.Anchors, func(v cty.Value) string {
			if !v.Type().IsObjectType() && !v.Type().IsMapType() {
				return "must be an object of [x, y] fractions"
			}
			anchors := v.AsValueMap()
			for _, k := range slices.Sorted(maps.Keys(anchors)) {
				if p := numbers(2, 0, 1)(anchors[k]); p != "" {
					return fmt.Sprintf("for anchor %q %s", k, p)
				}
			}
			return ""
		})
		if ok {
			sh.Anchors = map[string][2]float64{}
			for k, a := range v.AsValueMap() {
				xy := floats(a)
				sh.Anchors[k] = [2]float64{xy[0], xy[1]}
			}
		}
	}
	return sh, diags
}

func duplicate(what, label string, rng, prev hcl.Range) *hcl.Diagnostic {
	return spec.Errorf(rng, "Duplicate definition", "The %s %q is already defined at line %d of this file.", what, label, prev.Start.Line)
}

// Validate reports invalid labels, styles of undefined kinds, unknown shapes, and custom shape
// anchors that name no port of a kind drawn with that shape.
func (s *Styles) Validate(sp *spec.Spec) hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, r := range s.Node {
		diags = append(diags, CheckLabel(r, false, sp, nil)...)
		diags = append(diags, s.CheckShape(r)...)
	}
	for _, r := range s.Edge {
		diags = append(diags, CheckLabel(r, true, sp, nil)...)
	}
	for _, kind := range slices.Sorted(maps.Keys(sp.Kinds)) {
		set, ok := s.ForNode(sp, kind, nil)["shape"]
		if !ok || s.Shapes[set.Value.AsString()] == nil {
			continue
		}
		sh := s.Shapes[set.Value.AsString()]
		ports := sp.Ports(kind)
		for _, a := range slices.Sorted(maps.Keys(sh.Anchors)) {
			if !slices.Contains(ports, a) {
				diags = append(diags, spec.Errorf(sh.AnchorsRange, "Anchor without port", "Shape %q has anchor %q, but kind %q, drawn with it, has no such port.", sh.Name, a, kind))
			}
		}
	}
	return diags
}

// CheckShape reports a shape property naming neither a built-in nor a custom shape.
func (s *Styles) CheckShape(r Rule) hcl.Diagnostics {
	set, ok := r.Props["shape"]
	if !ok {
		return nil
	}
	name := set.Value.AsString()
	if slices.Contains(BuiltinShapes, name) || s.Shapes[name] != nil {
		return nil
	}
	return hcl.Diagnostics{spec.Errorf(set.Range, "Invalid style value", "The shape %q is neither a built-in shape nor defined by a shape block.", name)}
}

// CheckLabel reports an invalid label, or one naming an undefined kind (a warning). Outside a
// view nodeExists is nil, and node.<id> and edge.<id> labels are errors.
func CheckLabel(r Rule, edge bool, sp *spec.Spec, exists func(string) bool) hcl.Diagnostics {
	elem := "node"
	if edge {
		elem = "edge"
	}
	if tag, ok := strings.CutPrefix(r.Label, "tag."); ok {
		return spec.CheckSimple("tag", tag, r.Range)
	}
	if id, ok := strings.CutPrefix(r.Label, elem+"."); ok {
		switch {
		case exists == nil:
			return hcl.Diagnostics{spec.Errorf(r.Range, "Invalid style label", "The label %q names one %s; such rules are only allowed in a view.", r.Label, elem)}
		case !exists(id):
			return hcl.Diagnostics{spec.Errorf(r.Range, "Undefined "+elem, "The style label %q names an undefined %s.", r.Label, elem)}
		}
		return nil
	}
	if !spec.IsSimpleID(r.Label) {
		return hcl.Diagnostics{spec.Errorf(r.Range, "Invalid style label", "The label %q is not a kind, tag.<tag> or %s.<id>.", r.Label, elem)}
	}
	if (edge && sp.EdgeKinds[r.Label] == nil) || (!edge && sp.Kinds[r.Label] == nil) {
		return hcl.Diagnostics{spec.Warnf(r.Range, "Undefined kind", "Style label %q names no defined kind.", r.Label)}
	}
	return nil
}

// ForNode returns the settings for a node of kind with tags: its kinds' styles from the root
// down, then its tags' styles in order.
func (s *Styles) ForNode(sp *spec.Spec, kind string, tags []string) map[string]Setting {
	var labels []string
	for _, k := range sp.Lineage(kind) {
		labels = append(labels, k.Name)
	}
	return Cascade(s.Node, append(labels, tagLabels(tags)...))
}

// ForEdge returns the settings for an edge of kind with tags: the default edge kind's style,
// its own kind's, then its tags' styles in order.
func (s *Styles) ForEdge(sp *spec.Spec, kind string, tags []string) map[string]Setting {
	var labels []string
	for _, k := range sp.EdgeLineage(kind) {
		labels = append(labels, k.Name)
	}
	return Cascade(s.Edge, append(labels, tagLabels(tags)...))
}

func tagLabels(tags []string) []string {
	labels := make([]string, len(tags))
	for i, t := range tags {
		labels[i] = "tag." + t
	}
	return labels
}

// Cascade applies the rules matching each label in turn, later settings winning.
func Cascade(rules []Rule, labels []string) map[string]Setting {
	props := map[string]Setting{}
	for _, label := range labels {
		for _, r := range rules {
			if r.Label == label {
				maps.Copy(props, r.Props)
			}
		}
	}
	return props
}

// DrawnNode returns the drawn part of a node's settings.
func DrawnNode(props map[string]Setting) Node {
	return Node{
		Shape:        str(props, "shape"),
		Fill:         str(props, "fill"),
		Stroke:       str(props, "stroke"),
		StrokeWidth:  num(props, "stroke_width"),
		StrokeDash:   list(props, "stroke_dash"),
		CornerRadius: num(props, "corner_radius"),
		Opacity:      num(props, "opacity"),
		TextColor:    str(props, "text_color"),
		FontSize:     num(props, "font_size"),
		FontWeight:   num(props, "font_weight"),
		Icon:         str(props, "icon"),
		Image:        str(props, "image"),
		Padding:      num(props, "padding"),
		MinW:         num(props, "min_w"),
		MinH:         num(props, "min_h"),
		MaxW:         num(props, "max_w"),
		Z:            num(props, "z"),
	}
}

// DrawnEdge returns the drawn part of an edge's settings.
func DrawnEdge(props map[string]Setting) Edge {
	return Edge{
		Stroke:      str(props, "stroke"),
		StrokeWidth: num(props, "stroke_width"),
		StrokeDash:  list(props, "stroke_dash"),
		ArrowStart:  str(props, "arrow_start"),
		ArrowEnd:    str(props, "arrow_end"),
		Routing:     str(props, "routing"),
		FontSize:    num(props, "font_size"),
		Animate:     animate(props),
	}
}

// DrawnOverlay returns an overlay's style from its settings.
func DrawnOverlay(props map[string]Setting) Overlay {
	return Overlay{
		Fill:        str(props, "fill"),
		Stroke:      str(props, "stroke"),
		StrokeWidth: num(props, "stroke_width"),
		StrokeDash:  list(props, "stroke_dash"),
		Opacity:     num(props, "opacity"),
		Padding:     num(props, "padding"),
	}
}

// animate returns the edge's animation, or "" for none.
func animate(props map[string]Setting) string {
	if a := str(props, "animate"); a != "none" {
		return a
	}
	return ""
}

func str(props map[string]Setting, name string) string {
	if set, ok := props[name]; ok {
		return set.Value.AsString()
	}
	return ""
}

func num(props map[string]Setting, name string) float64 {
	if set, ok := props[name]; ok {
		f, _ := float(set.Value)
		return f
	}
	return 0
}

func list(props map[string]Setting, name string) []float64 {
	if set, ok := props[name]; ok {
		return floats(set.Value)
	}
	return []float64{}
}

// floats returns the numbers of a list that a value check has accepted.
func floats(v cty.Value) []float64 {
	out := []float64{}
	for _, e := range v.AsValueSlice() {
		f, _ := float(e)
		out = append(out, f)
	}
	return out
}

var colorRE = regexp.MustCompile(`^(#[0-9A-Fa-f]{3}|#[0-9A-Fa-f]{6}|none)$`)

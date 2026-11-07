package style

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/spec"
)

// Prop is a style property: its value check and, for completion, the values it takes when it
// takes few, as HCL expressions.
type Prop struct {
	check  func(cty.Value) string
	Values []string
}

// BuiltinShapes are the shapes a style may name without a shape block (spec 4.3).
var BuiltinShapes = []string{"rect", "rounded", "stadium", "ellipse", "diamond", "cylinder", "hexagon", "parallelogram", "document", "person", "cloud", "text", "image", "point", "bullseye", "table"}

var markers = []string{"none", "arrow", "triangle", "triangle-open", "diamond", "diamond-open", "circle", "circle-open", "crow-one", "crow-zero-one", "crow-many", "crow-one-many", "crow-zero-many"}

var (
	colorProp = Prop{color, []string{`"none"`}}
	boolProp  = Prop{isBool, []string{"true", "false"}}
)

func check(f func(cty.Value) string) Prop { return Prop{check: f} }

func oneOf(values ...string) Prop {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = `"` + v + `"`
	}
	return Prop{enum(values...), quoted}
}

// NodeProps are the properties of style blocks: node properties and container properties.
var NodeProps = map[string]Prop{
	"shape":           {isString, oneOf(BuiltinShapes...).Values},
	"fill":            colorProp,
	"stroke":          colorProp,
	"stroke_width":    check(number(0, inf)),
	"stroke_dash":     check(dash),
	"corner_radius":   check(number(0, inf)),
	"opacity":         check(number(0, 1)),
	"text_color":      colorProp,
	"font_size":       check(number(1, inf)),
	"font_weight":     {weight, []string{"100", "200", "300", "400", "500", "600", "700", "800", "900"}},
	"sub_text_color":  colorProp,
	"sub_font_size":   check(number(1, inf)),
	"sub_font_weight": {weight, []string{"100", "200", "300", "400", "500", "600", "700", "800", "900"}},
	"icon":            check(picture),
	"image":           check(picture),
	"padding":         check(number(0, inf)),
	"min_w":           check(number(0, inf)),
	"min_h":           check(number(0, inf)),
	"max_w":           check(number(0, inf)),
	"z":               check(integer(-inf, inf)),
	"layout":          oneOf("free", "stack", "grid"),
	"direction":       oneOf("horizontal", "vertical"),
	"gap":             check(number(0, inf)),
	"stretch":         boolProp,
	"columns":         check(integer(1, inf)),
	"align":           oneOf("start", "center", "end"),
	"label_side":      oneOf("top", "left"),
}

// EdgeProps are the properties of edge_style blocks.
var EdgeProps = map[string]Prop{
	"stroke":       colorProp,
	"stroke_width": check(number(0, inf)),
	"stroke_dash":  check(dash),
	"arrow_start":  oneOf(markers...),
	"arrow_end":    oneOf(markers...),
	"routing":      oneOf("orthogonal", "polyline", "straight"),
	"font_size":    check(number(1, inf)),
	"animate":      oneOf("none", "flow"),
}

// OverlayProps are the properties of overlay_style blocks.
var OverlayProps = map[string]Prop{
	"fill":         colorProp,
	"stroke":       colorProp,
	"stroke_width": check(number(0, inf)),
	"stroke_dash":  check(dash),
	"opacity":      check(number(0, 1)),
	"padding":      check(number(0, inf)),
}

const inf = 1e18

func known(v cty.Value) bool { return v.IsWhollyKnown() && !v.IsNull() }

func isString(v cty.Value) string {
	if !known(v) || v.Type() != cty.String {
		return "must be a string"
	}
	return ""
}

// PictureProblem says why svg is not a picture a browser can draw, or returns "".
func PictureProblem(svg string) string {
	return picture(cty.StringVal(svg))
}

// ColorProblem says why s is not a color, or returns "".
func ColorProblem(s string) string {
	return color(cty.StringVal(s))
}

// picture accepts the text of an SVG file: well-formed XML whose root element is svg in the SVG
// namespace, which a browser needs to draw it as a picture.
func picture(v cty.Value) string {
	if p := isString(v); p != "" {
		return p
	}
	root, problem := pictureRoot(v.AsString())
	if problem != "" {
		return "is not well-formed SVG: " + problem
	}
	if root != (xml.Name{Space: svgNS, Local: "svg"}) {
		return `must be an SVG picture, whose root element is svg with xmlns="http://www.w3.org/2000/svg"`
	}
	return ""
}

const (
	svgNS   = "http://www.w3.org/2000/svg"
	xmlNS   = "http://www.w3.org/XML/1998/namespace"
	xmlnsNS = "http://www.w3.org/2000/xmlns/"
)

// pictureRoot returns the root element of the XML document svg, or what is wrong with it.
// encoding/xml takes a sequence of elements rather than a document, and neither binds prefixes nor
// looks for duplicate attributes, so this does that over its raw tokens, on the input's own bytes
// where a token hides them; a browser refuses a file that has any of these wrong.
func pictureRoot(svg string) (root xml.Name, problem string) {
	svg = strings.TrimPrefix(svg, "\uFEFF")
	dec := xml.NewDecoder(strings.NewReader(svg))
	line := 1
	wrong := func(format string, args ...any) (xml.Name, string) {
		return root, fmt.Sprintf("line %d: ", line) + fmt.Sprintf(format, args...)
	}
	type element struct {
		name  xml.Name
		bound int // len(bound) before this element's declarations
	}
	var open []element
	var bound []string // the prefixes the open elements declare, innermost last
	ns := map[string][]string{"xml": {xmlNS}, "xmlns": {xmlnsNS}}
	expand := func(n xml.Name) (xml.Name, bool) {
		uris := ns[n.Space]
		if len(uris) == 0 {
			return n, n.Space == ""
		}
		return xml.Name{Space: uris[len(uris)-1], Local: n.Local}, true
	}
	attrs := map[xml.Name]bool{}
	doctype := false
	for {
		start := dec.InputOffset()
		line, _ = dec.InputPos()
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return root, err.Error()
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(open) == 0 && root.Local != "" {
				return wrong("more than one root element")
			}
			open = append(open, element{t.Name, len(bound)})
			for _, a := range t.Attr {
				prefix, declares := "", a.Name.Space == "" && a.Name.Local == "xmlns"
				if a.Name.Space == "xmlns" {
					prefix, declares = a.Name.Local, true
					if a.Value == "" {
						return wrong("prefix %s bound to no namespace", prefix)
					}
				}
				if declares {
					ns[prefix] = append(ns[prefix], a.Value)
					bound = append(bound, prefix)
				}
			}
			name, ok := expand(t.Name)
			if !ok || strings.Contains(t.Name.Local, ":") {
				return wrong("the name of element <%s> has no namespace bound", qualified(t.Name))
			}
			if root.Local == "" {
				root = name
			}
			clear(attrs)
			for _, a := range t.Attr {
				key := a.Name
				if key.Space != "" {
					key, ok = expand(key)
				}
				if !ok || strings.Contains(a.Name.Local, ":") {
					return wrong("the name of attribute %s has no namespace bound", qualified(a.Name))
				}
				if attrs[key] {
					return wrong("element <%s> repeats attribute %s", qualified(t.Name), qualified(a.Name))
				}
				attrs[key] = true
			}
		case xml.EndElement:
			if len(open) == 0 || open[len(open)-1].name != t.Name {
				return wrong("</%s> closes no open element", qualified(t.Name))
			}
			last := open[len(open)-1]
			open = open[:len(open)-1]
			for _, prefix := range bound[last.bound:] {
				ns[prefix] = ns[prefix][:len(ns[prefix])-1]
			}
			bound = bound[:last.bound]
		case xml.CharData:
			if len(open) == 0 && strings.Trim(svg[start:dec.InputOffset()], " \t\r\n") != "" {
				return wrong("text outside the root element")
			}
		case xml.ProcInst:
			if t.Target == "xml" && start != 0 {
				return wrong("an XML declaration that does not come first")
			}
		case xml.Directive:
			if doctype || root.Local != "" || !bytes.HasPrefix(t, []byte("DOCTYPE")) {
				return wrong("a directive that is not one doctype before the root element")
			}
			doctype = true
		}
	}
	if len(open) > 0 {
		return wrong("element <%s> is not closed", qualified(open[len(open)-1].name))
	}
	return root, ""
}

func qualified(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	return n.Space + ":" + n.Local
}

func isBool(v cty.Value) string {
	if !known(v) || v.Type() != cty.Bool {
		return "must be true or false"
	}
	return ""
}

func color(v cty.Value) string {
	if isString(v) != "" || !colorRE.MatchString(v.AsString()) {
		return `must be "#RGB", "#RRGGBB" or "none"`
	}
	return ""
}

func enum(values ...string) func(cty.Value) string {
	return func(v cty.Value) string {
		if isString(v) != "" || !slices.Contains(values, v.AsString()) {
			return "must be one of " + spec.QuoteAll(values)
		}
		return ""
	}
}

func float(v cty.Value) (float64, bool) {
	if !known(v) || v.Type() != cty.Number {
		return 0, false
	}
	f, _ := v.AsBigFloat().Float64()
	return f, true
}

func rangeText(lo, hi float64) string {
	switch {
	case lo == -inf:
		return "a number"
	case hi == inf:
		return fmt.Sprintf("a number of at least %g", lo)
	}
	return fmt.Sprintf("a number from %g to %g", lo, hi)
}

func number(lo, hi float64) func(cty.Value) string {
	return func(v cty.Value) string {
		if f, ok := float(v); !ok || f < lo || f > hi {
			return "must be " + rangeText(lo, hi)
		}
		return ""
	}
}

func integer(lo, hi float64) func(cty.Value) string {
	return func(v cty.Value) string {
		if f, ok := float(v); !ok || f < lo || f > hi || f != float64(int64(f)) {
			if lo == -inf {
				return "must be a whole number"
			}
			return fmt.Sprintf("must be a whole number of at least %g", lo)
		}
		return ""
	}
}

func weight(v cty.Value) string {
	if f, ok := float(v); !ok || f < 100 || f > 900 || int(f)%100 != 0 || f != float64(int(f)) {
		return "must be 100, 200, … or 900"
	}
	return ""
}

func dash(v cty.Value) string {
	if !known(v) || !(v.Type().IsTupleType() || v.Type().IsListType()) {
		return "must be a list of numbers of at least 0"
	}
	for _, e := range v.AsValueSlice() {
		if f, ok := float(e); !ok || f < 0 {
			return "must be a list of numbers of at least 0"
		}
	}
	return ""
}

func numbers(n int, lo, hi float64) func(cty.Value) string {
	return func(v cty.Value) string {
		problem := fmt.Sprintf("must be a list of %d numbers from %g to %g", n, lo, hi)
		if !known(v) || !(v.Type().IsTupleType() || v.Type().IsListType()) || v.LengthInt() != n {
			return problem
		}
		for _, e := range v.AsValueSlice() {
			if f, ok := float(e); !ok || f < lo || f > hi {
				return problem
			}
		}
		return ""
	}
}

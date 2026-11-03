package style

import (
	"encoding/xml"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/spec"
)

// Prop is a style property and its value check.
type Prop struct {
	check func(cty.Value) string
}

// BuiltinShapes are the shapes a style may name without a shape block (spec 4.3).
var BuiltinShapes = []string{"rect", "rounded", "stadium", "ellipse", "diamond", "cylinder", "hexagon", "parallelogram", "document", "person", "cloud", "text", "image", "point", "bullseye", "table"}

var markers = []string{"none", "arrow", "triangle", "triangle-open", "diamond", "diamond-open", "circle", "circle-open", "crow-one", "crow-zero-one", "crow-many", "crow-one-many", "crow-zero-many"}

// NodeProps are the properties of style blocks: node properties and container properties.
var NodeProps = map[string]Prop{
	"shape":         {isString},
	"fill":          {color},
	"stroke":        {color},
	"stroke_width":  {number(0, inf)},
	"stroke_dash":   {dash},
	"corner_radius": {number(0, inf)},
	"opacity":       {number(0, 1)},
	"text_color":    {color},
	"font_size":     {number(1, inf)},
	"font_weight":   {weight},
	"icon":          {picture},
	"image":         {picture},
	"padding":       {number(0, inf)},
	"min_w":         {number(0, inf)},
	"min_h":         {number(0, inf)},
	"max_w":         {number(0, inf)},
	"z":             {integer(-inf, inf)},
	"layout":        {enum("free", "stack", "grid")},
	"direction":     {enum("horizontal", "vertical")},
	"gap":           {number(0, inf)},
	"stretch":       {isBool},
	"columns":       {integer(1, inf)},
	"align":         {enum("start", "center", "end")},
	"label_side":    {enum("top", "left")},
}

// EdgeProps are the properties of edge_style blocks.
var EdgeProps = map[string]Prop{
	"stroke":       {color},
	"stroke_width": {number(0, inf)},
	"stroke_dash":  {dash},
	"arrow_start":  {enum(markers...)},
	"arrow_end":    {enum(markers...)},
	"routing":      {enum("orthogonal", "polyline", "straight")},
	"font_size":    {number(1, inf)},
	"animate":      {enum("none", "flow")},
}

// OverlayProps are the properties of overlay_style blocks.
var OverlayProps = map[string]Prop{
	"fill":         {color},
	"stroke":       {color},
	"stroke_width": {number(0, inf)},
	"stroke_dash":  {dash},
	"opacity":      {number(0, 1)},
	"padding":      {number(0, inf)},
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
	dec := xml.NewDecoder(strings.NewReader(v.AsString()))
	var root xml.Name
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "is not well-formed SVG: " + err.Error()
		}
		if el, ok := tok.(xml.StartElement); ok && root.Local == "" {
			root = el.Name
		}
	}
	if root != (xml.Name{Space: "http://www.w3.org/2000/svg", Local: "svg"}) {
		return `must be an SVG picture, whose root element is svg with xmlns="http://www.w3.org/2000/svg"`
	}
	return ""
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

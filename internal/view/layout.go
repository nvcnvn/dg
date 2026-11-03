package view

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"go.yaml.in/yaml/v3"

	"github.com/nvcnvn/dg/internal/spec"
)

// Layout is a layout file (spec 4.6) with its IDs decoded.
type Layout struct {
	Place       map[string]*Place
	EdgeHints   map[string]*Hint
	LiftHints   map[[2]string]*Hint
	Decorations []*Decoration
	Range       hcl.Range
}

// Place is a pinned node.
type Place struct {
	In    string
	X     *int
	Y     *int
	W     *int
	H     *int
	Order []string
	Ports map[string]*PortPin
	Range hcl.Range
}

// PortPin is a pinned port position.
type PortPin struct {
	Side   string
	Offset *float64
	Range  hcl.Range
}

// Hint is an edge hint.
type Hint struct {
	FromSide string
	ToSide   string
	Via      [][2]int
	LabelPos *float64
	Range    hcl.Range
}

// Decoration is a decoration of a view (spec 5.7).
type Decoration struct {
	ID          string
	Type        string
	In          string
	Attach      string
	X           *int
	Y           *int
	DX          *int
	DY          *int
	W           *int
	H           *int
	Z           *int
	Text        string
	Src         string
	Points      [][2]int
	Path        string
	Fill        string
	Stroke      string
	StrokeWidth *float64
	StrokeDash  []float64
	Opacity     *float64
	TextColor   string
	FontSize    *float64
	Range       hcl.Range
}

// DecorationTypes are the types of decorations.
var DecorationTypes = []string{"text", "rect", "ellipse", "band", "image", "arrow", "freehand"}

var pinSides = []string{"top", "right", "bottom", "left"}

// EncodeID percent-encodes the characters of id that are special in layout files (spec 4.6).
func EncodeID(id string) string {
	return strings.NewReplacer("%", "%25", ":", "%3A", "#", "%23").Replace(id)
}

var escape = regexp.MustCompile(`%.{0,2}`)

// DecodeID reverses EncodeID; a raw ':' or '#' or another escape is an error.
func DecodeID(s string) (string, error) {
	if strings.ContainsAny(s, ":#") {
		return "", fmt.Errorf("%q contains a raw ':' or '#'; write %%3A or %%23", s)
	}
	var bad string
	out := escape.ReplaceAllStringFunc(s, func(e string) string {
		switch strings.ToUpper(e) {
		case "%25":
			return "%"
		case "%3A":
			return ":"
		case "%23":
			return "#"
		}
		bad = e
		return e
	})
	if bad != "" {
		return "", fmt.Errorf("%q contains %q; only %%25, %%3A and %%23 are escapes", s, bad)
	}
	return out, nil
}

type decoder struct {
	file  string
	diags hcl.Diagnostics
}

func (d *decoder) rng(n *yaml.Node) hcl.Range {
	pos := hcl.Pos{Line: n.Line, Column: n.Column}
	return hcl.Range{Filename: d.file, Start: pos, End: pos}
}

func (d *decoder) errorf(n *yaml.Node, format string, args ...any) {
	d.diags = append(d.diags, spec.Errorf(d.rng(n), "Invalid layout", format, args...))
}

var (
	yamlLine      = regexp.MustCompile(`^yaml: line (\d+): `)
	typeErrorLine = regexp.MustCompile(`^line \d+: `)
)

// DecodeLayout decodes a layout file; file names it in diagnostics.
func DecodeLayout(src []byte, file string) (*Layout, hcl.Diagnostics) {
	d := &decoder{file: file}
	l := &Layout{Place: map[string]*Place{}, EdgeHints: map[string]*Hint{}, LiftHints: map[[2]string]*Hint{}}
	l.Range = hcl.Range{Filename: file, Start: hcl.InitialPos, End: hcl.InitialPos}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	var doc, extra yaml.Node
	err := dec.Decode(&doc)
	if errors.Is(err, io.EOF) {
		d.errorf(&yaml.Node{Line: 1, Column: 1}, "Missing version: 1.")
		return l, d.diags
	}
	if err == nil {
		if err = dec.Decode(&extra); err == nil {
			d.errorf(&extra, "A layout file holds one YAML document.")
			return l, d.diags
		}
		if errors.Is(err, io.EOF) {
			err = nil
		}
	}
	if err != nil {
		d.syntaxError(err)
		return l, d.diags
	}
	if len(doc.Content) == 0 {
		d.errorf(&doc, "Missing version: 1.")
		return l, d.diags
	}
	root := doc.Content[0]
	if !d.mapping(root) {
		return l, d.diags
	}
	version := false
	d.fields(root, []string{"version", "place", "edge_hints", "decorations"}, func(key string, k, v *yaml.Node) {
		switch key {
		case "version":
			version = true
			if n := d.intp(v); n != nil && *n != 1 {
				d.errorf(v, "Version %d is not supported; the version is 1.", *n)
			}
		case "place":
			d.entries(v, func(id string, lift *[2]string, k, v *yaml.Node) {
				if lift != nil {
					d.errorf(k, "A lift: key is only valid in edge_hints.")
				} else if p := d.place(v); p != nil {
					p.Range = d.rng(k)
					l.Place[id] = p
				}
			})
		case "edge_hints":
			d.entries(v, func(id string, lift *[2]string, k, v *yaml.Node) {
				h := d.hint(v)
				switch {
				case h == nil:
				case lift != nil:
					h.Range = d.rng(k)
					l.LiftHints[*lift] = h
				default:
					h.Range = d.rng(k)
					l.EdgeHints[id] = h
				}
			})
		case "decorations":
			if v.Kind != yaml.SequenceNode {
				d.errorf(v, "Expected a list of decorations.")
				return
			}
			seen := map[string]bool{}
			for _, item := range v.Content {
				dec := d.decoration(item)
				if dec == nil {
					continue
				}
				if seen[dec.ID] {
					d.diags = append(d.diags, spec.Errorf(dec.Range, "Duplicate ID", "The decoration ID %q is used twice in this view.", dec.ID))
					continue
				}
				seen[dec.ID] = true
				l.Decorations = append(l.Decorations, dec)
			}
		}
	})
	if !version {
		d.errorf(root, "Missing version: 1.")
	}
	return l, d.diags
}

// syntaxError reports a YAML syntax error. The yaml package gives its line but not its column.
func (d *decoder) syntaxError(err error) {
	line := 1
	msg := err.Error()
	if m := yamlLine.FindStringSubmatch(msg); m != nil {
		line, _ = strconv.Atoi(m[1])
		msg = strings.TrimPrefix(msg, m[0])
	}
	d.errorf(&yaml.Node{Line: line, Column: 1}, "%s.", strings.TrimPrefix(msg, "yaml: "))
}

func (d *decoder) mapping(n *yaml.Node) bool {
	if n.Kind != yaml.MappingNode {
		d.errorf(n, "Expected a mapping.")
		return false
	}
	return true
}

// fields calls f for each key of mapping n, reporting keys not in allowed and repeated keys.
func (d *decoder) fields(n *yaml.Node, allowed []string, f func(key string, k, v *yaml.Node)) {
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		switch {
		case !slices.Contains(allowed, k.Value):
			d.errorf(k, "Unknown field %q; expected one of %s.", k.Value, strings.Join(allowed, ", "))
		case seen[k.Value]:
			d.errorf(k, "Field %q is repeated.", k.Value)
		default:
			seen[k.Value] = true
			f(k.Value, k, v)
		}
	}
}

// entries calls f for each entry of mapping n with its decoded ID key, or for a key of the form
// lift:<from>:<to> with its decoded ends, reporting invalid and repeated keys.
func (d *decoder) entries(n *yaml.Node, f func(id string, lift *[2]string, k, v *yaml.Node)) {
	if !d.mapping(n) {
		return
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		var id string
		var lift *[2]string
		ok := true
		if rest, isLift := strings.CutPrefix(k.Value, "lift:"); isLift {
			parts := strings.Split(rest, ":")
			if len(parts) != 2 {
				d.errorf(k, "A lifted edge key is lift:<from>:<to>.")
				continue
			}
			from, ok1 := d.id(k, parts[0])
			to, ok2 := d.id(k, parts[1])
			ok = ok1 && ok2
			lift = &[2]string{from, to}
			id = "lift:" + EncodeID(from) + ":" + EncodeID(to)
		} else {
			id, ok = d.id(k, k.Value)
		}
		switch {
		case !ok:
		case seen[id]:
			d.errorf(k, "The key %q is repeated.", k.Value)
		default:
			seen[id] = true
			f(id, lift, k, v)
		}
	}
}

func (d *decoder) id(n *yaml.Node, s string) (string, bool) {
	id, err := DecodeID(s)
	if err == nil && s == "" {
		err = fmt.Errorf("an ID is empty")
	}
	if err != nil {
		d.errorf(n, "Invalid ID: %v.", err)
		return "", false
	}
	return id, true
}

func (d *decoder) decode(n *yaml.Node, v any) bool {
	if err := n.Decode(v); err != nil {
		msg := err.Error()
		if te, ok := err.(*yaml.TypeError); ok {
			msg = typeErrorLine.ReplaceAllString(te.Errors[0], "")
		}
		d.errorf(n, "%s.", strings.TrimPrefix(msg, "yaml: "))
		return false
	}
	return true
}

func (d *decoder) ids(n *yaml.Node) []string {
	var raw []string
	if !d.decode(n, &raw) {
		return nil
	}
	var out []string
	for _, s := range raw {
		if id, ok := d.id(n, s); ok {
			out = append(out, id)
		}
	}
	return out
}

func (d *decoder) enum(n *yaml.Node, values []string) string {
	var s string
	if d.decode(n, &s) && !slices.Contains(values, s) {
		d.errorf(n, "%q is not one of %s.", s, strings.Join(values, ", "))
	}
	return s
}

func (d *decoder) fraction(n *yaml.Node) *float64 {
	var f float64
	if !d.decode(n, &f) {
		return nil
	}
	if !(f >= 0 && f <= 1) {
		d.errorf(n, "%g is not between 0 and 1.", f)
	}
	return &f
}

func (d *decoder) points(n *yaml.Node) [][2]int {
	if n.Kind != yaml.SequenceNode {
		d.errorf(n, "Expected a list of [x, y] points.")
		return nil
	}
	var out [][2]int
	for _, p := range n.Content {
		if p.Kind != yaml.SequenceNode || len(p.Content) != 2 {
			d.errorf(p, "A point is [x, y].")
			return nil
		}
		x, y := d.intp(p.Content[0]), d.intp(p.Content[1])
		if x == nil || y == nil {
			return nil
		}
		out = append(out, [2]int{*x, *y})
	}
	return out
}

func (d *decoder) size(n *yaml.Node) *int {
	i := d.intp(n)
	if i != nil && *i < 0 {
		d.errorf(n, "A size is at least 0.")
		return nil
	}
	return i
}

// intp decodes a whole number; yaml.v3 would otherwise truncate 1.5 to 1.
func (d *decoder) intp(n *yaml.Node) *int {
	var i int
	if n.ShortTag() != "!!int" {
		d.errorf(n, "%q is not a whole number.", n.Value)
		return nil
	}
	if !d.decode(n, &i) {
		return nil
	}
	return &i
}

func (d *decoder) floatp(n *yaml.Node) *float64 {
	var f float64
	if !d.decode(n, &f) {
		return nil
	}
	if math.IsInf(f, 0) || math.IsNaN(f) {
		d.errorf(n, "%q is not a finite number.", n.Value)
		return nil
	}
	return &f
}

func (d *decoder) str(n *yaml.Node) string {
	var s string
	d.decode(n, &s)
	return s
}

func (d *decoder) place(n *yaml.Node) *Place {
	if !d.mapping(n) {
		return nil
	}
	p := &Place{}
	d.fields(n, []string{"in", "x", "y", "w", "h", "order", "ports"}, func(key string, k, v *yaml.Node) {
		switch key {
		case "in":
			p.In, _ = d.id(v, d.str(v))
		case "x":
			p.X = d.intp(v)
		case "y":
			p.Y = d.intp(v)
		case "w":
			p.W = d.size(v)
		case "h":
			p.H = d.size(v)
		case "order":
			p.Order = d.ids(v)
		case "ports":
			if !d.mapping(v) {
				return
			}
			p.Ports = map[string]*PortPin{}
			for i := 0; i+1 < len(v.Content); i += 2 {
				pk, pv := v.Content[i], v.Content[i+1]
				name, ok := d.id(pk, pk.Value)
				switch {
				case !ok:
					continue
				case p.Ports[name] != nil:
					d.errorf(pk, "The port %q is repeated.", pk.Value)
					continue
				case !d.mapping(pv):
					continue
				}
				pin := &PortPin{Range: d.rng(pk)}
				d.fields(pv, []string{"side", "offset"}, func(key string, _, v *yaml.Node) {
					if key == "side" {
						pin.Side = d.enum(v, pinSides)
					} else {
						pin.Offset = d.fraction(v)
					}
				})
				p.Ports[name] = pin
			}
		}
	})
	return p
}

func (d *decoder) hint(n *yaml.Node) *Hint {
	if !d.mapping(n) {
		return nil
	}
	h := &Hint{}
	d.fields(n, []string{"from_side", "to_side", "via", "label_pos"}, func(key string, _, v *yaml.Node) {
		switch key {
		case "from_side":
			h.FromSide = d.enum(v, pinSides)
		case "to_side":
			h.ToSide = d.enum(v, pinSides)
		case "via":
			h.Via = d.points(v)
		case "label_pos":
			h.LabelPos = d.fraction(v)
		}
	})
	return h
}

var decorationFields = []string{"id", "type", "in", "attach", "x", "y", "dx", "dy", "w", "h", "z", "text", "src", "points", "path", "fill", "stroke", "stroke_width", "stroke_dash", "opacity", "text_color", "font_size"}

func (d *decoder) decoration(n *yaml.Node) *Decoration {
	if !d.mapping(n) {
		return nil
	}
	dec := &Decoration{Range: d.rng(n)}
	d.fields(n, decorationFields, func(key string, _, v *yaml.Node) {
		switch key {
		case "id":
			dec.ID = d.str(v)
		case "type":
			dec.Type = d.enum(v, DecorationTypes)
		case "in":
			dec.In, _ = d.id(v, d.str(v))
		case "attach":
			dec.Attach, _ = d.id(v, d.str(v))
		case "x":
			dec.X = d.intp(v)
		case "y":
			dec.Y = d.intp(v)
		case "dx":
			dec.DX = d.intp(v)
		case "dy":
			dec.DY = d.intp(v)
		case "w":
			dec.W = d.size(v)
		case "h":
			dec.H = d.size(v)
		case "z":
			dec.Z = d.intp(v)
		case "text":
			dec.Text = d.str(v)
		case "src":
			dec.Src = d.str(v)
		case "points":
			dec.Points = d.points(v)
		case "path":
			dec.Path = d.str(v)
		case "fill":
			dec.Fill = d.str(v)
		case "stroke":
			dec.Stroke = d.str(v)
		case "stroke_width":
			dec.StrokeWidth = d.floatp(v)
		case "stroke_dash":
			if v.Kind != yaml.SequenceNode {
				d.errorf(v, "Expected a list of numbers.")
				return
			}
			for _, e := range v.Content {
				if f := d.floatp(e); f != nil {
					dec.StrokeDash = append(dec.StrokeDash, *f)
				}
			}
		case "opacity":
			dec.Opacity = d.fraction(v)
		case "text_color":
			dec.TextColor = d.str(v)
		case "font_size":
			dec.FontSize = d.floatp(v)
		}
	})
	if !spec.IsSimpleID(dec.ID) {
		d.errorf(n, "A decoration needs an id that is a simple ID.")
		return nil
	}
	if dec.Type == "" {
		d.errorf(n, "Decoration %q needs a type.", dec.ID)
		return nil
	}
	return dec
}

// Encode returns l in canonical form: sorted keys, one entry per line where possible.
func (l *Layout) Encode() []byte {
	root := &yaml.Node{Kind: yaml.MappingNode}
	add := func(m *yaml.Node, key string, v *yaml.Node) {
		m.Content = append(m.Content, str(key), v)
	}
	add(root, "version", scalar("", "1"))
	if len(l.Place) > 0 {
		add(root, "place", sorted(l.Place, EncodeID, placeNode))
	}
	hints := map[string]*Hint{}
	for id, h := range l.EdgeHints {
		hints[EncodeID(id)] = h
	}
	for k, h := range l.LiftHints {
		hints["lift:"+EncodeID(k[0])+":"+EncodeID(k[1])] = h
	}
	if len(hints) > 0 {
		add(root, "edge_hints", sorted(hints, func(s string) string { return s }, hintNode))
	}
	if len(l.Decorations) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, dec := range l.Decorations {
			seq.Content = append(seq.Content, decorationNode(dec))
		}
		add(root, "decorations", seq)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		panic(fmt.Sprintf("encoding a layout built in memory: %v", err))
	}
	return buf.Bytes()
}

func sorted[T any](m map[string]T, key func(string) string, node func(T) *yaml.Node) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode}
	keys := slices.SortedFunc(maps.Keys(m), func(a, b string) int { return cmp.Compare(key(a), key(b)) })
	for _, k := range keys {
		out.Content = append(out.Content, str(key(k)), node(m[k]))
	}
	return out
}

type fieldList struct{ n *yaml.Node }

func flow() fieldList { return fieldList{&yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}} }

func (f fieldList) add(key string, v *yaml.Node) {
	f.n.Content = append(f.n.Content, str(key), v)
}

func (f fieldList) str(key, v string) {
	if v != "" {
		f.add(key, str(v))
	}
}

func (f fieldList) int(key string, v *int) {
	if v != nil {
		f.add(key, scalar("", strconv.Itoa(*v)))
	}
}

func (f fieldList) float(key string, v *float64, prec int) {
	if v != nil {
		f.add(key, scalar("", strconv.FormatFloat(*v, 'f', prec, 64)))
	}
}

func (f fieldList) points(key string, pts [][2]int) {
	if len(pts) == 0 {
		return
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, p := range pts {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Content: []*yaml.Node{
			scalar("", strconv.Itoa(p[0])), scalar("", strconv.Itoa(p[1])),
		}})
	}
	f.add(key, seq)
}

func placeNode(p *Place) *yaml.Node {
	f := flow()
	f.str("in", EncodeID(p.In))
	f.int("x", p.X)
	f.int("y", p.Y)
	f.int("w", p.W)
	f.int("h", p.H)
	if len(p.Order) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, id := range p.Order {
			seq.Content = append(seq.Content, str(EncodeID(id)))
		}
		f.add("order", seq)
	}
	if len(p.Ports) > 0 {
		f.n.Style = 0
		f.add("ports", sorted(p.Ports, EncodeID, func(pin *PortPin) *yaml.Node {
			pf := flow()
			pf.str("side", pin.Side)
			pf.float("offset", pin.Offset, 2)
			return pf.n
		}))
	}
	return f.n
}

func hintNode(h *Hint) *yaml.Node {
	f := flow()
	f.str("from_side", h.FromSide)
	f.str("to_side", h.ToSide)
	f.points("via", h.Via)
	f.float("label_pos", h.LabelPos, 2)
	return f.n
}

func decorationNode(d *Decoration) *yaml.Node {
	f := flow()
	f.str("id", d.ID)
	f.str("type", d.Type)
	f.str("in", EncodeID(d.In))
	f.str("attach", EncodeID(d.Attach))
	f.int("x", d.X)
	f.int("y", d.Y)
	f.int("dx", d.DX)
	f.int("dy", d.DY)
	f.int("w", d.W)
	f.int("h", d.H)
	f.int("z", d.Z)
	f.str("text", d.Text)
	f.str("src", d.Src)
	f.points("points", d.Points)
	f.str("path", d.Path)
	f.str("fill", d.Fill)
	f.str("stroke", d.Stroke)
	f.float("stroke_width", d.StrokeWidth, -1)
	if len(d.StrokeDash) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, v := range d.StrokeDash {
			seq.Content = append(seq.Content, scalar("", strconv.FormatFloat(v, 'f', -1, 64)))
		}
		f.add("stroke_dash", seq)
	}
	f.float("opacity", d.Opacity, -1)
	f.str("text_color", d.TextColor)
	f.float("font_size", d.FontSize, -1)
	return f.n
}

func str(s string) *yaml.Node { return scalar("!!str", s) }

func scalar(tag, v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: v}
}

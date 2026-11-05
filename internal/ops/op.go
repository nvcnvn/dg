package ops

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/view"
)

// Op is one operation (spec FR-12) as dg op, the MCP op tool and dg serve send it: the field
// op names it, and the other fields are its arguments (Defs says which each takes). In set_rows,
// one compartment without a name holds rows declared directly in the node.
type Op struct {
	Op           string             `json:"op"`
	ID           string             `json:"id,omitempty"`
	Type         string             `json:"type,omitempty"`
	Kind         string             `json:"kind,omitempty"`
	Label        *string            `json:"label,omitempty"`
	Parent       string             `json:"parent,omitempty"`
	Before       string             `json:"before,omitempty"`
	View         string             `json:"view,omitempty"`
	X            *int               `json:"x,omitempty"`
	Y            *int               `json:"y,omitempty"`
	W            *int               `json:"w,omitempty"`
	H            *int               `json:"h,omitempty"`
	Recursive    bool               `json:"recursive,omitempty"`
	Tags         []string           `json:"tags,omitzero"`
	Props        map[string]any     `json:"props,omitzero"`
	Name         string             `json:"name,omitempty"`
	Side         string             `json:"side,omitempty"`
	Offset       *float64           `json:"offset,omitempty"`
	Port         string             `json:"port,omitempty"`
	Compartments []view.Compartment `json:"compartments,omitzero"`
	From         *view.End          `json:"from,omitempty"`
	To           *view.End          `json:"to,omitempty"`
	Labels       *model.Labels      `json:"labels,omitempty"`
	Hint         *view.EdgeHint     `json:"hint,omitempty"`
	Old          string             `json:"old,omitempty"`
	New          string             `json:"new,omitempty"`
	Title        string             `json:"title,omitempty"`
	Include      []string           `json:"include,omitzero"`
	Exclude      []string           `json:"exclude,omitzero"`
	Collapse     []string           `json:"collapse,omitzero"`
	Edges        []string           `json:"edges,omitzero"`
	Of           string             `json:"of,omitempty"`
	Direction    string             `json:"direction,omitempty"`
	Overlays     []OverlayArgs      `json:"overlays,omitzero"`
	Styles       []StyleArgs        `json:"styles,omitzero"`
	Decoration   *DecorationArgs    `json:"decoration,omitempty"`
	Unset        []string           `json:"unset,omitzero"`
	Container    string             `json:"container,omitempty"`
	Order        []string           `json:"order,omitzero"`
	Ops          []Op               `json:"ops,omitzero"`
}

// Def describes an operation: what it does and the fields it takes.
type Def struct {
	Name    string
	Summary string
	Fields  []Field
}

// Field describes a field of an operation: its JSON schema, what it means, an example value as
// JSON and, for a field that nests, the Shape of its value.
type Field struct {
	Name     string
	Schema   string
	Help     string
	Example  string
	Shape    string
	Required bool
}

func (f Field) req() Field { f.Required = true; return f }

func (f Field) help(h string) Field { f.Help = h; return f }

func (f Field) shape(s string) Field { f.Shape = s; return f }

const (
	strSchema        = `{"type":"string"}`
	intSchema        = `{"type":"integer"}`
	endSchema        = `{"type":"object","properties":{"node":{"type":"string"},"port":{"type":"string"},"row":{"type":"string"}},"required":["node"],"additionalProperties":false}`
	stringList       = `{"type":"array","items":{"type":"string"}}`
	sideSchema       = `{"type":"string","enum":["top","right","bottom","left"]}`
	labelsSchema     = `{"type":"object","properties":{"start":{"type":"string"},"mid":{"type":"string"},"end":{"type":"string"}},"additionalProperties":false}`
	rowsSchema       = `{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"rows":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"type":{"type":"string"},"key":{"type":"string"}},"required":["name"],"additionalProperties":false}}},"required":["rows"],"additionalProperties":false}}`
	overlaysSchema   = `{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"members":{"type":"array","items":{"type":"string"}},"style":{"type":"string"},"label":{"type":"string"}},"required":["id","members"],"additionalProperties":false}}`
	stylesSchema     = `{"type":"array","items":{"type":"object","properties":{"type":{"type":"string","enum":["style","edge_style"]},"label":{"type":"string"},"props":{"type":"object","additionalProperties":{"anyOf":[{"type":["string","number","boolean"]},{"type":"array","items":{"type":"number"}}]}}},"required":["label","props"],"additionalProperties":false}}`
	decorationSchema = `{"type":"object","properties":{"type":{"type":"string","enum":["text","rect","ellipse","band","image","arrow","freehand"]},"in":{"type":"string"},"attach":{"type":"string"},"x":{"type":"integer"},"y":{"type":"integer"},"dx":{"type":"integer"},"dy":{"type":"integer"},"w":{"type":"integer","minimum":1},"h":{"type":"integer","minimum":1},"z":{"type":"integer"},"text":{"type":"string"},"src":{"type":"string"},"points":{"type":"array","items":{"type":"array","items":{"type":"integer"},"minItems":2,"maxItems":2}},"path":{"type":"string"},"fill":{"type":"string"},"stroke":{"type":"string"},"stroke_width":{"type":"number","minimum":0},"stroke_dash":{"type":"array","items":{"type":"number","minimum":0}},"opacity":{"type":"number","minimum":0,"maximum":1},"text_color":{"type":"string"},"font_size":{"type":"number","minimum":1}},"additionalProperties":false}`
	hintSchema       = `{"type":"object","properties":{"from_side":` + sideSchema + `,"to_side":` + sideSchema + `,"via":{"type":"array","items":{"type":"array","items":{"type":"integer"},"minItems":2,"maxItems":2}},"label_pos":{"type":"number","minimum":0,"maximum":1}},"additionalProperties":false}`

	endShape        = `{node, port?, row?}`
	labelsShape     = `{start?, mid?, end?}`
	rowsShape       = `[{name?, rows: [{name, type?, key?}]}]`
	overlaysShape   = `[{id, members, style?, label?}]`
	stylesShape     = `[{type?, label, props}]`
	decorationShape = `in?, attach?, x?, y?, dx?, dy?, w?, h?, z?, text?, src?, points?, path?, fill?, stroke?, stroke_width?, stroke_dash?, opacity?, text_color?, font_size?}`
	hintShape       = `{from_side?, to_side?, via?: [[x, y]], label_pos?}`
)

var (
	fNode     = Field{Name: "id", Schema: strSchema, Help: "ID of the node", Example: `"api"`}
	fEdge     = Field{Name: "id", Schema: strSchema, Help: "ID of the edge", Example: `"api-db"`}
	fElement  = Field{Name: "id", Schema: strSchema, Help: "ID of the node, or of the edge with type edge", Example: `"api"`}
	fType     = Field{Name: "type", Schema: `{"type":"string","enum":["node","edge"]}`, Help: "what id names: node (the default) or edge", Example: `"edge"`}
	fKind     = Field{Name: "kind", Schema: strSchema, Help: "kind of the node (dg vocabulary lists them)", Example: `"service"`}
	fLabel    = Field{Name: "label", Schema: strSchema, Help: "label of the node; a newline character breaks a line (in a shell, $'a\\nb')", Example: `"API"`}
	fParent   = Field{Name: "parent", Schema: strSchema, Help: "ID of the container node", Example: `"backend"`}
	fView     = Field{Name: "view", Schema: strSchema, Help: "view ID", Example: `"main"`}
	fX        = Field{Name: "x", Schema: intSchema, Help: "x relative to the node's frame (its nearest container drawn in the view, else the canvas), rounded to the project grid", Example: `100`}
	fY        = Field{Name: "y", Schema: intSchema, Help: "y relative to the node's frame, rounded to the project grid", Example: `40`}
	fTags     = Field{Name: "tags", Schema: stringList, Help: "tags (simple IDs); an empty list removes them", Example: `["critical"]`}
	fPortName = Field{Name: "name", Schema: strSchema, Help: "port name", Example: `"grpc"`}
	fLabels   = Field{Name: "labels", Schema: labelsSchema, Help: "edge labels {start?, mid?, end?}", Example: `{"mid": "SQL"}`, Shape: labelsShape}
	fSels     = func(name, help string) Field {
		return Field{Name: name, Schema: stringList, Help: help, Example: `["node.api"]`}
	}
	fViewID       = Field{Name: "id", Schema: strSchema, Help: "view ID", Example: `"overview"`}
	fTitle        = Field{Name: "title", Schema: strSchema, Help: "view title", Example: `"Overview"`}
	fInclude      = fSels("include", `selectors of the nodes the view shows, as in views.hcl: all, node.<id> or node["<id>"], kind.<kind>, tag.<tag>, children.<id>, descendants.<id> (default [all])`)
	fExclude      = fSels("exclude", "selectors of nodes the view leaves out")
	fCollapse     = fSels("collapse", "selectors of containers the view draws collapsed")
	fEdgeSels     = fSels("edges", "selectors of the edges the view shows: edge_kind.<kind>, tag.<tag>, edge.<id> (default: every edge whose ends are shown)")
	fOf           = Field{Name: "of", Schema: strSchema, Help: "ID of the container whose inside the view shows (drill-down)", Example: `"backend"`}
	fDirection    = Field{Name: "direction", Schema: `{"type":"string","enum":["down","right","up","left"]}`, Help: "the way the layers of the view's first layout run: down, right, up or left (default: down, or right when an edge ends at a row)", Example: `"right"`}
	fOverlays     = Field{Name: "overlays", Schema: overlaysSchema, Help: "overlays [{id, members: [selectors], style?, label?}]", Example: `[{"id": "blast", "members": ["tag.critical"], "label": "Blast radius"}]`, Shape: overlaysShape}
	fStyles       = Field{Name: "styles", Schema: stylesSchema, Help: "view style rules [{type?: style (default) or edge_style, label: a kind, tag.<tag>, node.<id> or edge.<id>, props: {property: value}}]", Example: `[{"label": "tag.new", "props": {"fill": "#E6F4EA"}}]`, Shape: stylesShape}
	fUnset        = Field{Name: "unset", Schema: stringList, Example: `["of"]`}
	fDecorationID = Field{Name: "id", Schema: strSchema, Help: "ID of the decoration", Example: `"note"`}
	fDecoration   = Field{Name: "decoration", Schema: decorationSchema, Help: "decoration fields as in layout files: type (text, rect, ellipse, band, image, arrow, freehand), in or attach, x and y (in the frame) or dx and dy (from the attached node's center), w, h, z, text, src, points, path, fill, stroke, stroke_width, stroke_dash, opacity, text_color, font_size", Example: `{"type": "text", "x": 0, "y": -60, "text": "Title"}`, Shape: "{type?, " + decorationShape}
)

// Defs are the operations, in the order help lists them.
var Defs = []Def{
	{"create_node", "Append a node block. With view, also make the view show it (adding node.<id> to its include if needed) and, with x and y, pin it there; where a stack or grid container places it in the view, it goes in the container's order, before before or last.", []Field{
		fKind.req(), fNode.help("ID of the new node (generated from the label, or the kind, when omitted)"), fLabel, fParent, fView.help("view to show the node in"), fX, fY,
		{Name: "before", Schema: strSchema, Help: "with view, where a stack or grid container places the new node: the sibling it goes before in the container's order (default: last)", Example: `"review"`}}},
	{"delete_node", "Remove a node with its edges, layout entries, decorations attached to it or placed in it, view selectors and node.<id> view rules. Refused while a view has of on it.", []Field{
		fNode.req(), {Name: "recursive", Schema: `{"type":"boolean"}`, Help: "also delete the node's descendants; otherwise its children move to its parent, unpinned", Example: `true`}}},
	{"set_label", "Change a node's label. A node created in the same batch without a label or ID takes a new ID from this first label (spec 6.3).", []Field{fNode.req(), fLabel.req()}},
	{"set_view_label", "Set the label a view draws a node with instead of the model's; without label, remove it, so the view draws the model's again.", []Field{fView.req(), fNode.req(), fLabel.help("label of the node in this view; a newline character breaks a line (in a shell, $'a\\nb')")}},
	{"set_parent", "Set or remove a node's parent.", []Field{fNode.req(), fParent.help("ID of the container node; empty or omitted removes the parent")}},
	{"set_kind", "Change the kind of a node or edge.", []Field{fElement.req(), fKind.help("kind of the node, or edge kind of the edge").req(), fType}},
	{"set_tags", "Replace the tags of a node or edge.", []Field{fElement.req(), fTags.req(), fType}},
	{"set_props", "Replace a node's properties.", []Field{fNode.req(), {Name: "props", Schema: `{"type":"object","additionalProperties":{"type":["string","number","boolean"]}}`, Help: "property values by name, of the types the kind declares; an empty object removes them", Example: `{"replicas": 3}`, Required: true}}},
	{"add_port", "Add an instance port block to a node.", []Field{fNode.req(), fPortName.req(), {Name: "side", Schema: `{"type":"string","enum":["top","right","bottom","left","auto"]}`, Help: "side of the port: top, right, bottom, left or auto", Example: `"right"`}}},
	{"remove_port", "Remove an instance port block, and its pinned positions. Refused while an edge uses the port.", []Field{fNode.req(), fPortName.req()}},
	{"set_rows", "Replace a node's rows and compartments.", []Field{fNode.req(), {Name: "compartments", Schema: rowsSchema, Help: "compartments of rows ({name, rows: [{name, type?, key?}]}); one compartment without a name holds rows declared directly in the node; an empty list removes every row", Example: `[{"rows": [{"name": "id", "type": "uuid", "key": "pk"}]}]`, Shape: rowsShape, Required: true}}},
	{"connect", "Append an edge block. With view, make the view's edges filter show it (adding edge.<id>) if needed.", []Field{
		{Name: "from", Schema: endSchema, Help: "start of the edge: {node, port?} or {node, row?}", Example: `{"node": "api"}`, Shape: endShape, Required: true},
		{Name: "to", Schema: endSchema, Help: "end of the edge: {node, port?} or {node, row?}", Example: `{"node": "db"}`, Shape: endShape, Required: true},
		fEdge.help("ID of the new edge (<from>-<to> when omitted)"), fKind.help("edge kind"), fLabels, fTags, fView.help("view to show the edge in")}},
	{"disconnect", "Remove an edge with its hints and view references.", []Field{fEdge.req()}},
	{"set_edge_labels", "Replace an edge's labels; none removes them.", []Field{fEdge.req(), fLabels}},
	{"move", "Pin a node in a view at x, y relative to its frame. Only this node is pinned: dg op computes no geometry; dg export --format json gives every node's frame and rel, the coordinates move takes. Refused for a node a stack or grid container places (ordered_in in the export); reorder it instead.", []Field{fView.req(), fNode.req(), fX.req(), fY.req()}},
	{"resize", "Set the size of a node pinned in a view, or of a child of a stack or grid container.", []Field{fView.req(), fNode.req(),
		{Name: "w", Schema: `{"type":"integer","minimum":1}`, Help: "width, positive, rounded to the project grid (at least one unit)", Example: `160`, Required: true},
		{Name: "h", Schema: `{"type":"integer","minimum":1}`, Help: "height, positive, rounded to the project grid (at least one unit)", Example: `80`, Required: true}}},
	{"reorder", "Set the order in which a stack or grid container places its children in a view; children the order leaves out follow in sorted ID order.", []Field{fView.req(),
		{Name: "container", Schema: strSchema, Help: "ID of the stack or grid container, drawn expanded in the view or the view's of", Example: `"lanes"`, Required: true},
		{Name: "order", Schema: stringList, Help: "IDs of the container's children the view draws, in order; empty or omitted removes the order of any node, even one that no longer places its children in order", Example: `["intake", "review"]`}}},
	{"unpin", "Remove a node's position, size and pinned ports from a view; a container keeps its order.", []Field{fView.req(), fNode.req()}},
	{"pin_port", "Pin a port of a node pinned in a view to a side, at an offset along it.", []Field{fView.req(), fNode.req(),
		{Name: "port", Schema: strSchema, Help: "port name", Example: `"grpc"`, Required: true},
		{Name: "side", Schema: sideSchema, Help: "side: top, right, bottom or left", Example: `"right"`, Required: true},
		{Name: "offset", Schema: `{"type":"number","minimum":0,"maximum":1}`, Help: "position along the side, 0 to 1, kept to two decimals", Example: `0.5`}}},
	{"set_edge_hint", "Replace an edge's routing hint in a view.", []Field{fView.req(),
		fEdge.help("ID of the edge, or lift:<from>:<to> for a lifted edge (with %, : and # in those IDs written %25, %3A and %23)").req(),
		{Name: "hint", Schema: hintSchema, Help: "{from_side?, to_side?, via?: [[x, y], ...] in canvas coordinates, label_pos?: 0 to 1}; empty removes the hint", Example: `{"to_side": "top"}`, Shape: hintShape, Required: true}}},
	{"rename_id", "Rename a node, edge or view everywhere it is referenced: model files, views.hcl, layout files (and the layout file's name).", []Field{
		{Name: "type", Schema: `{"type":"string","enum":["node","edge","view"]}`, Help: "what old names: node, edge or view", Example: `"node"`, Required: true},
		{Name: "old", Schema: strSchema, Help: "current ID", Example: `"api"`, Required: true},
		{Name: "new", Schema: strSchema, Help: "new ID", Example: `"gateway"`, Required: true}}},
	{"create_view", "Append a view block to views.hcl.", []Field{
		fViewID.help("ID of the new view (a simple ID)").req(), fTitle, fInclude, fExclude, fCollapse, fEdgeSels, fOf, fDirection, fOverlays, fStyles}},
	{"update_view", "Change attributes and overlays of a view in views.hcl: the fields given replace the view's, unset removes them.", []Field{
		fViewID.req(), fTitle, fInclude, fExclude, fCollapse, fEdgeSels, fOf, fDirection,
		fOverlays.help(fOverlays.Help + "; replaces every overlay of the view, keeping unchanged ones as they are written"),
		fStyles.help(fStyles.Help + "; replaces every style rule of the view, keeping unchanged ones as they are written"),
		fUnset.help("attributes to remove: title, include, exclude, collapse, edges, of, direction, overlays, styles")}},
	{"delete_view", "Remove a view from views.hcl, and its layout file.", []Field{fViewID.req()}},
	{"show_node", "Make a view show an existing node: remove node.<id> from its exclude and, when its selectors still do not show the node, add node.<id> to its include. Refused when the view still does not show it (excluded by another selector, or inside a collapsed container).", []Field{fView.req(), fNode.req()}},
	{"hide_node", "Make a view stop showing a node: remove node.<id> from its include and, when its selectors still show the node, add node.<id> to its exclude.", []Field{fView.req(), fNode.req()}},
	{"add_decoration", "Add a decoration to a view's layout file (spec 5.7).", []Field{fView.req(),
		fDecorationID.help("ID of the new decoration (generated from its type when omitted)"),
		fDecoration.help(fDecoration.Help + "; type is required").shape("{type, " + decorationShape).req()}},
	{"update_decoration", "Change fields of a decoration of a view: the fields given replace its, unset removes them.", []Field{fView.req(), fDecorationID.req(), fDecoration,
		fUnset.help("decoration fields to remove, such as attach, dx and dy")}},
	{"delete_decoration", "Remove a decoration from a view.", []Field{fView.req(), fDecorationID.req()}},
	{"promote_decoration", "Replace a decoration of a view with a model node pinned where the decoration is, at its size; a text decoration's text becomes the label, and a decoration in a frame becomes a child of that container. Refused for attached decorations, arrows and freehand lines.", []Field{fView.req(), fDecorationID.req(), fKind.req(),
		{Name: "new", Schema: strSchema, Help: "ID of the new node (generated from the label, or the kind, when omitted)", Example: `"note"`},
		fLabel.help("label of the new node (default: the text of a text decoration)")}},
	{"batch", "Apply operations in order as one change: all files are written together, or none.", []Field{
		{Name: "ops", Schema: `{"type":"array","items":{"type":"object"}}`, Help: "operations, each an object with op and its fields", Example: `[{"op": "create_node", "kind": "service"}]`, Required: true}}},
}

// LookupDef returns the definition of operation name.
func LookupDef(name string) (Def, bool) {
	i := slices.IndexFunc(Defs, func(d Def) bool { return d.Name == name })
	if i < 0 {
		return Def{}, false
	}
	return Defs[i], true
}

// Field returns d's field name.
func (d Def) Field(name string) (Field, bool) {
	i := slices.IndexFunc(d.Fields, func(f Field) bool { return f.Name == name })
	if i < 0 {
		return Field{}, false
	}
	return d.Fields[i], true
}

// Signature lists the fields d takes, the optional ones marked with ?, each nested field with its
// shape.
func (d Def) Signature() string {
	var parts []string
	for _, f := range d.Fields {
		part := f.Name
		if !f.Required {
			part += "?"
		}
		if f.Shape != "" {
			part += ": " + f.Shape
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

// Example returns an operation object of d with its required fields, as JSON.
func (d Def) Example() string {
	parts := []string{fmt.Sprintf(`"op": %q`, d.Name)}
	for _, f := range d.Fields {
		if f.Required {
			parts = append(parts, fmt.Sprintf("%q: %s", f.Name, f.Example))
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// OpNames returns the names of the operations.
func OpNames() []string {
	names := make([]string, len(Defs))
	for i, d := range Defs {
		names[i] = d.Name
	}
	return names
}

// DecodeOp decodes one operation from JSON, checking that it names a known operation and has
// exactly the fields that operation takes; a batch's operations are checked too.
func DecodeOp(data []byte) (Op, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Op{}, fmt.Errorf("an operation is a JSON object: %w", err)
	}
	var name string
	if err := json.Unmarshal(raw["op"], &name); err != nil || raw["op"] == nil {
		return Op{}, fmt.Errorf("an operation needs \"op\", one of %s", strings.Join(OpNames(), ", "))
	}
	def, ok := LookupDef(name)
	if !ok {
		return Op{}, fmt.Errorf("unknown operation %q; operations are %s", name, strings.Join(OpNames(), ", "))
	}
	for _, k := range slices.Sorted(maps.Keys(raw)) {
		if _, ok := def.Field(k); k != "op" && !ok {
			return Op{}, fmt.Errorf("%s: unknown field %q; it takes %s", name, k, def.Signature())
		}
	}
	for _, f := range def.Fields {
		if v, ok := raw[f.Name]; f.Required && (!ok || string(v) == "null" || string(v) == `""` && f.Name != "label") {
			return Op{}, fmt.Errorf("%s: missing field %q; it takes %s", name, f.Name, def.Signature())
		}
	}
	ops := raw["ops"]
	delete(raw, "ops")
	rest, err := json.Marshal(raw)
	if err != nil {
		return Op{}, err
	}
	op, err := decodeFields(rest)
	if err != nil {
		return Op{}, decodeError(def, raw, err)
	}
	if ops != nil {
		var list []json.RawMessage
		if err := json.Unmarshal(ops, &list); err != nil {
			return Op{}, fmt.Errorf("batch: ops is a list of operations: %w", err)
		}
		op.Ops = make([]Op, len(list))
		for i, item := range list {
			if op.Ops[i], err = DecodeOp(item); err != nil {
				return Op{}, fmt.Errorf("batch operation %d: %w", i+1, err)
			}
		}
	}
	return op, nil
}

// DecodeOps decodes the input of dg op batch: one operation object, or a list of them, which
// is one batch.
func DecodeOps(data []byte) (Op, error) {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		data = append(append([]byte(`{"op":"batch","ops":`), data...), '}')
	}
	return DecodeOp(data)
}

// Schema returns the JSON schema of an operation object as the MCP op tool declares it: the
// union of every operation's fields, each described per operation. Clients such as the
// Anthropic API refuse oneOf at the top of a tool's input schema, so which fields each
// operation requires is in the tool's description and checked by DecodeOp.
func Schema() json.RawMessage {
	props := map[string]any{"op": map[string]any{"type": "string", "enum": OpNames(), "description": "the operation"}}
	var order []string
	schemas := map[string][]string{}
	helps := map[string][][2]string{}
	for _, d := range Defs {
		for _, f := range d.Fields {
			if _, ok := schemas[f.Name]; !ok {
				order = append(order, f.Name)
			}
			if !slices.Contains(schemas[f.Name], f.Schema) {
				schemas[f.Name] = append(schemas[f.Name], f.Schema)
			}
			helps[f.Name] = append(helps[f.Name], [2]string{d.Name, f.Help})
		}
	}
	for _, name := range order {
		var schema map[string]any
		if list := schemas[name]; len(list) == 1 {
			schema = mustJSON(list[0])
		} else {
			var alts []any
			for _, s := range list {
				alts = append(alts, mustJSON(s))
			}
			schema = map[string]any{"anyOf": alts}
		}
		var parts []string
		for _, h := range helps[name] {
			if i := slices.IndexFunc(parts, func(p string) bool { return strings.HasSuffix(p, ": "+h[1]) }); i >= 0 {
				ops, help, _ := strings.Cut(parts[i], ": ")
				parts[i] = ops + ", " + h[0] + ": " + help
				continue
			}
			parts = append(parts, h[0]+": "+h[1])
		}
		schema["description"] = strings.Join(parts, ". ")
		props[name] = schema
	}
	data, err := json.Marshal(map[string]any{"type": "object", "properties": props, "required": []string{"op"}})
	if err != nil {
		panic(err)
	}
	return data
}

func mustJSON(s string) map[string]any {
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(fmt.Sprintf("schema %s: %v", s, err))
	}
	return v
}

func decodeFields(data []byte) (Op, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	var op Op
	err := dec.Decode(&op)
	return op, err
}

// decodeError explains err, from decoding the fields in raw, with the shape of the nested field
// at fault when there is one. encoding/json names neither that field nor, for an unknown key,
// where the key is, so each nested field is decoded alone to find it.
func decodeError(def Def, raw map[string]json.RawMessage, err error) error {
	for _, f := range def.Fields {
		value, ok := raw[f.Name]
		if !ok || f.Shape == "" {
			continue
		}
		alone, merr := json.Marshal(map[string]json.RawMessage{f.Name: value})
		if merr != nil {
			continue
		}
		if _, ferr := decodeFields(alone); ferr != nil {
			msg := strings.Replace(typeError(def.Name, ferr).Error(), "json: ", f.Name+": ", 1)
			return fmt.Errorf("%s; it takes %s", msg, f.Shape)
		}
	}
	return typeError(def.Name, err)
}

// typeError explains a JSON value of the wrong type without naming Go types.
func typeError(op string, err error) error {
	var te *json.UnmarshalTypeError
	if !errors.As(err, &te) {
		return fmt.Errorf("%s: %w", op, err)
	}
	t := te.Type
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	want := map[reflect.Kind]string{
		reflect.Int: "a whole number", reflect.Float64: "a number", reflect.String: "a string", reflect.Bool: "true or false",
		reflect.Slice: "a list", reflect.Array: "a list", reflect.Map: "an object", reflect.Struct: "an object",
	}[t.Kind()]
	if want == "" {
		want = "of another type"
	}
	return fmt.Errorf("%s: %s must be %s, not %s", op, cmp.Or(te.Field, "the value"), want, te.Value)
}

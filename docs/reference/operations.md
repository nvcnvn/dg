# Operations

<!-- begin generated toc: go test ./cmd/dg -run TestDocs -update -->

- [Flags and JSON](#flags-and-json)
- [Results](#results)
- [The operations](#the-operations)

<!-- end toc -->

Operations:

<!-- begin generated operations-toc: go test ./cmd/dg -run TestDocs -update -->

[create_node](#create_node), [delete_node](#delete_node), [set_label](#set_label), [set_parent](#set_parent), [set_kind](#set_kind), [set_tags](#set_tags), [set_props](#set_props), [add_port](#add_port), [remove_port](#remove_port), [set_rows](#set_rows), [connect](#connect), [disconnect](#disconnect), [set_edge_labels](#set_edge_labels), [move](#move), [resize](#resize), [reorder](#reorder), [unpin](#unpin), [pin_port](#pin_port), [set_edge_hint](#set_edge_hint), [rename_id](#rename_id), [create_view](#create_view), [update_view](#update_view), [delete_view](#delete_view), [show_node](#show_node), [hide_node](#hide_node), [add_decoration](#add_decoration), [update_decoration](#update_decoration), [delete_decoration](#delete_decoration), [promote_decoration](#promote_decoration), [batch](#batch)

<!-- end operations-toc -->

Every change dg makes to a project's files is an operation: the editor sends them, `dg op` runs
them from the command line, and agents call them through `dg op` or the [MCP](cli.md#dg-mcp)
`op` tool. An operation edits the HCL files in place, keeping their comments and formatting, and
rewrites the layout files it touches.

```sh
mkdir my-diagram
dg op create_node -C my-diagram --kind service --label "API"
dg op create_node -C my-diagram --kind database --id db --label "Orders"
dg op connect -C my-diagram --from api --to db --label "SQL" --json
dg op move -C my-diagram --view main --id db --x 200 --y 0
```

The first operation generates the ID `api` from the label; `--id` names one. The model goes to
`model.hcl` and the position to `views/main.layout.yaml`.

## Flags and JSON

Each field of an operation is a flag of the same name. Lists such as `--tags`, `--include` and
`--order` are repeated, one value each, and `--props`, `--compartments`, `--overlays`, `--styles`
and `--decoration` take JSON. The exceptions are the ends of `connect` (`--from`, `--from-port`,
`--from-row`, `--to`, `--to-port`, `--to-row`), edge labels (`--start-label`, `--label`,
`--end-label`) and the hint of `set_edge_hint` (`--from-side`, `--to-side`, `--via x,y` repeated,
`--label-pos`).

As JSON, an operation is an object whose `op` field names it and whose other fields are its
arguments; an unknown or missing field is an error. `dg op batch` reads a list of them, or one,
from standard input or `--file`, and applies them in order as one change:

```sh
echo '[{"op": "set_label", "id": "db", "label": "Orders DB"},
       {"op": "set_tags", "id": "api-db", "type": "edge", "tags": ["hot"]}]' |
  dg op batch -C my-diagram
```

## Results

`dg op` prints each operation with the ID it created or changed (after a rename, the new ID) and,
for `move` and `resize`, the values it stored, then each file it wrote. With `--json` it prints
`{"results": [...], "files": [...], "warnings": [...]}`, the warnings being those the operation
added (`warnings` is left out when there are none), or `{"error": ..., "diagnostics": [...]}` when it was refused.

An operation, a batch included, is applied completely or not at all:

- It is refused, writing nothing, when it would add a validation error (such as `set_kind` to a
  kind that is not a container while the node has children), when it would change a `read_only`
  file (the error names it), while `dg.hcl` has errors or an HCL file does not parse, or when it
  would edit a layout file that has errors.
- Every file it changes is written together, through temporary files.
- If a file changed since it was read, such as a save in a text editor, the operation is applied
  again to the new content, and refused if it no longer applies.
- Writers of one project take turns: an operation waits up to 10 seconds for another to finish.

New blocks go at the end of the file that holds the node they relate to (the parent, or the
`from` node) when it is writable, else of the first writable model file, else of a new
`model.hcl`.

`dg op` has no geometry: `move` pins only the node it names, at coordinates relative to the
node's frame. [`dg export --format json`](cli.md#dg-export) gives every node's frame and its
drawn position `rel` in it, which `move` takes; to keep a whole automatic layout, pin every node
at its `rel` in one batch, as the editor's first drag does. A child of a stack or grid container
is marked `ordered_in` in the export: use `reorder`, not `move`, for it.

`dg op` has no undo: git is its undo. The editor keeps an undo history for its session.

## The operations

The rest of this page is generated from dg's definitions of the operations.

<!-- begin generated operations: go test ./cmd/dg -run TestDocs -update -->

### create_node

Append a node block. With view, also make the view show it (adding node.&lt;id&gt; to its include if needed) and, with x and y, pin it there; where a stack or grid container places it in the view, it goes in the container's order, before before or last.

| Flag | Value | Meaning |
|---|---|---|
| `--before` | string | with view, where a stack or grid container places the new node: the sibling it goes before in the container's order (default: last) |
| `--id` | string | ID of the new node (generated from the label, or the kind, when omitted) |
| `--kind` | string | (required) kind of the node (dg vocabulary lists them) |
| `--label` | string | label of the node; a newline character breaks a line (in a shell, $'a\nb') |
| `--parent` | string | ID of the container node |
| `--view` | string | view to show the node in |
| `-x` | integer | x relative to the node's frame (its nearest container drawn in the view, else the canvas), rounded to the project grid (an integer) |
| `-y` | integer | y relative to the node's frame, rounded to the project grid (an integer) |

As JSON, with the required fields: `{"op": "create_node", "kind": "service"}`

Fields (`?` marks the optional ones): `kind, id?, label?, parent?, view?, x?, y?, before?`

### delete_node

Remove a node with its edges, layout entries, decorations attached to it or placed in it, view selectors and node.&lt;id&gt; view rules. Refused while a view has of on it.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--recursive` |  | also delete the node's descendants; otherwise its children move to its parent, unpinned |

As JSON, with the required fields: `{"op": "delete_node", "id": "api"}`

Fields (`?` marks the optional ones): `id, recursive?`

### set_label

Change a node's label. A node created in the same batch without a label or ID takes a new ID from this first label (spec 6.3).

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--label` | string | (required) label of the node; a newline character breaks a line (in a shell, $'a\nb') |

As JSON, with the required fields: `{"op": "set_label", "id": "api", "label": "API"}`

Fields (`?` marks the optional ones): `id, label`

### set_parent

Set or remove a node's parent.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--parent` | string | ID of the container node; empty or omitted removes the parent |

As JSON, with the required fields: `{"op": "set_parent", "id": "api"}`

Fields (`?` marks the optional ones): `id, parent?`

### set_kind

Change the kind of a node or edge.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node, or of the edge with type edge |
| `--kind` | string | (required) kind of the node, or edge kind of the edge |
| `--type` | string | what id names: node (the default) or edge |

As JSON, with the required fields: `{"op": "set_kind", "id": "api", "kind": "service"}`

Fields (`?` marks the optional ones): `id, kind, type?`

### set_tags

Replace the tags of a node or edge.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node, or of the edge with type edge |
| `--tags` | string | (required) tags (simple IDs); an empty list removes them (repeatable) |
| `--type` | string | what id names: node (the default) or edge |

As JSON, with the required fields: `{"op": "set_tags", "id": "api", "tags": ["critical"]}`

Fields (`?` marks the optional ones): `id, tags, type?`

### set_props

Replace a node's properties.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--props` | JSON | (required) property values by name, of the types the kind declares; an empty object removes them, as JSON |

As JSON, with the required fields: `{"op": "set_props", "id": "api", "props": {"replicas": 3}}`

Fields (`?` marks the optional ones): `id, props`

### add_port

Add an instance port block to a node.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--name` | string | (required) port name |
| `--side` | string | side of the port: top, right, bottom, left or auto |

As JSON, with the required fields: `{"op": "add_port", "id": "api", "name": "grpc"}`

Fields (`?` marks the optional ones): `id, name, side?`

### remove_port

Remove an instance port block, and its pinned positions. Refused while an edge uses the port.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--name` | string | (required) port name |

As JSON, with the required fields: `{"op": "remove_port", "id": "api", "name": "grpc"}`

Fields (`?` marks the optional ones): `id, name`

### set_rows

Replace a node's rows and compartments.

| Flag | Value | Meaning |
|---|---|---|
| `--compartments` | JSON | (required) compartments of rows ({name, rows: [{name, type?, key?}]}); one compartment without a name holds rows declared directly in the node; an empty list removes every row, as JSON |
| `--id` | string | (required) ID of the node |

As JSON, with the required fields: `{"op": "set_rows", "id": "api", "compartments": [{"rows": [{"name": "id", "type": "uuid", "key": "pk"}]}]}`

Fields (`?` marks the optional ones): `id, compartments`

### connect

Append an edge block. With view, make the view's edges filter show it (adding edge.&lt;id&gt;) if needed.

| Flag | Value | Meaning |
|---|---|---|
| `--end-label` | string | label at the edge's end |
| `--from` | string | (required) node the edge starts at |
| `--from-port` | string | port of the start node |
| `--from-row` | string | row of the start node |
| `--id` | string | ID of the new edge (&lt;from&gt;-&lt;to&gt; when omitted) |
| `--kind` | string | edge kind |
| `--label` | string | label at the edge's middle |
| `--start-label` | string | label at the edge's start |
| `--tags` | string | tags (simple IDs); an empty list removes them (repeatable) |
| `--to` | string | (required) node the edge ends at |
| `--to-port` | string | port of the end node |
| `--to-row` | string | row of the end node |
| `--view` | string | view to show the edge in |

As JSON, with the required fields: `{"op": "connect", "from": {"node": "api"}, "to": {"node": "db"}}`

Fields (`?` marks the optional ones): `from, to, id?, kind?, labels?, tags?, view?`

### disconnect

Remove an edge with its hints and view references.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the edge |

As JSON, with the required fields: `{"op": "disconnect", "id": "api-db"}`

Fields (`?` marks the optional ones): `id`

### set_edge_labels

Replace an edge's labels; none removes them.

| Flag | Value | Meaning |
|---|---|---|
| `--end-label` | string | label at the edge's end |
| `--id` | string | (required) ID of the edge |
| `--label` | string | label at the edge's middle |
| `--start-label` | string | label at the edge's start |

As JSON, with the required fields: `{"op": "set_edge_labels", "id": "api-db"}`

Fields (`?` marks the optional ones): `id, labels?`

### move

Pin a node in a view at x, y relative to its frame. Only this node is pinned: dg op computes no geometry; dg export --format json gives every node's frame and rel, the coordinates move takes. Refused for a node a stack or grid container places (ordered_in in the export); reorder it instead.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--view` | string | (required) view ID |
| `-x` | integer | (required) x relative to the node's frame (its nearest container drawn in the view, else the canvas), rounded to the project grid (an integer) |
| `-y` | integer | (required) y relative to the node's frame, rounded to the project grid (an integer) |

As JSON, with the required fields: `{"op": "move", "view": "main", "id": "api", "x": 100, "y": 40}`

Fields (`?` marks the optional ones): `view, id, x, y`

### resize

Set the size of a node pinned in a view, or of a child of a stack or grid container.

| Flag | Value | Meaning |
|---|---|---|
| `-h` | integer | (required) height, positive, rounded to the project grid (at least one unit) (an integer) |
| `--id` | string | (required) ID of the node |
| `--view` | string | (required) view ID |
| `-w` | integer | (required) width, positive, rounded to the project grid (at least one unit) (an integer) |

As JSON, with the required fields: `{"op": "resize", "view": "main", "id": "api", "w": 160, "h": 80}`

Fields (`?` marks the optional ones): `view, id, w, h`

### reorder

Set the order in which a stack or grid container places its children in a view; children the order leaves out follow in sorted ID order.

| Flag | Value | Meaning |
|---|---|---|
| `--container` | string | (required) ID of the stack or grid container, drawn expanded in the view or the view's of |
| `--order` | string | IDs of the container's children the view draws, in order; empty or omitted removes the order of any node, even one that no longer places its children in order (repeatable) |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "reorder", "view": "main", "container": "lanes"}`

Fields (`?` marks the optional ones): `view, container, order?`

### unpin

Remove a node's position, size and pinned ports from a view; a container keeps its order.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "unpin", "view": "main", "id": "api"}`

Fields (`?` marks the optional ones): `view, id`

### pin_port

Pin a port of a node pinned in a view to a side, at an offset along it.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--offset` | number | position along the side, 0 to 1, kept to two decimals (a number) |
| `--port` | string | (required) port name |
| `--side` | string | (required) side: top, right, bottom or left |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "pin_port", "view": "main", "id": "api", "port": "grpc", "side": "right"}`

Fields (`?` marks the optional ones): `view, id, port, side, offset?`

### set_edge_hint

Replace an edge's routing hint in a view.

| Flag | Value | Meaning |
|---|---|---|
| `--from-side` | string | side the edge leaves its start node by: top, right, bottom or left |
| `--id` | string | (required) ID of the edge, or lift:&lt;from&gt;:&lt;to&gt; for a lifted edge (with %, : and # in those IDs written %25, %3A and %23) |
| `--label-pos` | string | position of the middle label along the route, 0 to 1 |
| `--to-side` | string | side the edge enters its end node by |
| `--via` | string | waypoint x,y in canvas coordinates (repeatable) |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "set_edge_hint", "view": "main", "id": "api-db", "hint": {"to_side": "top"}}`

Fields (`?` marks the optional ones): `view, id, hint`

### rename_id

Rename a node, edge or view everywhere it is referenced: model files, views.hcl, layout files (and the layout file's name).

| Flag | Value | Meaning |
|---|---|---|
| `--new` | string | (required) new ID |
| `--old` | string | (required) current ID |
| `--type` | string | (required) what old names: node, edge or view |

As JSON, with the required fields: `{"op": "rename_id", "type": "node", "old": "api", "new": "gateway"}`

Fields (`?` marks the optional ones): `type, old, new`

### create_view

Append a view block to views.hcl.

| Flag | Value | Meaning |
|---|---|---|
| `--collapse` | string | selectors of containers the view draws collapsed (repeatable) |
| `--direction` | string | the way the layers of the view's first layout run: down, right, up or left (default: down, or right when an edge ends at a row) |
| `--edges` | string | selectors of the edges the view shows: edge_kind.&lt;kind&gt;, tag.&lt;tag&gt;, edge.&lt;id&gt; (default: every edge whose ends are shown) (repeatable) |
| `--exclude` | string | selectors of nodes the view leaves out (repeatable) |
| `--id` | string | (required) ID of the new view (a simple ID) |
| `--include` | string | selectors of the nodes the view shows, as in views.hcl: all, node.&lt;id&gt; or node["&lt;id&gt;"], kind.&lt;kind&gt;, tag.&lt;tag&gt;, children.&lt;id&gt;, descendants.&lt;id&gt; (default [all]) (repeatable) |
| `--of` | string | ID of the container whose inside the view shows (drill-down) |
| `--overlays` | JSON | overlays [{id, members: [selectors], style?, label?}], as JSON |
| `--styles` | JSON | view style rules [{type?: style (default) or edge_style, label: a kind, tag.&lt;tag&gt;, node.&lt;id&gt; or edge.&lt;id&gt;, props: {property: value}}], as JSON |
| `--title` | string | view title |

As JSON, with the required fields: `{"op": "create_view", "id": "overview"}`

Fields (`?` marks the optional ones): `id, title?, include?, exclude?, collapse?, edges?, of?, direction?, overlays?, styles?`

### update_view

Change attributes and overlays of a view in views.hcl: the fields given replace the view's, unset removes them.

| Flag | Value | Meaning |
|---|---|---|
| `--collapse` | string | selectors of containers the view draws collapsed (repeatable) |
| `--direction` | string | the way the layers of the view's first layout run: down, right, up or left (default: down, or right when an edge ends at a row) |
| `--edges` | string | selectors of the edges the view shows: edge_kind.&lt;kind&gt;, tag.&lt;tag&gt;, edge.&lt;id&gt; (default: every edge whose ends are shown) (repeatable) |
| `--exclude` | string | selectors of nodes the view leaves out (repeatable) |
| `--id` | string | (required) view ID |
| `--include` | string | selectors of the nodes the view shows, as in views.hcl: all, node.&lt;id&gt; or node["&lt;id&gt;"], kind.&lt;kind&gt;, tag.&lt;tag&gt;, children.&lt;id&gt;, descendants.&lt;id&gt; (default [all]) (repeatable) |
| `--of` | string | ID of the container whose inside the view shows (drill-down) |
| `--overlays` | JSON | overlays [{id, members: [selectors], style?, label?}]; replaces every overlay of the view, keeping unchanged ones as they are written, as JSON |
| `--styles` | JSON | view style rules [{type?: style (default) or edge_style, label: a kind, tag.&lt;tag&gt;, node.&lt;id&gt; or edge.&lt;id&gt;, props: {property: value}}]; replaces every style rule of the view, keeping unchanged ones as they are written, as JSON |
| `--title` | string | view title |
| `--unset` | string | attributes to remove: title, include, exclude, collapse, edges, of, direction, overlays, styles (repeatable) |

As JSON, with the required fields: `{"op": "update_view", "id": "overview"}`

Fields (`?` marks the optional ones): `id, title?, include?, exclude?, collapse?, edges?, of?, direction?, overlays?, styles?, unset?`

### delete_view

Remove a view from views.hcl, and its layout file.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) view ID |

As JSON, with the required fields: `{"op": "delete_view", "id": "overview"}`

Fields (`?` marks the optional ones): `id`

### show_node

Make a view show an existing node: remove node.&lt;id&gt; from its exclude and, when its selectors still do not show the node, add node.&lt;id&gt; to its include. Refused when the view still does not show it (excluded by another selector, or inside a collapsed container).

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "show_node", "view": "main", "id": "api"}`

Fields (`?` marks the optional ones): `view, id`

### hide_node

Make a view stop showing a node: remove node.&lt;id&gt; from its include and, when its selectors still show the node, add node.&lt;id&gt; to its exclude.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the node |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "hide_node", "view": "main", "id": "api"}`

Fields (`?` marks the optional ones): `view, id`

### add_decoration

Add a decoration to a view's layout file (spec 5.7).

| Flag | Value | Meaning |
|---|---|---|
| `--decoration` | JSON | (required) decoration fields as in layout files: type (text, rect, ellipse, band, image, arrow, freehand), in or attach, x and y (in the frame) or dx and dy (from the attached node's center), w, h, z, text, src, points, path, fill, stroke, stroke_width, stroke_dash, opacity, text_color, font_size; type is required, as JSON |
| `--id` | string | ID of the new decoration (generated from its type when omitted) |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "add_decoration", "view": "main", "decoration": {"type": "text", "x": 0, "y": -60, "text": "Title"}}`

Fields (`?` marks the optional ones): `view, id?, decoration`

### update_decoration

Change fields of a decoration of a view: the fields given replace its, unset removes them.

| Flag | Value | Meaning |
|---|---|---|
| `--decoration` | JSON | decoration fields as in layout files: type (text, rect, ellipse, band, image, arrow, freehand), in or attach, x and y (in the frame) or dx and dy (from the attached node's center), w, h, z, text, src, points, path, fill, stroke, stroke_width, stroke_dash, opacity, text_color, font_size, as JSON |
| `--id` | string | (required) ID of the decoration |
| `--unset` | string | decoration fields to remove, such as attach, dx and dy (repeatable) |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "update_decoration", "view": "main", "id": "note"}`

Fields (`?` marks the optional ones): `view, id, decoration?, unset?`

### delete_decoration

Remove a decoration from a view.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the decoration |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "delete_decoration", "view": "main", "id": "note"}`

Fields (`?` marks the optional ones): `view, id`

### promote_decoration

Replace a decoration of a view with a model node pinned where the decoration is, at its size; a text decoration's text becomes the label, and a decoration in a frame becomes a child of that container. Refused for attached decorations, arrows and freehand lines.

| Flag | Value | Meaning |
|---|---|---|
| `--id` | string | (required) ID of the decoration |
| `--kind` | string | (required) kind of the node (dg vocabulary lists them) |
| `--label` | string | label of the new node (default: the text of a text decoration) |
| `--new` | string | ID of the new node (generated from the label, or the kind, when omitted) |
| `--view` | string | (required) view ID |

As JSON, with the required fields: `{"op": "promote_decoration", "view": "main", "id": "note", "kind": "service"}`

Fields (`?` marks the optional ones): `view, id, kind, new?, label?`

### batch

Apply operations in order as one change: all files are written together, or none.

| Flag | Value | Meaning |
|---|---|---|
| `--file` | string | file holding the operations as JSON (default standard input) |

As JSON, with the required fields: `{"op": "batch", "ops": [{"op": "create_node", "kind": "service"}]}`


<!-- end operations -->

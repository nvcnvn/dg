# Views and layouts

A view is one picture of the model: which nodes and edges it shows, how, and where they sit. What
a view shows is defined in `views.hcl`; where its nodes sit is in its layout file,
`views/<view>.layout.yaml`, which the editor writes.

<!-- begin generated toc: go test ./cmd/dg -run TestDocs -update -->

- [Views](#views)
  - [Selectors](#selectors)
  - [Collapse and edge lifting](#collapse-and-edge-lifting)
  - [Drill-down](#drill-down)
  - [Overlays](#overlays)
  - [View style rules](#view-style-rules)
- [Layout files](#layout-files)
  - [Pinned and unpinned nodes](#pinned-and-unpinned-nodes)
  - [Frames](#frames)
  - [Sizes](#sizes)
  - [Container policies](#container-policies)
  - [Ports](#ports)
  - [Edge hints](#edge-hints)
  - [Decorations](#decorations)

<!-- end toc -->

## Views

```hcl
view "overview" {
  title    = "Inference stack overview"
  include  = [all]
  collapse = [node.pool]
}

view "pool" {
  title   = "Inside the GPU pool"
  of      = node.pool
  include = [children.pool]
}

view "rdma" {
  title     = "RDMA fabric"
  include   = [kind.gpu-node, kind.switch, kind.storage]
  exclude   = [tag.retired]
  edges     = [edge_kind.rdma]
  direction = "right"
}
```

| Attribute | Meaning |
|---|---|
| `title` | shown in the editor's view switcher |
| `include` | selectors of the nodes the view shows; default `[all]`, or `[descendants.X]` with `of = node.X` |
| `exclude` | selectors of nodes it leaves out |
| `edges` | selectors of the edges it shows; by default every edge whose two ends it shows |
| `collapse` | selectors of containers drawn as one box ([collapse](#collapse-and-edge-lifting)) |
| `of` | the container whose inside the view shows ([drill-down](#drill-down)) |
| `direction` | `down`, `right`, `up` or `left`: the way the automatic layout runs |

A project without views has one, `main`, that shows everything. A view holds no coordinates and
no file paths.

`direction` sets the way the layers of the automatic layout run, from the nodes edges leave
toward the nodes they enter. Without it, layers run down, except in a view where an edge ends at
a row, where they run right, since rows attach at their left and right ends.

### Selectors

| Selector | Selects |
|---|---|
| `all` | every node |
| `node.api`, `node["kube-system/coredns"]` | one node |
| `kind.service` | the nodes of a kind and of the kinds that extend it |
| `tag.critical` | the nodes with a tag |
| `children.pool` | the direct children of a node |
| `descendants.pool` | everything inside a node, at any depth |
| `edge_kind.rdma`, `tag.hot`, `edge.api-db` | in `edges`: edges of an edge kind, with a tag, or one edge |

A node is in a view when `include` selects it, `exclude` does not, and it is not inside a
collapsed container. An edge is in a view when the `edges` filter passes it and both its ends, or
the boxes they are [lifted](#collapse-and-edge-lifting) to, are drawn. A selector that matches
nothing is a warning.

### Collapse and edge lifting

`collapse` draws each selected container as one box with a "+" badge under its label, and hides
everything inside it. An edge with an end hidden inside a collapsed container is drawn to that
container instead (the outermost collapsed one), without its port or row. Lifted edges that then
join the same two boxes are merged into one: a single edge keeps its labels and style; several are
labeled `×N` and styled by their edge kind when they share one. An edge whose two ends are lifted
to the same box is not drawn.

So a model can hold relationships between its most detailed elements, such as components, and a
higher-level view that collapses their systems shows the relationships between systems.

A lifted edge has the ID `lift:<from>:<to>`, which its [edge hint](#edge-hints) uses.

### Drill-down

`of = node.X` makes the view show the inside of the container `X`. In every other view that draws
`X`, the editor puts a drill-down badge (a magnifying glass) on it; clicking it opens the view, or
a menu when several views have `of = node.X`. A view with `of` shows breadcrumbs leading back to
the views that draw `X`. Exports draw neither the drill-down badge nor breadcrumbs.

When `X` is a `stack` or `grid` container and the view does not draw `X` itself, `X`'s policy
still places the children the view draws.

### Overlays

An overlay outlines a set of nodes that cut across containment, such as everything an outage
reaches. It is computed every time the view is drawn: a round-cornered convex hull around its
members' boxes.

```hcl
view "incident" {
  overlay "blast-radius" {
    members = [tag.degraded]     # selectors
    style   = "hull-red"         # an overlay_style; default "hull"
    label   = "Blast radius"
  }
}
```

Overlays are drawn over expanded containers and under every other node. A member hidden by a
collapse is represented by its collapsed container; an overlay with no member drawn is not drawn.

### View style rules

`style` and `edge_style` blocks inside a view apply only in that view, after every other style
(see [the cascade](styles.md#the-cascade)). They may also select one element:

```hcl
view "incident" {
  style "tag.degraded" {
    fill = "#FCE8E6"
  }
  style "node.payments" {
    stroke_width = 3
  }
  edge_style "edge.payments-psp" {
    animate = "flow"
  }
}
```

## Layout files

`views/<view>.layout.yaml` holds what was arranged by hand in a view; `layout_dir` in `dg.hcl`
changes the directory. The editor and [operations](operations.md) write it; you rarely edit it,
and dg rewrites it in one canonical form (sorted keys, one entry per line), so diffs stay small.

```yaml
version: 1
place:
  gateway:  {x: 0, y: 200}
  pool:     {x: 200, y: 100, w: 400, h: 300}
  worker-1: {in: pool, x: 20, y: 40}
  tor-1:
    x: 700
    y: 100
    ports:
      p12: {side: bottom, offset: 0.40}
  lanes:    {x: 0, y: 600, order: [ingest, train, serve]}
  ingest:   {h: 200}
edge_hints:
  "lift:router:pool": {to_side: left}
  w1-tor:
    from_side: bottom
    to_side: top
    via: [[300, 420]]
    label_pos: 0.50
decorations:
  - {id: title, type: text, x: 0, y: -60, text: "Inference stack"}
  - {id: rack-a, type: band, x: -20, y: -20, w: 200, h: 420, z: -1}
  - {id: note-1, type: text, attach: kvstore, dx: 0, dy: -40, text: "p99 4ms"}
```

Coordinates and sizes are whole numbers on the project's `grid`. In an ID, `%`, `:` and `#` are
written `%25`, `%3A` and `%23`.

### Pinned and unpinned nodes

A node listed in `place` with a position or size is **pinned**: drawn where stored. Any other node
is **unpinned** and placed when the view is drawn:

- In a view with nothing pinned, the whole view is laid out automatically, in layers along the
  view's `direction`, and the result is not saved. The editor says the layout is not saved.
- In a view with some nodes pinned, each unpinned node is placed near the nodes it is connected
  to, in ID order, with room for edges around it. The editor draws unpinned nodes faded.

Dragging a node pins it. In a view with nothing pinned, the first drag pins every node where it is
drawn, so accepting the automatic layout and adjusting it is one gesture. Unpin returns a node to
automatic placement. The same files always give the same drawing.

### Frames

A node's position is relative to its **frame**: the nearest container around it that the view
draws expanded, or the canvas. `in:` names the frame; without `in:` it is the canvas. Coordinates
in a container start at its content origin, past its label and padding, so moving a container is
one changed line and its contents move with it.

When the frame changes, because the model's `parent` or the view's `include` or `collapse`
changed, the stored position no longer matches. If the stored frame is still drawn, the node is
drawn where the stored position puts it, until it is next moved; otherwise it is placed as if
unpinned. `dg check` warns about such stale frames either way.

### Sizes

A node is as large as its pinned `w` and `h`, else as its label, icon and rows need with
`padding`, never smaller than `min_w` and `min_h`. A container without a pinned size grows to
hold its children. A container pinned too small for its children is drawn with a warning outline,
and the editor and `dg export` warn about it.

### Container policies

A container's [`layout` style](styles.md#container-properties) decides who places its direct
children:

| Policy | Children are placed by | The layout file stores |
|---|---|---|
| `free` (default) | their own `place` entries, or automatically | `x`, `y` per child |
| `stack` | the container, one after another along `direction`, `gap` apart | `order: [...]` on the container |
| `grid` | the container, row by row in `columns` columns, aligned by `align` | `order: [...]` on the container |

- Children missing from `order` follow in ID order; entries naming no drawn child are skipped.
- Dragging a child of a stack or grid reorders it; such children have no `x`, `y` (stored ones
  are ignored with a warning), but their `w`, `h` and pinned ports apply.
- With `stretch = true`, stacked children take the stack's full width (height, when
  `horizontal`).
- A child of a stack can be a `free` container itself: a swimlane holds freely placed steps.
  When nothing is pinned, the steps of all the lanes of a stack are laid out together, so they
  line up across the lanes in the order the edges run.

### Ports

`ports` under a pinned node pins a port to a `side` (`top`, `right`, `bottom` or `left`) at an
`offset` from 0 to 1 along it. Otherwise a port sits at its custom shape's anchor, else on its
declared side (ports on one side spread evenly), else, with `auto`, on the side facing the other
end of its edge.

### Edge hints

Routes are never stored; `edge_hints` steer them:

| Hint | Effect |
|---|---|
| `from_side`, `to_side` | the side the edge leaves its start or enters its end by, at the middle of that side |
| `via` | points the route passes through, in canvas coordinates |
| `label_pos` | where the middle label sits along the route, from 0 to 1 |

Without hints, an edge leaves and enters a box on the sides that face each other, edges on one
side are spread along it, and the middle label goes where it overlaps nothing.

### Decorations

Decorations are drawing that belongs to a view, not to the model: titles, notes, background bands,
legends, arrows.

| Type | Takes |
|---|---|
| `text` | `text` (lines break at `\n` only) |
| `rect`, `ellipse`, `band` | `w`, `h`; a band is a shaded background area |
| `image` | `src`, a project-relative SVG file, and `w`, `h` |
| `arrow` | `points`, at least two `[x, y]`; an arrowhead at the last |
| `freehand` | `path`, SVG path data |

- **Position:** `x`, `y` on the canvas, or in the frame of the node named by `in:`. With
  `attach: <node>` instead, the decoration is centered at the node's center moved by `dx`, `dy`,
  and follows the node. Points and paths are relative to the position.
- **Style:** `fill`, `stroke`, `stroke_width`, `stroke_dash`, `opacity`, `text_color` and
  `font_size`. Text is 14 pixels in `#202124`; outlines are `#5F6368`, 1.5 wide, without fill; a
  band is filled `#F1F3F4` without an outline.
- **Order:** a negative `z` draws the decoration behind every node, any other `z` over everything;
  `band` defaults to `-1`, the others to `1`.
- A decoration attached to or placed in a node the view does not draw is not drawn.
- The editor's "Promote to node…" turns a text, rect, ellipse, band or image decoration into a
  model node pinned where it was.

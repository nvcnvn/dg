# `dg` — Diagram tool with separate model and layout

Working name: `dg`.

This document is written for an implementing agent. It records every design decision, the file formats, the rules for geometry, and the functional requirements with acceptance criteria. Section 15 lists decisions that were considered and rejected, so they are not re-opened during implementation.

**How to read it:** sections 1–3 explain the ideas, sections 4–9 define the formats and rules, section 10 is the requirement list to implement against, section 13 is the build order.

---

## 1. Goal

A diagram-as-code tool where:

- **What exists** (the model) is written as text, reviewed in pull requests, and edited by humans or agents.
- **Where things sit** (the layout) is set by dragging in a browser editor and stored in a separate file.
- **Arrows are never stored.** They are routed around boxes every time the diagram is drawn.
- Someone who only uses the editor never needs to know there are two files. Someone who only writes text still gets a reasonable picture without opening the editor.

### In scope

Architecture and infrastructure diagrams, network topology, flowcharts and swimlanes, entity-relationship and class diagrams, state machines, dependency graphs, nested drill-down diagrams.

### Out of scope for v1

Diagrams where **position is data**: sequence diagrams (vertical order = message order), Gantt charts and timelines (x = time), charts (position = value), Venn/Euler diagrams (overlap = meaning). These may later become separate view types with derived layout (section 14). Also out of scope: real-time multi-user editing, freehand whiteboarding.

---

## 2. Core ideas

### 2.1 Four concerns

| Concern | Analogy | Where it lives | Contains |
|---|---|---|---|
| **Spec** | type definitions (like a Terraform provider schema) | packs, or project `spec.hcl` | element kinds, their ports, typed properties; edge kinds |
| **Style** | CSS | packs, or project `style.hcl` | how kinds and tags look; container layout policies; custom shapes |
| **Model** | HTML | `*.hcl` model files | node and edge instances, containment, tags, properties |
| **View** | a page that shows part of the document, plus where things sit on it | `views.hcl` + `views/<view-id>.layout.yaml` | which elements a view shows; positions; edge hints; decorations |

A **pack** is a spec and style shipped together (for example `generic`, `inference`, `kubernetes`). Most users only write the model and arrange views.

### 2.2 Rules that hold everywhere

1. **The identifier (ID) is not the label.** Changing a label never changes an ID.
2. **The ID does not encode hierarchy.** Containment is the `parent` attribute. Moving a node into another group never changes its ID.
3. **The kind is not the shape.** The model says `kind = "postgres"`; the style decides it is drawn as a cylinder. The model never contains colors, shapes or positions.
4. **Dependencies point one way:** `spec ← style`, `spec ← model ← view definitions ← layout files`. The model never names a view or a file path. Views refer to model elements only by ID. Layout files are found by convention.
5. **A layout gesture never changes the model by accident.** Re-parenting by drag requires an explicit modifier key (section 7.6).

### 2.3 Who computes what

| Step | Done by | Output |
|---|---|---|
| Parse, validate, decide which elements are in a view, collapse, edge lifting, style cascade | **Go** | *resolved view*: structure only, no absolute geometry |
| Text measurement, sizes, container policies, placement of unpinned nodes, port positions, routing | **Browser (TypeScript)** | geometry and the drawing |
| Writing files | **Go** | model edits through `hclwrite`; layout files regenerated |

Go never measures text and never computes absolute coordinates. This keeps the browser renderer the single source of geometry, so the editor and exported files always match.

### 2.4 Pinned and unpinned

- A node **listed** in its view's layout file is **pinned**: drawn where stored.
- A node **not listed** is **unpinned**: placed by the engine at render time, drawn faded in the editor.
- Dragging an unpinned node pins it. In a view with nothing pinned, the first drag pins every visible node where it is drawn, so accepting the first layout and adjusting it is one gesture (section 7.4).
- The layout file therefore records only positions a human or agent placed or accepted, never engine output nobody saw.

---

## 3. Technology decisions

### 3.1 Languages

**Go** for the backend and command-line tool; **TypeScript** for the browser editor and renderer.

### 3.2 Model language: HCL

HCL (HashiCorp Configuration Language) via `github.com/hashicorp/hcl/v2`, because:

- block labels make IDs mandatory by syntax: `node "api" { ... }`;
- `hclwrite` edits files while preserving comments and formatting, which the editor needs when it writes back;
- references such as `node.api` and `node["kube-system/coredns"]` parse as traversals without evaluation, so references can be validated and renamed safely;
- infrastructure engineers already read it.

Layout files are YAML, because they are machine-owned and rarely edited by hand.

### 3.3 Libraries

**Go**

| Purpose | Library |
|---|---|
| HCL parse, decode, diagnostics | `github.com/hashicorp/hcl/v2` (`hclsyntax`, `gohcl`) |
| HCL functions (`port_range`, `file`) | `github.com/zclconf/go-cty` function API |
| Comment-preserving writes | `github.com/hashicorp/hcl/v2/hclwrite` |
| YAML with stable key order | `go.yaml.in/yaml/v3` (`yaml.Node`; the maintained fork of the archived `gopkg.in/yaml.v3`) |
| File watching | `github.com/fsnotify/fsnotify` |
| WebSocket | `github.com/coder/websocket` |
| Headless export | `github.com/chromedp/chromedp` |
| GIF export | standard library `image/gif` |
| MCP server | `github.com/modelcontextprotocol/go-sdk` (MIT) |
| Embedding the web app in the binary | standard library `embed` |

**TypeScript (browser)**

| Purpose | Library |
|---|---|
| Canvas and interaction | `@joint/core` (JointJS 4.3 or newer, MPL-2.0) |
| Obstacle-avoiding orthogonal routing | `@joint/router-avoid` (MPL-2.0), running libavoid compiled to WebAssembly by `libavoid-js` (LGPL-2.1-or-later; `libavoid.wasm` stays a separate file) |
| First layout of a view with nothing pinned | `elkjs` (Eclipse Layout Kernel, EPL-2.0), in a Web Worker |
| Bundled font | Inter (OFL-1.1) from `@fontsource/inter` |
| Build | Vite, with `tsc` for type checking |
| Unit tests | Vitest |

**TypeScript (VS Code extension)**

| Purpose | Library |
|---|---|
| Language client | `vscode-languageclient` (MIT) |
| HCL highlighting | HCL TextMate grammar from `github.com/hashicorp/syntax` (MPL-2.0) |

### 3.4 Fonts

One bundled font, Inter at weight 400 with all its Unicode subsets, loaded by the browser renderer. System fonts are never used. Only the browser measures text. An exported SVG names the font (`Inter, sans-serif`) but does not embed it.

---

## 4. Project structure and files

```
my-diagrams/
├── dg.hcl                       # project file
├── spec.hcl                     # optional project-local kinds
├── style.hcl                    # optional project-local styles
├── model.hcl                    # hand-written model
├── generated/k8s.hcl            # optional generated model (read-only)
├── views.hcl                    # view definitions
├── views/
│   ├── overview.layout.yaml
│   └── rdma.layout.yaml
└── packs/inference/             # optional local pack
    ├── spec.hcl
    ├── style.hcl
    └── shapes/gpu.svg
```

### 4.1 Project file — `dg.hcl`

```hcl
project "inference-stack" {
  packs      = ["generic", "packs/inference"]   # "generic" is built in
  models     = ["model.hcl", "generated/*.hcl"]
  read_only  = ["generated/*.hcl"]              # editor and operations never modify these
  views      = "views.hcl"                      # default "views.hcl"
  layout_dir = "views"                          # default "views"
  grid       = 10                               # default 10
  font       = "fonts/Inter.ttf"                # optional; default is the bundled font
}
```

`dg.hcl` is optional and holds at most one `project` block; every field is optional. Without it, the project root is the current directory (or `-C <dir>`) and every field has its default.

- Paths are slash-separated, relative to the project root, and stay inside it.
- Symbolic links are followed only while they stay inside the project.
- `packs` lists local pack directories, each holding an optional `spec.hcl` and `style.hcl`; the project root itself is not a pack. The built-in `generic` pack is always loaded first, whether or not `packs` lists it.
- `models` lists glob patterns (`path.Match` syntax, no `**`). Default: every `*.hcl` in the root. `dg.hcl`, the `views` file, and the `spec.hcl` and `style.hcl` of the project and its packs are never model files. A pattern that matches no file is a warning; a directory it matches is an error.
- `read_only` lists glob patterns of files dg never writes: `dg fmt` skips them, and operations that would write them fail naming the file.
- `font` is checked to exist; drawing with it comes in M3.

Load order for kinds and styles: built-in `generic` pack, then packs in listed order, then project-local `spec.hcl` / `style.hcl`. A later `kind`, `edge_kind`, `shape` or `overlay_style` with the same name replaces the earlier one; a later `style` or `edge_style` with the same label overrides only the properties it sets (later wins, as in the cascade, section 8.4). Defining a name twice in one file is an error. `dg check` reports each override across files as information, a severity that `--strict` never turns into an error.

### 4.2 Spec — kinds

```hcl
kind "node" {}                         # root of all node kinds (built in)

kind "service" {
  extends = "node"
}

kind "group" {
  extends   = "node"
  container = true                     # other nodes may set parent to it
}

kind "gpu-node" {
  extends           = "node"
  ports             = ["nic0", "nic1", "pcie0"]
  default_port_side = "auto"           # side for instance ports with no side
  property "gpu_count" {
    type    = number
    default = 8
  }
}

kind "table" {
  extends = "node"
  rows    = true                       # instances may declare rows and compartments
}

edge_kind "default" {}
edge_kind "rdma" {}
```

A kind without `extends` extends `node`, and every edge kind other than `default` extends `default`. A kind inherits `container`, `rows`, `ports`, `default_port_side` and properties from the kind it `extends`, and may add ports and properties; redeclaring an inherited port or property is an error. Property types are `string`, `number` and `bool`.

### 4.3 Style

Style blocks are selected by a label:

| Label form | Matches |
|---|---|
| `"service"` | nodes of kind `service` and its sub-kinds |
| `"tag.deprecated"` | nodes with tag `deprecated` |
| `"node.kvstore"` | one node; only in a view (section 4.5) |

`edge_style` uses the same label forms for edges, with `"edge.<id>"` in place of `"node.<id>"`. `overlay_style` and `shape` define named overlay styles and custom shapes.

```hcl
style "service" {
  shape         = "rounded"
  fill          = "#E8F0FE"
  stroke        = "#1A73E8"
  padding       = 12
  min_w         = 120
  min_h         = 48
  max_w         = 240                  # labels wrap at this width
}

style "tag.deprecated" {
  stroke_dash = [4, 4]
  opacity     = 0.6
}

style "lane-set" {
  layout    = "stack"                  # free | stack | grid (section 7.3)
  direction = "vertical"
  gap       = 0
  stretch   = true
}

style "gpu-node" {
  shape = "gpu-node"                   # custom shape below
}

edge_style "rdma" {
  stroke      = "#D93025"
  stroke_dash = [6, 3]
  arrow_end   = "triangle"
  routing     = "orthogonal"           # orthogonal | polyline | straight
}

overlay_style "hull-red" {
  stroke  = "#D93025"
  fill    = "#FCE8E6"
  padding = 16
}

shape "gpu-node" {
  svg      = file("shapes/gpu.svg")
  text_box = [0.1, 0.6, 0.8, 0.3]                 # x, y, w, h as fractions of the box
  anchors  = { nic0 = [1, 0.3], nic1 = [1, 0.7], pcie0 = [0, 0.5] }
  stretch  = "nine-slice"                         # nine-slice | uniform | none
}
```

**Node style properties:** `shape`, `fill`, `stroke`, `stroke_width`, `stroke_dash`, `corner_radius`, `opacity`, `text_color`, `font_size`, `font_weight`, `icon`, `image`, `padding`, `min_w`, `min_h`, `max_w`, `z`. `icon` and `image` (the picture for the `image` shape) take `file("…")`, an SVG resolved relative to the file that calls it.
**Container properties:** `layout`, `direction` (`horizontal` | `vertical`), `gap`, `stretch`, `columns`, `align` (`start` | `center` | `end`), `label_side` (`top` | `left`; `left` draws the label rotated along the left edge, as on a swimlane).
**Overlay style properties:** `fill`, `stroke`, `stroke_width`, `stroke_dash`, `opacity`, `padding`.
**Values:** colors are `#RGB`, `#RRGGBB` or `none`; sizes, widths, gaps and padding are numbers of at least 0, `font_size` at least 1; `opacity` is 0–1; `font_weight` is 100–900 in steps of 100; `z` and `columns` are whole numbers; `stroke_dash` is a list of numbers.
**Custom shapes:** a shape's name may not be a built-in shape. Each of its `anchors` must name a port of every kind whose style (by kind, section 8.4 step 1–2) uses the shape; ports without an anchor are placed by section 7.5.
**Edge style properties:** `stroke`, `stroke_width`, `stroke_dash`, `arrow_start`, `arrow_end`, `routing`, `font_size`, `animate`.
**Animation** (`animate`): `none` (default) or `flow`. `flow` draws the edge dashed (its `stroke_dash`, or `[6, 4]`) and moves the dashes from `from` toward `to` by one dash period every half second, so every animated edge repeats with the same half-second period. Animation is style, so it is selected by kind or tag (`edge_style "tag.hot" { animate = "flow" }`), never set in the model.
**Arrow markers** (`arrow_start`, `arrow_end`): `none`, `arrow` (open), `triangle`, `triangle-open`, `diamond`, `diamond-open`, `circle`, `circle-open`, and crow's-foot `crow-one`, `crow-zero-one`, `crow-many`, `crow-one-many`, `crow-zero-many`.

**Built-in shapes:** `rect`, `rounded`, `stadium`, `ellipse`, `diamond`, `cylinder`, `hexagon`, `parallelogram`, `document`, `person`, `cloud`, `text`, `image`, `point` (filled dot, for junctions and initial states), `bullseye` (final states), `table` (rows, section 5.4).

### 4.4 Model

```hcl
node "gateway" {
  kind  = "service"
  label = "Inference Gateway"
}

node "pool" {
  kind  = "group"
  label = "GPU Pool"
}

node "worker-1" {
  kind   = "gpu-node"
  label  = "Worker 1"
  parent = node.pool
  tags   = ["critical"]
  props  = { gpu_count = 8 }
}

node "tor-1" {
  kind  = "switch"
  ports = port_range("p", 1, 48)        # instance ports: p1 .. p48
}

node "api" {
  kind = "service"
  port "grpc" {
    side = "right"
  }
}

node "kube-system/coredns" {             # quoted ID, e.g. from a generator
  kind = "service"
}

edge "w1-tor" {
  from  = node.worker-1.port.nic0
  to    = node.tor-1.port.p1
  kind  = "rdma"
  label = "100G"
}

edge "dns" {
  from = node["kube-system/coredns"]
  to   = node.api.port.grpc
}
```

- `node` and `edge` blocks must have exactly one label: the ID.
- Defaults: node `kind = "node"` and `label` = ID; edge `kind = "default"` and no label.
- `from` / `to`: `node.<id>`, `node.<id>.port.<port>`, or `node.<id>.row.<row>`; index form for IDs that are not simple (`node["a/b"]`, `node.t.row["save()"]`).
- `label` is shorthand for `labels = { mid = "..." }`. Full form: `labels = { start = "1", mid = "places", end = "0..*" }`. Setting both is an error.
- `tags` is a list of simple IDs; `props` is an object literal whose keys the kind declares and whose values have the declared type exactly (no conversion). Properties without a value take the kind's `default`, if any.

**Functions**, available in every HCL file:
- `port_range(prefix, from, to)`: the list `prefix<from>` … `prefix<to>`; `from` and `to` are whole numbers between -10⁹ and 10⁹, `from ≤ to`, at most 10,000 names.
- `file(path)`: the contents of a UTF-8 text file, `path` relative to the directory of the file that calls it and inside the project.

### 4.5 View definitions — `views.hcl`

```hcl
view "overview" {
  title    = "Inference stack overview"
  include  = [all]
  collapse = [node.pool]                 # drawn as one box; edges lifted (section 8.3)
}

view "pool" {
  title   = "Inside the GPU pool"
  of      = node.pool                    # drill-down target (section 8.2)
  include = [children.pool]
}

view "rdma" {
  title   = "RDMA fabric"
  include = [kind.gpu-node, kind.switch, kind.storage]
  edges   = [edge_kind.rdma]
}

view "incident" {
  include = [all]

  overlay "blast-radius" {               # drawn around members, never stored as geometry
    members = [tag.critical]
    style   = "hull-red"
    label   = "Blast radius"
  }

  style "tag.new" {                      # view-only style rule
    fill = "#E6F4EA"
  }
  style "node.kvstore" {
    stroke_width = 3
  }
}
```

**Selectors** (used in `include`, `exclude`, `collapse`, overlay `members`): `all`, `node.<id>` / `node["<id>"]`, `kind.<kind>` (includes sub-kinds), `tag.<tag>`, `children.<id>` (direct children), `descendants.<id>`. `edges` uses `edge_kind.<kind>`, `tag.<tag>` and `edge.<id>`; by default all edges whose endpoints are both visible are included.

`include` defaults to `[all]`, or to `[descendants.X]` when `of = node.X` is set.

If no view is defined, the project behaves as if `views.hcl` contained `view "main" { include = [all] }`, so a model with no views still draws. The first operation that writes `views.hcl` writes that block out.

A view definition contains no coordinates and no file paths.

### 4.6 Layout file — `views/<view-id>.layout.yaml`

```yaml
version: 1
place:                                   # pinned nodes; keys sorted
  gateway:  {x: 0,   y: 200}
  pool:     {x: 200, y: 100, w: 400, h: 300}
  worker-1: {in: pool, x: 20, y: 40}     # relative to pool's content origin (section 7.1)
  tor-1:
    x: 700
    y: 100
    ports:                               # pinned port positions
      p12: {side: bottom, offset: 0.40}
  lanes:    {x: 0, y: 600, order: [ingest, train, serve]}   # stack/grid container
edge_hints:                              # keys sorted
  w1-tor:              {from_side: bottom, to_side: top, via: [[300, 420]], label_pos: 0.50}
  "lift:router:pool":  {to_side: left}   # hint for a lifted edge
decorations:
  - {id: title,  type: text, x: 0, y: -60, text: "Inference stack"}
  - {id: rack-a, type: band, x: -20, y: -20, w: 200, h: 420, z: -1}
  - {id: note-1, type: text, attach: kvstore, dx: 0, dy: -40, text: "p99 4ms"}
```

- `x`, `y`, `w`, `h` are integers on the project grid. Port `offset` and `label_pos` are written with two decimal places.
- `in` names the frame the coordinates are relative to (section 7.1). No `in` means the canvas.
- Wherever an ID appears in a layout file, as a key or a value, the characters `%`, `:` and `#` inside it are percent-encoded (`%25`, `%3A`, `%23`), so `lift:` keys stay unambiguous for quoted IDs such as `system:controller`.
- The file is fully regenerated on every write: sorted keys, one entry per line where possible.

---

## 5. Elements

### 5.1 Node

Has an ID, kind, label, optional parent, tags, properties, optional instance ports and rows.

### 5.2 Container

A node whose kind has `container = true`. Children point to it with `parent`. Containment is a tree; cycles are an error.

### 5.3 Port

A named attachment point on a node.
- **Kind ports** are declared in the spec (`ports = [...]`, which may call `port_range`).
- **Instance ports** are declared in the model (`port "name" { side = ... }` blocks or `ports = port_range(prefix, from, to)`).
- A node's ports = kind ports + instance ports; duplicates are an error.
- `side` ∈ `top | right | bottom | left | auto`.

### 5.4 Rows and compartments (tables, classes)

Nodes of a kind with `rows = true` may declare rows, optionally grouped in compartments:

```hcl
node "orders" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "pk"
  }
  row "customer_id" {
    type = "uuid"
    key  = "fk"
  }
}

node "user" {
  kind  = "class"
  compartment "fields" {
    row "name" {}
  }
  compartment "methods" {
    row "save()" {}
  }
}
```

A row takes optional strings `type` and `key` (such as `pk`, `fk`). A node declares its rows either all directly or all in compartments. Row names are unique within the node, across compartments; compartment names are unique within the node.

A row is drawn as a line inside the box **and acts as a port** whose attachment point is the left or right end of that line (whichever faces the other endpoint). Reference: `node.orders.row.customer_id`, or `node.user.row["save()"]`.

### 5.5 Edge

ID, `from`, `to`, kind, labels (`start`, `mid`, `end`), tags.
- Several edges between the same pair are allowed (each has its own ID).
- Self-loops are allowed.
- Edges may attach to containers.

### 5.6 Junction

A node of the built-in kind `junction` (shape `point`, no label) used for buses and fan-out (one line splitting to many). v1 routes edges to the junction as ordinary edges; libavoid's hyperedge optimization is a later improvement.

### 5.7 Decoration (view only)

Types: `text`, `rect`, `ellipse`, `band` (shaded background area), `image`, `arrow` (unattached), `freehand` (SVG path).
- IDs are unique within their view.
- Fields: every type takes `x`, `y`, relative to the canvas or to the frame named by `in:`. `rect`, `ellipse`, `band` and `image` take `w`, `h`; `text` takes `text`; `image` takes `src` (a project-relative path); `arrow` takes `points` (a list of `[x, y]`); `freehand` takes `path` (SVG path data).
- A decoration may set `fill`, `stroke`, `stroke_width`, `stroke_dash`, `opacity`, `text_color` and `font_size` inline.
- `z`: draw order relative to nodes (negative = behind nodes; default `-1` for `band`, `1` otherwise).
- `attach: <node-id>` with `dx`, `dy` (offset from the node's center) makes the decoration follow the node. It is not drawn while that node is not drawn.
- **Promote to model**: converts a decoration into a model node; its geometry becomes the new node's placement in that view.

### 5.8 Overlay (view only)

A named outline drawn around the boxes of its members, computed at render time from selectors. Used for groupings that overlap or cut across containment. Never stored as geometry.

---

## 6. Identifiers

### 6.1 Format

- **Simple IDs:** match `^[A-Za-z_][A-Za-z0-9_-]*$`; referenced as `node.api`.
- **Quoted IDs:** any other non-empty string up to 200 characters without control characters; referenced with index syntax `node["kube-system/coredns"]`. Intended for generated models.
- Reserved, cannot be simple IDs: `all`, `node`, `kind`, `tag`, `children`, `descendants`, `edge_kind`, `port`, `row`, `lift`.
- Node IDs and edge IDs are separate namespaces. Port and row names are unique within their node and may be simple or quoted.
- Kind, edge kind, tag, view, overlay and decoration IDs must be simple IDs.

### 6.2 Generation (editor and operations)

```go
func newID(kind, label string, taken map[string]bool) string {
    base := slug(label)                  // lowercase, [a-z0-9-], collapse dashes, trim, at most 64 characters
    if base == "" { base = slug(kind) }
    if !startsWithLetter(base) { base = "n-" + base }
    id := base
    for i := 2; taken[id] || reserved[id]; i++ {
        id = fmt.Sprintf("%s-%d", base, i)
    }
    return id
}
```

Edge IDs: `slug(fromNode) + "-" + slug(toNode)` (node IDs only, without port or row) with the same collision suffix. Generated IDs are always simple.

### 6.3 Freezing

- An ID is frozen once created.
- One exception: a node created with an ID derived from its kind (no label yet) is re-slugged **once**, from the first label entered in the same editing session.
- Changing a label never changes an ID.

### 6.4 Renaming

`rename_id {type: node | edge | view, old, new}` is an explicit atomic refactor. All files are written together (temporary files, then rename).
- **Node:** the block label, every reference in all writable model files, selectors, `of` and `node.<id>` rules in `views.hcl`, and keys and values (`in:`, `attach:`, `order:` entries, `lift:` keys) in every layout file.
- **Edge:** the block label, `edge.<id>` selectors and rules in `views.hcl`, and `edge_hints` keys in every layout file.
- **View:** the block label and the layout file's name.

If the definition or any reference lives in a read-only file, the rename is refused.

### 6.5 Deferred

Showing the same node twice in one view (layout keys like `db#2`) is not in v1. The `#` character is reserved in layout keys.

---

## 7. Geometry rules

All geometry is computed in the browser (section 2.3).

### 7.1 Frames and relative coordinates

- A node's **frame** in a view is its nearest ancestor that is visible and not collapsed in that view; if none, the canvas.
- Stored coordinates are relative to the frame's **content origin**: for a container, the top-left of its inside, past its label and padding; for the canvas, (0, 0). `in:` names the frame; omitted means the canvas. A negative coordinate inside a container is drawn as 0.
- Moving a container therefore changes one layout line; its descendants move with it. Changing a container's label or padding moves its content with it instead of under the label.
- **Stale frame:** if the stored `in:` differs from the node's current frame (the model's `parent` changed, or the view's `include` or `collapse` changed):
  - if the stored frame is still drawn in this view, the browser converts the coordinates to the current frame for display; the entry keeps its stored values until the node is next moved or unpinned (Go has no geometry to convert them);
  - otherwise the node is treated as unpinned.
  In both cases `dg check` reports a warning.
- Coordinates in operations (FR-12) are always relative to the node's current frame.

### 7.2 Sizes

1. Pinned `w` / `h` in the layout, if present.
2. Otherwise measured content: label (wrapped at `max_w`), icon, rows, plus `padding`; never smaller than `min_w` / `min_h`.
3. Shapes with `stretch = "uniform"` keep their aspect ratio when resized.
4. Containers without a pinned size fit their label, padding, and children (from the content origin to the furthest child edge).
5. A container with a pinned size too small for its children is drawn with a warning outline; children are not clipped.
6. A collapsed container is sized as a leaf node by rules 1–2, using this view's pinned size if any.

### 7.3 Container layout policies

Set in style (`layout = ...`), applying to a container's **direct** children only.

| Policy | Children positioned by | Stored in layout file |
|---|---|---|
| `free` (default) | their own `place` entries, or the placer if unpinned | `x`, `y` per child |
| `stack` | the container: one after another along `direction`, separated by `gap`; with `stretch = true` children fill the cross axis | `order: [...]` on the container |
| `grid` | the container: row-major fill with `columns`, aligned per `align` | `order: [...]` on the container |

- In `stack` and `grid` containers, dragging a child **reorders** it; children never get `x`/`y`. Stored `x`/`y` on such a child is ignored with a warning.
- Children missing from `order` are appended in sorted ID order.
- In `stack`, `stretch` sets each child's cross-axis size to the container's; a child's pinned size along the main axis still applies. In `grid`, a column is as wide as its widest child and a row as tall as its tallest.
- A child of a `stack` container can itself be a `free` container (a swimlane holding freely placed steps).

### 7.4 Placement of unpinned nodes

- If a view has **no** pinned nodes at all: run `elkjs` (layered algorithm) once, with nodes and edges given in sorted ID order, and `stack`/`grid` containers laid out by their policy first and passed as fixed-size boxes. The result is not saved.
- Otherwise, for each unpinned node in **sorted ID order**: start from the centroid of its already-placed neighbors (fallback: its frame's center, then the canvas origin), then search outward in a spiral on the grid until it keeps two grid units of clearance from everything in its frame.
- In a view with nothing pinned, the editor sends the first drag as one batch (FR-12) that pins every visible node where it is drawn, plus the drag itself. Otherwise pinning one node would hand every other node from the elk layout to the spiral placer and the picture would jump.
- `dg op` has no geometry: a `move` from the command line pins only the node it names. Agents read drawn positions with `dg export --format json`.
- Placement is deterministic. Pinned nodes never move.
- Unpinned nodes are drawn faded in the editor and normally in export.

### 7.5 Ports and routing

Port position, in priority order:
1. the custom shape's anchor with the same name;
2. a pinned position in the layout (`ports: {p12: {side, offset}}`);
3. the declared `side`, with ports on the same side spread evenly in declaration order;
4. `auto`: the side facing the other endpoint's center.

Routing:
- `@joint/router-avoid`, orthogonal by default (`routing` in edge style).
- Leaf nodes and collapsed containers are obstacles, except an edge's own endpoints. Expanded containers are not: edges cross container borders, so an edge between two swimlanes can pass through the lanes between them.
- Hints: `from_side` / `to_side`, `via` waypoints, `label_pos` (0–1 along the path) for the middle label; start and end labels are placed next to their ends.
- Self-loops route around one corner of the node.
- Moving a node reroutes only the affected edges, live during dragging.
- Routes are never written to any file.

### 7.6 Dragging rules

- **Without a modifier key:** a node stays inside its frame. If the frame is a container without a pinned size, the container grows; if it has a pinned size, the node's position is clamped.
- **With the re-parent modifier** (Alt / Option): containers under the pointer are highlighted; dropping onto the innermost one sets `parent` (a model change); dropping on empty canvas clears `parent`. The parent change and the node's new position, or its place in a `stack`/`grid` `order`, are sent as one batch. The editor shows an undo toast.
- Dropping a node onto itself or one of its descendants is refused, as is re-parenting a node whose definition is read-only.
- Dragging a multi-node selection sends one batch of moves.

---

## 8. Views

### 8.1 Membership

A node is in a view if it matches `include` and not `exclude`, and is not inside a collapsed container. An edge is in a view if both its (possibly lifted) endpoints are visible and it passes the `edges` filter.

### 8.2 Drill-down

`of = node.X` declares that a view shows the inside of container `X`.
- In every other view, `X` shows a drill-down badge; clicking it opens the view. If several views have `of = node.X`, the editor shows a menu.
- The editor shows breadcrumbs: for a view with `of = node.X`, the views whose `of` targets are ancestors of `X`, outermost first.

### 8.3 Collapse and edge lifting

`collapse = [...]` draws each selected container as a single box with a "+" badge and hides its descendants, including collapsed ones.
- An edge endpoint hidden inside a collapsed container is **lifted** to the outermost collapsed container that holds it (ports and rows are dropped).
- An edge whose two drawn endpoints are the same collapsed container is hidden.
- Lifted edges with the same drawn `from` and `to` are merged into one. With N = 1 it keeps its labels and style; with N > 1 it is labeled `×N` and styled by its edges' kind if they share one, otherwise by `default`.
- A lifted edge's layout key is `lift:<from>:<to>` (in drawn direction, IDs encoded per section 4.6).
- An overlay member hidden by a collapse is represented by its drawn collapsed container. An overlay with no drawn members is not drawn.

### 8.4 Style cascade

For each element, property by property, later wins:
1. styles of ancestor kinds, starting from the root (`node` for nodes, `default` for edges);
2. style of its own kind;
3. tag styles, in the order the tags are listed on the element;
4. view rules in this order: kind rules, tag rules, `node.<id>` / `edge.<id>` rules.

There are no defaults outside the cascade: the built-in `generic` pack's `style "node"` and `edge_style "default"` set every property that has a default, and a pack or project overrides them like any other style.

---

## 9. Built-in `generic` pack

| Kind | Notes |
|---|---|
| `node` | root kind, shape `rect` |
| `service` | `rounded` |
| `database` | `cylinder` |
| `queue` | `parallelogram` |
| `storage` | `cylinder` |
| `person` | `person`, uniform stretch |
| `external` | `rounded`, dashed |
| `note` | `document` |
| `group` | container, `free` |
| `lane-set` | container, `stack` vertical, stretch |
| `lane` | container, `free`, `label_side = "left"` |
| `grid-group` | container, `grid`, 3 columns |
| `decision` | `diamond` |
| `terminal` | `stadium` |
| `state` | container, `free`, `rounded` |
| `initial` | `point`, no label drawn |
| `final` | `bullseye`, no label drawn |
| `table` | `rows = true`, shape `table` |
| `class` | `rows = true`, shape `table` with compartments |
| `junction` | shape `point`, no label drawn |

Edge kinds:

| Edge kind | Drawn as |
|---|---|
| `default` | solid, `arrow` at `to` |
| `async` | dashed, `arrow` at `to` |
| `association` | solid, no markers |
| `inherits` | solid, `triangle-open` at `to` |
| `realizes` | dashed, `triangle-open` at `to` |
| `composition` | solid, `diamond` at `from` (the whole) |
| `aggregation` | solid, `diamond-open` at `from` (the whole) |
| `dependency` | dashed, `arrow` at `to` |

Edge tag styles for entity-relationship cardinality: `start-one`, `start-zero-one`, `start-many`, `start-one-many`, `start-zero-many` set `arrow_start` to the matching crow's-foot marker, and `end-…` the same for `arrow_end`. An edge tagged `["start-one", "end-zero-many"]` draws one-to-zero-or-many.

Overlay style: `hull` (neutral).

---

## 10. Functional requirements

Each requirement has acceptance criteria (AC).

### A. Project and language

**FR-1 Load project.** Parse `dg.hcl`, packs (built-in and listed), project-local spec and style, model files, `views.hcl`, layout files. Evaluate HCL functions `port_range` and `file`.
- AC: syntax errors report file, line, column, message.
- AC: missing optional files are not errors.
- AC: a layout file is an error at its line and column (the line only, for YAML syntax errors) when it is not one YAML document, lacks `version: 1`, has unknown fields, values of the wrong type, a negative `w` or `h`, or IDs with a raw `:` or `#` or another escape than those of section 4.6. Port names in layout files are encoded like IDs.

**FR-2 Validate (`dg check`).** Errors:
- duplicate node, edge, view, overlay or decoration IDs; invalid or reserved IDs;
- undefined node, port, row, kind, edge kind, or selector target; `extends` cycles;
- `parent` pointing to a non-container kind; containment cycles;
- duplicate port, row or compartment names on a node; a custom shape anchor naming no port of a kind drawn with it (section 4.3);
- rows declared on a kind without `rows = true`;
- properties the kind does not declare, or of the wrong type;
- unknown style properties or invalid style values;
- `of` or `collapse` naming a non-container;
- layout files with no matching view definition.

Warnings:
- layout entries naming IDs that no longer exist (orphans);
- style labels naming undefined kinds; selectors that match nothing;
- stale frames (section 7.1);
- `x`/`y` on children of `stack`/`grid` containers;
- pinned containers too small for their children (computed at render time; reported by the editor and by `dg export`).

Information: a kind, edge kind, shape or style overridden by a later pack or the project (section 4.1).

- AC: exit code 0 if no errors, 1 if errors, 2 on internal failure (such as an unreadable project directory); `--strict` makes warnings errors.
- AC: `--json` prints a JSON array of diagnostics, each with `severity` (`error`, `warning` or `info`), `summary`, `detail`, and, when it has a position, `file`, `line`, `column`, `end_line` and `end_column`. `dg serve` (FR-10) and the language server send the same shape.
- AC: `file` is the project-relative slash path, the same whatever `-C` and the working directory are; files of the built-in pack are `<generic>/spec.hcl` and `<generic>/style.hcl`, which no project path can be. A diagnostic about one element of a list (a pattern, port, tag or selector) points at that element.
- AC: diagnostics are sorted by file, line and column.

**FR-3 Format (`dg fmt`).** Format every writable HCL file the project loads (`dg.hcl`, local packs, project `spec.hcl` and `style.hcl`, model files, `views.hcl`) with `hclwrite.Format`; regenerate layout files in canonical form. Files matched by `read_only` are never written; files that do not parse are left as they are and reported. All changed files are written together (temporary files, then rename), keeping each file's permissions and writing through symbolic links that stay inside the project, and `dg fmt` prints the path of each. If `dg.hcl` has errors, `dg fmt` writes nothing, since `read_only` may be incomplete. `dg fmt --check` lists the files that would change, writes nothing, and exits 1 if there are any.

### B. Resolution (Go)

**FR-4 Resolve a view.** Produce the resolved view: visible nodes with kind, label, merged style (section 8.4), frame, ports, rows, pin data from the layout (relative coordinates, order lists), collapse state, drill-down targets, read-only flags; visible and lifted edges with labels, style, hints; decorations; overlays with member IDs.
- AC: no absolute coordinates are computed in Go.
- AC: lifting and merging follow section 8.3 exactly.

### C. Geometry and rendering (browser)

**FR-5 Sizes** per section 7.2.
**FR-6 Container policies** per section 7.3.
**FR-7 Placement** per section 7.4.
- AC: identical input produces identical positions.
**FR-8 Ports and routing** per section 7.5.
- AC: dragging with 200 nodes and 400 edges stays at 60 frames per second.
**FR-9 Draw** nodes, rows, edges with start/mid/end labels, animated edges, decorations in `z` order, overlays as padded outlines around member boxes, drill-down and collapse badges.

### D. Editor

**FR-10 Serve (`dg serve`).** HTTP server, open the browser, WebSocket protocol:
- server → client: `snapshot` (resolved view), `diagnostics`, `ack` (operation result, including generated IDs), `error`;
- client → server: `op`, `select-view`, `undo`, `redo`.
- Undo history belongs to the `dg serve` session and is shared by its clients; `dg op` has none (git is its undo). An undo whose inverse no longer applies, because a file changed outside the editor, fails with an error and is dropped.
- AC: a change on disk produces a new `snapshot` within 300 milliseconds.
- AC: if the project fails to parse, the editor shows diagnostics and keeps the last good snapshot visible, read-only.

**FR-11 Interactions.**
- Pan, zoom, select (click, shift-click, box select).
- Drag to move (pins), resize handles (pins `w`/`h`), reorder within `stack`/`grid` containers.
- Re-parent with the modifier key (section 7.6).
- Palette of kinds from loaded packs; drag onto canvas or into a container to create.
- Add an existing model node to the view, searched by ID or label.
- Inspector for the selection: kind, tags, properties.
- Copy and paste nodes with the edges among them; pasted nodes get new IDs (section 6.2) and are pinned at the paste point.
- Draw connections from node, port, or row; choose edge kind.
- Drag an edge end to a side (`from_side`/`to_side`), drag the middle label (`label_pos`), add/remove via points.
- Inline label editing; row editing for table and class nodes.
- Context menu: hide from view (edits the view's selectors), delete from model, rename ID, unpin, add port, promote decoration, open drill-down view.
- Decorations toolbar; attach a decoration to a node.
- One-shot align and distribute actions that write coordinates (no stored constraints).
- View switcher, breadcrumbs, new-view dialog (title, selectors, collapse, overlays).
- Read-only elements show a lock icon; model edits on them are disabled, layout edits are allowed.
- AC: dragging runs locally; only the drop sends an operation.
- AC: a gesture that changes several things (multi-select move, align, paste, re-parent with move) sends one batch and is one undo step.

### E. Operations and files

**FR-12 Operation API.** One API used by the editor, `dg op`, and agents. Applied by Go, which writes files and returns the result.

| Operation | Effect |
|---|---|
| `batch {ops}` | apply the operations in order, atomically, as one undo step |
| `create_node {kind, label?, parent?, view?, x?, y?}` | append `node` block; if `view` given, pin at `x`,`y` (relative to the new node's frame) and, if the view's selectors would not show it, add `node.<id>` to its `include` |
| `delete_node {id, recursive?}` | remove the block, its edges, its layout entries and attached decorations, and its entries in view selectors and `node.<id>` rules; children move to its parent (unpinned), or are deleted too with `recursive`; refused while a view has `of` on it |
| `set_label {id, label}` | change `label` |
| `set_parent {id, parent \| null}` | set or remove `parent` |
| `set_kind {id, kind}`, `set_tags {id, tags}`, `set_props {id, props}` | change attribute |
| `add_port {id, name, side?}`, `remove_port {id, name}` | instance `port` block |
| `set_rows {id, compartments}` | replace rows/compartments |
| `connect {from, to, kind?, labels?, id?, view?}` | append `edge` block; if `view` given and its `edges` filter would hide the edge, add `edge.<id>` to it |
| `disconnect {id}` | remove edge, its hints and its view references |
| `set_edge_labels {id, labels}` | change labels |
| `move {view, id, x, y}` / `resize {view, id, w, h}` | update `place` (frame-relative) |
| `reorder {view, container, order}` | update `order` |
| `unpin {view, id}` | remove from `place` |
| `pin_port {view, id, port, side, offset}` | update `ports` in `place` |
| `set_edge_hint {view, id, hint}` | update `edge_hints` (also `lift:` keys) |
| `rename_id {type, old, new}` | atomic refactor of a node, edge or view ID (section 6.4) |
| `create_view` / `update_view` / `delete_view {id, ...}` | edit `views.hcl`; `delete_view` also removes its layout file |
| `add_decoration` / `update_decoration` / `delete_decoration {view, ...}` | update `decorations` |
| `promote_decoration {view, id, kind}` | create node, pin it, remove decoration |

- AC: model edits use `hclwrite`; comments and formatting elsewhere are unchanged (golden-file tests).
- AC: operations touching read-only files fail with an error naming the file.
- AC: every operation records an inverse for undo.
- AC: multi-file operations are atomic.
- AC: an operation that would add a validation error (FR-2) is refused with that error; for example `set_kind` to a non-container while the node has children, or `remove_port` while an edge uses the port.
- AC: before writing, Go checks that each file is unchanged since it was read; if not, it reloads and re-applies the operation, failing if it no longer applies. This covers `dg op` running beside `dg serve` and saves from a text editor.

**FR-13 File watching.** Watch all project files. After Go writes a file, record its content hash; a change event whose content matches is ignored.
- AC: an external save triggers reload; in-flight operations that reference missing IDs fail with a clear error.

### F. Command line

**FR-14 Export (`dg export --view <id> | --all --format svg|png|gif|json [--out path]`).** Go starts headless Chromium via `chromedp`, loads the embedded app in export mode, waits for geometry and routing, and reads back SVG, a PNG screenshot, a GIF, or JSON geometry (every node's box, port positions and edge routes in canvas coordinates). With `--all`, `--out` is a directory.
- SVG keeps edge animation as CSS. PNG shows the animation at time 0. GIF holds one half-second animation period as 10 frames of 50 milliseconds, looping forever; each frame is captured with the animation clock set to that frame's time, never the wall clock, and encoded with Go's `image/gif`.
- AC: output matches the editor (no selection, unpinned nodes not faded).
- The app writes the SVG and JSON from its computed geometry, not by copying a DOM, so the output holds no editor state.
- AC: JSON geometry lets an agent pin nodes relative to the drawn layout without opening the editor.
- AC: export works with no network: Chromium resolves no host name, and the app's content security policy allows only its embedded files.

**FR-15 Operations (`dg op <operation> [flags] [--json]`).** Direct mapping of FR-12.
- AC: an agent can build a full model and views using only `dg op`, `dg check`, and `dg export`.

### G. Integrations

**FR-16 MCP server (`dg mcp [-C dir]`).** A Model Context Protocol server over stdio with four tools: `check` (FR-2 diagnostics), `vocabulary` (the kinds, edge kinds, ports, properties and tags the loaded packs and model define), `op` (any FR-12 operation, including `batch`), and `export` (FR-14; PNG is returned as image content so the agent sees the drawing).
- AC: every tool gives the same result as the matching `dg` command.

**FR-17 Language server (`dg lsp`).** Language Server Protocol over stdio for model files and `views.hcl`, working on unsaved editor buffers:
- diagnostics from FR-2, with the same positions as `dg check`;
- completion of kinds, edge kinds, node IDs in references, ports after `.port.`, rows after `.row.`, selectors, style labels, and style property names and values;
- hover showing a referenced node's kind and label; go to definition and find references for node, edge, kind and view IDs;
- rename through `rename_id`: the server returns the files the operation would write, including layout files, as one workspace edit, and refuses as the operation does for read-only files;
- formatting through `dg fmt`.
- AC: diagnostics match `dg check` on the same files.

**FR-18 VS Code extension.** Bundles the `dg` binary (one VSIX per platform) and activates in a folder that contains `dg.hcl`. It runs `dg lsp` for the project's HCL files and highlights them with the HCL TextMate grammar. The command "Open diagram" opens a panel beside the text editor showing the embedded app from `dg serve` on a loopback port, so the panel has every editor feature that the browser has.
- AC: saving a model file updates the panel within 300 milliseconds (FR-10).
- AC: no network access at runtime (section 11).

---

## 11. Non-functional requirements

- **Deterministic:** the same files produce byte-identical layout writes and export output.
- **Diff-friendly:** sorted keys, integer grid coordinates, relative frames so moving a container is a one-line change.
- **Performance:** 60 frames per second dragging at 200 nodes / 400 edges; `dg check` under 1 second at 1,000 nodes.
- **Offline:** no network at runtime; fonts and WebAssembly bundled.
- **Single binary:** the web app is embedded in the Go binary.

---

## 12. Repository layout

```
/cmd/dg/                  # serve, check, fmt, export, op, mcp, lsp
/internal/project/        # dg.hcl, packs, file discovery, read-only rules
/internal/spec/           # kinds, edge kinds, inheritance
/internal/style/          # style blocks, shapes, cascade (section 8.4)
/internal/model/          # nodes, edges, ports, rows, validation
/internal/diag/           # diagnostics as dg check, dg serve and the language server report them
/internal/view/           # views.hcl, selectors, collapse and lifting, layout YAML, resolve (FR-4)
/internal/ops/            # operation API, hclwrite edits, undo log, atomic writes
/internal/server/         # HTTP, WebSocket, fsnotify
/internal/export/         # chromedp, GIF encoding
/internal/mcp/            # MCP tools (FR-16)
/internal/lsp/            # language server (FR-17)
/packs/generic/           # built-in pack (embedded)
/web/                     # embed.go embeds web/dist, the Vite build output (not committed; `make web` builds it)
  /src/geometry/          # sizes, container policies, frames
  /src/place/             # elkjs first layout, spiral placer
  /src/route/             # ports, hints, route clipping and labels
  /src/canvas/            # JointJS shapes and interactions, the router-avoid adapter, text measurement, SVG and JSON output
  /src/protocol/          # WebSocket client
/vscode/                  # VS Code extension (FR-18)
/testdata/                # golden files: hclwrite round-trips, layout output, resolved views
/examples/                # example projects, one per diagram type (section 13)
```

---

## 13. Milestones

The order follows two rules. First, export comes before editing: once `dg export` works, every later milestone is verified by files a test can compare, without anyone looking at a screen. Second, the headless surfaces (operations, `dg op`, MCP, LSP) come before the interactive editor, because they share the operation API and are cheaper to test.

**How a milestone is verified.** Rendering is tested through `dg export` in Go tests that drive headless Chromium: SVG and JSON geometry are golden files under `testdata/`, and every export test runs twice and requires byte-identical output (section 11). PNG and GIF are compared run against run on the same machine, not against goldens, because rasterization differs between Chromium versions. Each milestone ends with example projects in `/examples` for the diagram types it enables; their exports are goldens too.

**Language ahead of drawing.** From M2, `dg check` accepts the whole language of sections 4–6. `dg export` refuses, with an error at the source position that names the milestone, anything the renderer does not draw yet (a style value, rows, edges ending at ports or rows, collapse, pinned positions, …), so nothing is silently drawn wrong.

1. **M1 — Walking skeleton.** The thinnest path from file to picture: `node` and `edge` blocks with `kind`, `label` and `parent`; `kind`, `edge_kind`, `style` and `edge_style` with `shape` (`rect`, `rounded`), `fill` and `stroke`; resolve; the embedded web app with the bundled font, elkjs first layout and router-avoid routing; `dg export --format svg|json`.
   *Done when:* `examples/hello` exports a golden SVG, and export succeeds with the network blocked (font, WebAssembly and the elk worker are all embedded).
   *Why first:* the risk is in the joins (libavoid WebAssembly and the elk Web Worker inside the embedded app inside headless Chromium), not in any single part.
2. **M2 — Language core.** FR-1, FR-2, FR-3: kind inheritance, properties, ports, `port_range`, `file`, rows, quoted IDs, packs, project-local spec and style, read-only files; `views.hcl` and layout files parsed and validated, and views resolved by `include`, `exclude` and `edges`.
   *Done when:* every FR-2 error and warning has a golden diagnostic with file, line and column; HCL loading and layout YAML are fuzzed; the `dg check` benchmark at 1,000 nodes is under 1 second; `dg fmt` output is golden.
3. **M3 — C4 diagrams.** FR-4 (without collapse and overlays), FR-5, FR-7, FR-8, FR-9 for `free` containers; the full `generic` pack, every built-in shape and custom shapes; the node and edge style properties other than the container policies, markers and animation; the project `font`; label wrapping; pinned positions from layout files; `dg serve` read-only (FR-10 `snapshot` and `diagnostics`) with FR-13 reload, so editing HCL in a text editor updates the browser.
   *Done when:* `examples/c4-container` (people, a system boundary holding its containers, external systems, a local `c4` pack) matches its golden SVG with nothing pinned and with a partial layout file (pinned plus spiral-placed nodes); saving a model file updates the browser within 300 milliseconds; a broken file keeps the last snapshot.
4. **M4 — Entity-relationship and class diagrams.** Rows and compartments drawn; rows as ports (section 5.4); every arrow marker including crow's feet; start and end labels; self-loops; several edges between one pair.
   *Done when:* `examples/er` and `examples/class` match their goldens, with each row edge attached to the end of the row facing the other endpoint.
5. **M5 — Animated edges.** `animate = "flow"` (section 4.3) drawn in the browser and in SVG export; `dg export --format gif` (FR-14).
   *Done when:* the animated SVG is golden; the GIF is byte-identical across runs and loops without a jump; `examples/c4-container` has an animated variant.
6. **M6 — Operations and agent surface.** FR-12 except view, decoration and `reorder` operations; `batch`; inverses; read-only enforcement; `rename_id`; FR-15 `dg op`; FR-16 MCP server.
   *Done when:* every operation has a golden `hclwrite` round-trip; applying an operation then its inverse restores every file byte for byte; a refused operation leaves every file untouched; a file changed between read and write is reloaded and the operation re-applied; `dg op` arguments are fuzzed; a script rebuilds `examples/c4-container` from an empty directory using only `dg op` and matches the M3 golden; an in-process MCP client builds the same diagram through the MCP tools.
7. **M7 — Views.** Section 8 in full: collapse, edge lifting and merging, drill-down badges, breadcrumbs and view switcher, overlays, view style rules, decorations (including attached); the view and decoration operations, including `promote_decoration`.
   *Done when:* every lifting and merging case in section 8.3 has a golden resolved view; `examples/c4` shows three levels (system context with collapsed systems, a container view with `of` on a system, a component view with `of` on a container) navigable by drill-down; an incident example draws an overlay.
8. **M8 — Container policies.** `stack` and `grid`, `order`, the `reorder` operation.
   *Done when:* `examples/swimlane`, `examples/flowchart` and `examples/state-machine` match their goldens; children missing from `order` are appended in sorted ID order; `x`/`y` on a `stack` child is ignored with a warning.
9. **M9 — Language server and VS Code extension.** FR-17 and FR-18, with the diagram panel read-only.
   *Done when:* language server tests replay recorded JSON-RPC sessions (completion at given positions, rename edits, diagnostics identical to `dg check`); an extension test opens `examples/c4` in VS Code, receives diagnostics and opens the diagram panel; a platform-specific VSIX installs and runs offline.
10. **M10 — Layout editing.** Move (multi-select, first-drag pin-all), resize, unpin, align and distribute, edge hints, pinned ports, reorder by drag; frames and stale-frame handling; dragging rules (section 7.6, without re-parent); undo and redo (FR-10). Works in the browser and in the VS Code panel.
    *Done when:* a scripted drag in headless Chromium at 200 nodes and 400 edges holds 60 frames per second in a performance trace; each gesture sends one operation on drop and is one undo step.
11. **M11 — Model editing.** The rest of FR-11: palette, add existing node, connect, labels, inspector, copy and paste, hide and delete, ports, rows, re-parent modifier, read-only lock.
    *Done when:* each FR-11 interaction has a scripted browser test asserting the files written; the VS Code panel edits the same files the open text editors show.

---

## 14. Future work

- **Auto-layout view mode:** `view "deps" { layout = "layered" }` or `"tree"`; the engine positions everything and pinning is disabled. Covers mind maps and dependency trees.
- **Derived-layout view types:** `type = "sequence"`, `"timeline"`, where position comes from model order or values.
- Routing in Go (libavoid WebAssembly in `wazero`, or a native router) so export works without Chromium.
- Hyperedge routing for junctions.
- Duplicate node instances in one view (`db#2`).
- CUE for the spec layer.
- Remote packs and importing another repository's model as a read-only module.

---

## 15. Decision log

| Decision | Chosen | Rejected and why |
|---|---|---|
| Languages | Go + TypeScript | All TypeScript: no comment-preserving HCL writer in JavaScript. All Go: would mean rebuilding editor interactions JointJS already provides. |
| Model language | HCL | D2: path-based IDs, style and position mixed into the model. YAML: weaker ID enforcement, no typed references. CUE: weaker round-trip editing (possible later for spec). KDL: immature Go round-trip tooling. |
| Canvas | JointJS + router-avoid | tldraw: production license key required. React Flow: no obstacle-avoiding routing. maxGraph: fallback option, weaker routing. Sprotty: heavier than needed. |
| Spec separate from model | yes, with optional project-local `spec.hcl` | Kinds inside model files: no reuse, palette and validation need a shared vocabulary. |
| Views reference the model | by ID; layout files by convention | Model listing its views: breaks reuse; file moves become model edits. |
| View definition vs layout | `views.hcl` (selection) + `*.layout.yaml` (positions) | Selection inside YAML: loses reference validation. |
| Coordinates | relative to the frame's content origin, with explicit `in:` | Absolute: moving a container rewrites every descendant, a noisy diff. Relative to the container's top-left corner: a longer label or more padding pushes children under the label. |
| Routing obstacles | leaf nodes and collapsed containers | Expanded containers too: an edge between two swimlanes that are not adjacent has no route. |
| First drag in a view with nothing pinned | pins every node where drawn | Pin only the dragged node: every other node jumps from the elk layout to the spiral placer. |
| Re-parenting by drag | only with a modifier key | Implicit re-parent on drop: layout gestures silently changing the model. |
| Persistent alignment constraints | no; one-shot align/distribute and container policies | Constraint solver (Penrose-style): unpredictable edits, large effort. |
| Overlapping groupings | view overlays | Multiple parents: breaks the containment tree. |
| Per-node colors | tags + style rules; view-level `node.<id>` rules | Style attributes in the model: violates rule 3. |
| Text measurement | browser only; export via headless Chromium | Measuring in both Go and the browser: results drift. |
| Routes | never stored; hints only | Stored waypoints (draw.io): stale whenever a node moves. |
| Diagrams where position is data | out of v1 | Forcing them into free placement fits poorly. |
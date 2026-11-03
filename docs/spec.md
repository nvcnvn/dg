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

- A node **listed** in its view's layout file with a position or size is **pinned**: drawn where stored. An entry holding only a container's `order` pins nothing; a child of a `stack` or `grid` container is placed by its container and is drawn pinned when the container is, and a child of the `of` of a view whose policy places it on the canvas (section 7.3) when the view has pinned nodes.
- A node **not listed** is **unpinned**: placed by the engine at render time, drawn faded in the editor when the view has pinned nodes. In a view with nothing pinned nothing is faded; the editor says instead that the layout is not saved.
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
| MCP server | `github.com/modelcontextprotocol/go-sdk` (Apache-2.0 for new code, MIT for code not yet relicensed) |
| Project lock on Windows | `golang.org/x/sys/windows` (BSD-3-Clause) |
| Embedding the web app in the binary | standard library `embed` |
| Language Server Protocol | none: `internal/lsp` frames JSON-RPC and declares the few protocol types it uses with the standard library |

**TypeScript (browser)**

| Purpose | Library |
|---|---|
| Canvas and interaction | `@joint/core` (JointJS 4.3 or newer, MPL-2.0) |
| Obstacle-avoiding routing | libavoid compiled to WebAssembly by `libavoid-js` (LGPL-2.1-or-later; `libavoid.wasm` stays a separate file), called directly |
| First layout of a view with nothing pinned | `elkjs` (Eclipse Layout Kernel, EPL-2.0), in a Web Worker |
| Bundled font | Inter (OFL-1.1) from `@fontsource/inter` |
| Build | Vite, with `tsc` for type checking |
| Unit tests | Vitest |

**TypeScript (VS Code extension)**

| Purpose | Library |
|---|---|
| Language client | `vscode-languageclient` (MIT) |
| HCL highlighting | HCL TextMate grammar from `github.com/hashicorp/syntax` (MPL-2.0), vendored unmodified with its license in `vscode/syntaxes` |
| Type checking and compiling | `tsc` from TypeScript 7 (Apache-2.0) |
| Packaging, one VSIX per platform (build time only) | `@vscode/vsce` (MIT) |
| Extension tests in VS Code (test time only) | `@vscode/test-electron` (MIT), which may download a VS Code build for a test, never at runtime |

### 3.4 Fonts

One bundled font, Inter at weights 400 and 700 with all their Unicode subsets, loaded by the browser renderer. A `font_weight` up to 500 is drawn with weight 400, 600 and above with 700. System fonts are never used, with one known exception: a character that the font does not cover (CJK, most emoji) is measured and drawn with the browser's fallback font. Only the browser measures text. An exported SVG names the font (`Inter, sans-serif`) but does not embed it.

A project `font` (section 4.1) replaces Inter for every weight; the app loads it from the file's bytes, which the resolved view carries. An exported SVG names it by the file's base name without extension (`fonts/Brand.woff2` is `Brand, sans-serif`).

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
- `font` is checked to exist and is the font of every drawn text (section 3.4).

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

**Node style properties:** `shape`, `fill`, `stroke`, `stroke_width`, `stroke_dash`, `corner_radius`, `opacity`, `text_color`, `font_size`, `font_weight`, `icon`, `image`, `padding`, `min_w`, `min_h`, `max_w`, `z`. `icon` and `image` (the picture for the `image` shape) take `file("…")`, an SVG resolved relative to the file that calls it; other picture formats are not supported. A picture (also a custom shape's `svg`) must be well-formed XML whose root element is `svg` in the SVG namespace, which a browser needs to draw it; otherwise `dg check` reports an error at the `file` call. The app draws pictures from `data:` URIs of that text.
**Container properties:** `layout`, `direction` (`horizontal` | `vertical`), `gap`, `stretch`, `columns`, `align` (`start` | `center` | `end`) (section 7.3), `label_side` (`top` | `left`). `left` draws the label and icon a quarter turn counterclockwise, centered along the left edge, as on a swimlane: the label sits half a padding from the edge, a line in the stroke color runs the full height half a padding past it, and the content origin is a full padding past the label; the container is at least as tall as the label is long plus a padding above and below.
**Overlay style properties:** `fill`, `stroke`, `stroke_width`, `stroke_dash`, `opacity`, `padding`. An overlay is styled by `overlay_style "hull"` with the properties of the overlay style it names set over it, so `hull` holds every default.
**Values:** colors are `#RGB`, `#RRGGBB` or `none`; sizes, widths, gaps and padding are numbers of at least 0, `font_size` at least 1; `opacity` is 0–1; `font_weight` is 100–900 in steps of 100; `z` and `columns` are whole numbers; `stroke_dash` is a list of numbers.
**Custom shapes:** a shape's name may not be a built-in shape. Each of its `anchors` must name a port of every kind whose style (by kind, section 8.4 step 1–2) uses the shape; ports without an anchor are placed by section 7.5. `text_box` defaults to `[0, 0, 1, 1]` and `stretch` to `nine-slice`. The picture's natural size is its `viewBox`, else its `width` and `height`, else 100 by 100. `nine-slice` cuts the picture a quarter of its width and height in from each edge: corners keep their natural size (scaled down only when the box is smaller than half the picture), edges stretch along their side, the center both ways. `uniform` scales the picture to fit and keeps the box at its aspect ratio; `none` draws it at its natural size, centered, in a box at least that large. A custom shape's `fill` and `stroke` are not drawn; the picture has its own.

**Drawing:** labels break at `\n` and wrap at `max_w` minus twice `padding` (`max_w = 0`: no wrapping), by words, a word wider than the line broken between characters; lines are `ceil(1.4 × font_size)` apart. An `icon` is drawn above the label, `2 × font_size` square. The `image` shape draws its picture above the label, at least `6 × font_size` square, and no outline; `text` draws no outline. `point` and `bullseye` draw no label and are sized by `min_w` and `min_h`. `corner_radius` applies to `rounded`. `table` draws rows as section 5.4 says, and a node without rows as `rect`. Expanded containers are drawn first, then the other nodes, each in order of `z`, then depth in the containment tree, then ID; overlays are drawn between the two (section 5.8), and edges over nodes. So `z` orders containers among containers and other nodes among other nodes: no `z` raises a container above a leaf.
**Edge style properties:** `stroke`, `stroke_width`, `stroke_dash`, `arrow_start`, `arrow_end`, `routing`, `font_size`, `animate`.
**Animation** (`animate`): `none` (default) or `flow`. `flow` draws the edge dashed (its `stroke_dash`, or `[6, 4]` when it has no value above 0) and moves the dashes along the route from `from` toward `to` by one dash period every half second, so every animated edge repeats with the same half-second period. The period is the pattern's length: the sum of its values, twice that for an odd number of values, which SVG repeats. At time `t` seconds the dash offset is `−period × 2t` (CSS `stroke-dashoffset` animated linearly from 0 to minus the period over 500 ms, repeating), so time 0 is the edge drawn still and the drawing at half a second equals it. Markers are never dashed. Animation is style, so it is selected by kind or tag (`edge_style "tag.hot" { animate = "flow" }`), never set in the model.
**Arrow markers** (`arrow_start`, `arrow_end`): `none`, `arrow` (open), `triangle`, `triangle-open`, `diamond`, `diamond-open`, `circle`, `circle-open`, and crow's-foot `crow-one`, `crow-zero-one`, `crow-many`, `crow-one-many`, `crow-zero-many`. A marker lies along the last segment of the route at its end, in the edge's `stroke` and `stroke_width`, never dashed; `-open` markers are filled white, the others with the stroke color. Crow's-foot markers read outward from the node: `crow-one` two bars, `crow-zero-one` a bar then a ring, `crow-many` a foot spread across the end, `crow-one-many` a foot then a bar, `crow-zero-many` a foot then a ring.

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
  title     = "RDMA fabric"
  include   = [kind.gpu-node, kind.switch, kind.storage]
  edges     = [edge_kind.rdma]
  direction = "right"                    # layers of the first layout run left to right (section 7.4)
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

`direction` (`down`, `right`, `up` or `left`) sets the way the layers of the view's first layout run (section 7.4).

View style rules (`style` and `edge_style` blocks in a view) take the labels of section 4.3, `node.<id>` and `edge.<id>` included, and apply only in that view (section 8.4).

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
  ingest:   {h: 200}                         # a stack child: size only
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
- `order` lists the children of a `stack` or `grid` container in the order it places them (section 7.3). An entry with only `order` does not pin its node.
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

A row is drawn as a line inside the box **and acts as a port** whose attachment point is the left or right end of that line, whichever faces the other endpoint: the side the other endpoint's box (a port's point) lies on, or, when the two boxes overlap horizontally, the side where their edges are closer, right on a tie. So both ends of an edge between two rows of tables stacked above each other, or of the same table, take the same side and the route loops around. Ends at one row end with the same marker share its point; ends with different markers are spread 16 pixels apart along the side, centered on the row's line and ordered by their other ends. Every row end runs straight until a grid unit past the longest marker at its side of the table, so no line turns among another row's markers. A row of a node that is not drawn as a table (its shape is not `table`, or it is a container) has no line, so an edge ending there ends at the node itself. Reference: `node.orders.row.customer_id`, or `node.user.row["save()"]`.

**Drawing.** A node with rows whose shape is `table` is drawn as a rectangle holding its label (wrapped as usual, centered, a padding above and below) and then each compartment below a full-width separator line. A compartment holds one line per row, `ceil(1.4 × font_size) + padding / 2` tall, with half a padding above its first row and below its last; a compartment without rows is a band one padding tall. A row's line shows its `key` in bold at the left, its name after the widest key (one `font_size` apart), and its `type` aligned to the right edge, all at the node's `font_size` and `text_color`; columns without any value take no room. The box is as wide as the label or the widest row, whichever needs more, and is sized and pinned like any other node (section 7.2); a table draws no `icon`. The `generic` pack draws the label of `table` and `class` bold.

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
- Fields: `text` takes `text`; `rect`, `ellipse` and `band` take `w` and `h`; `image` takes `src` (a project-relative path to an SVG file, which must be a picture as for `icon`, section 4.3) and `w`, `h`; `arrow` takes `points` (at least two `[x, y]`); `freehand` takes `path` (SVG path data: commands and numbers). Each of these is required by its type and refused on the others.
- **Position:** `x`, `y` (default 0) relative to the canvas, or to the content origin of the node named by `in:` (a container's, else its box's top left corner). With `attach: <node-id>` the position is the node's center moved by `dx`, `dy` instead, so the decoration follows the node; an attached decoration takes neither `in` nor `x`, `y`, and `dx`, `dy` need `attach`.
- **Geometry:** a box (`rect`, `ellipse`, `band`, `image`) or a text block has its top left corner at the position, or, attached, is centered on it. An arrow's points and a freehand path are relative to the position. Text breaks at `\n` only.
- A decoration may set `fill`, `stroke`, `stroke_width`, `stroke_dash`, `opacity`, `text_color` and `font_size` inline. Defaults: `text` color `#202124`, `font_size` 14; `rect`, `ellipse`, `arrow` and `freehand` stroke `#5F6368`, width 1.5, no fill; `band` fill `#F1F3F4`, no stroke; `opacity` 1. An arrow has an `arrow` marker at its last point.
- `z`: draw order relative to nodes: a negative `z` is drawn behind every node, any other over every node and edge, each group in order of `z`, then layout file order. Default `-1` for `band`, `1` otherwise.
- A decoration is not drawn while its `attach` or `in` node is not drawn.
- **Promote to model** (`promote_decoration`): converts a `text`, `rect`, `ellipse`, `band` or `image` decoration into a model node pinned at its position (and its `w`, `h`), labeled by the text of a `text` decoration; a decoration `in:` a container becomes a child of it. Attached decorations, arrows and freehand lines have no position a node can take, and a decoration `in:` a node that is not a container, or a container the view does not draw expanded, has no frame the node can take; these are refused.

### 5.8 Overlay (view only)

A named outline drawn around the boxes of its members, computed at render time from selectors. Used for groupings that overlap or cut across containment. Never stored as geometry.
- The outline is the convex hull of the member boxes grown by the style's `padding`, with round corners of that radius; its label is drawn above the left end of its top edge, at 12 pixels in the stroke color.
- Overlays are drawn over expanded containers and under every other node, so an overlay's fill shows inside the containers it crosses.
- Members hidden by a collapse are represented by their collapsed container; an overlay with no drawn member is not drawn (section 8.3).

---

## 6. Identifiers

### 6.1 Format

- **Simple IDs:** match `^[A-Za-z_][A-Za-z0-9_-]*$`; referenced as `node.api`.
- **Quoted IDs:** any other non-empty string up to 200 characters without control characters; referenced with index syntax `node["kube-system/coredns"]`. Intended for generated models.
- Reserved, cannot be simple IDs: `all`, `node`, `kind`, `tag`, `children`, `descendants`, `edge_kind`, `port`, `row`, `lift`. An edge ID may not start with `lift:`, which names lifted edges (section 8.3).
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

Edge IDs: `slug(fromNode) + "-" + slug(toNode)` (node IDs only, without port or row), slugged again and prefixed alike, with the same collision suffix. Generated IDs are always simple.

`create_node` and `connect` (FR-12) take an optional `id`: given, it is used as it is (simple or quoted, section 6.1) and the operation is refused if the ID is taken or invalid; omitted, it is generated as above. An agent that builds a model names its IDs; the editor generates them.

### 6.3 Freezing

- An ID is frozen once created.
- One exception: a node created with an ID derived from its kind (no label and no `id` given) is re-slugged **once**, from the first label entered in the same editing session, with every reference renamed as by `rename_id`. A session is one `dg op` command or MCP `op` call (so the rule applies within a `batch`), or the lifetime of `dg serve`.
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
- Stored coordinates are relative to the frame's **content origin**: for a container, the top-left of its inside, past its label and padding (the label sits half a padding below the top, or from the left edge with `label_side = "left"`, and a full padding separates it from the content); for the canvas, (0, 0). `in:` names the frame; omitted means the canvas. A negative coordinate inside a container is drawn as 0.
- Moving a container therefore changes one layout line; its descendants move with it. Changing a container's label or padding moves its content with it instead of under the label.
- **Stale frame:** if the stored `in:` of an entry with `x` or `y` differs from the node's current frame (the model's `parent` changed, or the view's `include` or `collapse` changed):
  - if the stored frame is still drawn in this view, the browser converts the coordinates to the current frame for display: it places the view with the node unpinned, takes the absolute position the stored coordinates give in the stored frame, and places the view again with the node pinned there in its current frame. The entry keeps its stored values until the node is next moved or unpinned (Go has no geometry to convert them);
  - otherwise, or when the stored frame is the node itself or inside it, the node is treated as unpinned.
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

- The container places its children in the order of its `order` list in the view's layout file; children missing from `order` are appended in sorted ID order, and entries naming no child it draws are skipped.
- In `stack` and `grid` containers, dragging a child **reorders** it; children never get `x`/`y`. Stored `x`/`y` on such a child is ignored with a warning; its `w`, `h` and pinned ports apply.
- Positions are relative to the container's content origin. A `stack` puts its first child at the origin and each next one `gap` past the one before along `direction`. Across the stack its children have the room of the widest child (of the tallest, for `horizontal`) or the container's own inside, whichever is more, or exactly the container's inside when its size is pinned that way: `stretch` sets each child's cross-axis size to that, otherwise each is aligned in it by `align`. A child's size along the main axis is its own, pinned or measured.
- In `grid`, children fill `columns` columns row by row; a column is as wide as its widest child and a row as tall as its tallest, cells `gap` apart both ways, and each child is aligned in its cell both ways by `align`.
- The container is then sized by section 7.2 around its children. A child with a policy that a `stretch` resizes places its own children again in its new size.
- In a stretched `stack`, the container children whose labels run along the stack's cross axis (`label_side = "left"` in a `vertical` stack, `top` in a `horizontal` one) all take the widest of their label strips, so their content origins line up as lanes; the label is centered in the strip.
- A child of a `stack` container can itself be a `free` container (a swimlane holding freely placed steps).
- In a view whose `of` is a `stack` or `grid` container it does not draw, that container's policy places its children the view draws, with its `order` from the view's layout file, as one undrawn box on the canvas at the origin of the box; other nodes on the canvas (included from elsewhere, or descendants whose parent the view leaves out) are placed around that box as around any node (section 7.4), and the box as a node that is never pinned.

### 7.4 Placement of unpinned nodes

- If a view has **no** pinned nodes at all: run `elkjs` (layered algorithm) once, with nodes and edges given in sorted ID order. The result is not saved.
  - `stack`/`grid` containers are laid out by their policy first, innermost first, and passed as fixed-size boxes; an edge end inside one is given at that box.
  - The children of a `free` container that a `stack` places (lanes) are laid out first by elk too: those of all the lanes of the stack together, in one run whose layers run across the stack (right for a `vertical` stack, unless the view's direction is `left`; down for a `horizontal` one, unless it is `up`), so steps line up across lanes in the order edges run through them. Across the stack the steps of every lane then start at the lane's content origin together. Along it each lane's steps fill rows from the origin in elk's order, each in the first row where it keeps elk's node spacing from the others across the stack, centered in its row on the grid, rows that spacing apart.
  - The children of a `free` container a `grid` places are laid out by elk alone, moved to start at its content origin.
  - Each edge carries the sizes of its start, middle and end labels, so the layers leave room for them, and self-loops of leaf nodes are passed too.
  - Layers run the way the view's `direction` says (section 4.5); without one, top to bottom, except in a view where some edge ends at a row: rows attach at their left and right ends, so its layers run left to right. Layers of a view with an edge at a row are further apart, to hold the markers at both ends of an edge.
  - An edge with `triangle-open` at one end only (inheritance, realization) is given to elk from that end, so the general element is in an earlier layer than the specific ones (above them when the layers run down).
  - dg breaks cycles before elk does: a depth-first search from the nodes no edge enters, in ID order, following edges in ID order, gives every edge back to a node it is still inside to elk reversed, so a flow runs from its start and a loop back to an earlier step stays a loop back. Cycles no such node leads to are left to elk's own cycle breaking.
- Otherwise the spiral placer fills frames innermost first, so a container's size is known before it is placed. Within a frame, for each unpinned node in **sorted ID order**: start from the centroid of its already-placed neighbors in that frame (a neighbor in another frame counts as its ancestor in this frame, if any; fallback: the center of a container's pinned content area, else of what is already placed in the frame, else the frame's content origin), then search outward in a spiral on the grid until it keeps four grid units of clearance from everything placed in its frame, the room an edge and its label need between two boxes. Inside a container coordinates stay at least 0.
- In a view with nothing pinned, the editor sends the first drag as one batch (FR-12) that pins every visible node where it is drawn, except the children of `stack` and `grid` containers, plus the drag itself. Otherwise pinning one node would hand every other node from the elk layout to the spiral placer and the picture would jump.
- `dg op` has no geometry: a `move` from the command line pins only the node it names. Agents read drawn positions with `dg export --format json`, which gives every node's frame and its drawn position relative to that frame's content origin (`frame`, `rel`), the coordinates `move` takes. An agent that wants the elk layout kept pins every node at its `rel` in one batch, as the editor's first drag does, except the nodes the export marks `ordered_in` a container, which `move` refuses.
- Placement is deterministic. Pinned nodes never move.
- In a view with pinned nodes, unpinned nodes are drawn faded in the editor; in a view with nothing pinned no node is faded and the editor shows a hint that the layout is not saved. Export draws every node normally.

### 7.5 Ports and routing

Port position, in priority order:
1. the custom shape's anchor with the same name;
2. a pinned position in the layout (`ports: {p12: {side, offset}}`);
3. the declared `side`, with ports on the same side spread evenly in declaration order;
4. `auto`: the side facing the other endpoint's center.

Routing:
- libavoid, orthogonal by default (`routing` in edge style: `polyline` lets libavoid route with diagonal segments; `straight` is one line between the ends, through anything).
- Leaf nodes, collapsed containers and the labels of expanded containers are obstacles, except an edge's own endpoints. Expanded containers are not: edges cross container borders, so an edge between two swimlanes can pass through the lanes between them.
- An `auto` port faces the other end of the first edge, by ID, that uses it; a port with side `auto` that no drawn edge uses has no position.
- An edge end at a leaf node without a port or side hint is routed in two passes: the first from the node's center finds the side the edge enters by; the edges entering one side are then spread evenly along it, ordered by where their other ends lie, and routed again from those points. An edge whose two spread ends face each other less than two grid units apart is straightened. Ends are moved onto the outline of non-rectangular shapes.
- An end with a direction (a port, a row, a side hint, a spread end, an end at a container) leaves its side at a right angle: it is routed from a point just outside the router's clearance in that direction, joined to the end by a straight segment, which is longer at an end whose marker is longer than `arrow`, so the marker lies on one straight segment with some line beyond it. This also holds for `polyline` edges. When the router's nudging leaves such a segment of an orthogonal route at an end with a marker or a start or end label shorter than that (for a label, a line of its text and a few pixels), the next segment is moved out to make it that long, and further while it would lie on another edge, unless that would cross a leaf node.
- An inner segment of an orthogonal route that runs along a container's border, closer than one grid unit, is moved one grid unit away from it, unless that would cross a leaf node, so an edge is never drawn on top of a border.
- An edge end at an expanded container without a port starts on the container's side that faces the other end, or, when the other end is inside the container, on its nearest side, at the point nearest the other end.
- Hints: `from_side` / `to_side` attach the end at the middle of that side; `via` waypoints, in canvas coordinates, are passed in order; `label_pos` (0–1 along the path) places the middle label. Without `label_pos` the middle label goes to the first of the points at 0.5, 0.4, 0.6, 0.3, 0.7, 0.2 and 0.8 of the path, first on the path and then beside it (beside it first when the path is too short to hold the label), where it overlaps no leaf node, container label, marker or earlier label and crosses no other edge; if there is none, where it overlaps least.
- Start and end labels are placed before middle labels, each next to its end: beside the route's last segment, clear of the widest marker, starting a few pixels outside the node and extending away from it; on one side of the segment, else the other, then both sides moved one and two labels further out, taking the first that overlaps nothing above, crosses no edge and has no other edge within a few pixels (where it would read as that edge's label), else the one that overlaps least.
- A self-loop whose ends are the node itself (no port, row, side hint or `via`) is not routed: it leaves a vertical side and enters a horizontal one around one corner of the node, two grid units out (more for long markers). Each takes the corner, of those its node's other loops have not taken, where it and the room its end labels need overlap the least other nodes and edges, clockwise from the top right on ties; a fifth loop starts again one reach further out. Where edges are spread along a side a loop's end is on, the loop's end takes the slot nearest its corner among them. Other self-loops are routed like any edge.
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

`of = node.X` declares that a view shows the inside of container `X`. When `X` is a `stack` or `grid` container the view does not draw, its policy places its children there (section 7.3).
- In every other view, `X` shows a drill-down badge (a magnifying glass in the top right corner of its content area, or of its box when it is expanded); clicking it opens the view. If several views have `of = node.X`, the editor shows a menu of them. The badge is a control of the editor: exports do not draw it (the collapse badge, which says the box holds more, they do).
- The editor shows breadcrumbs for a view with `of = node.X`: the levels of views it leads back to, outermost first, then the view itself. The level above a view with `of = node.Y` is the views with `of` on the nearest ancestor of `Y` that some view targets, only those that draw `Y` when some do; when no ancestor of `Y` is targeted, it is the views without `of` that draw `Y`, collapsed or with its badge, and nothing is above that level. A level of one view is a link to it; a level of several is one crumb that opens a menu of them.

### 8.3 Collapse and edge lifting

`collapse = [...]` draws each selected container as a single box with a "+" badge under its label and hides its descendants, including collapsed ones. A collapsed container is a leaf for sizing, placement and routing (sections 7.2, 7.5); the badge is part of its measured content.
- The `edges` filter selects model edges before lifting.
- An edge endpoint hidden inside a collapsed container is **lifted** to the outermost collapsed container that holds it (ports and rows are dropped); when that container is not visible either, the edge is not drawn.
- An edge whose two drawn endpoints are the same collapsed container is hidden.
- Lifted edges with the same drawn `from` and `to` are merged into one. With N = 1 it keeps its labels and style; with N > 1 it is labeled `×N` (a middle label) and styled by its edges' kind if they share one, otherwise by `default`, without their tags or `edge.<id>` rules. An end that was not lifted keeps the port or row its edges share. An edge drawn between the same nodes without lifting stays a separate edge.
- A lifted edge's ID in the resolved view and its layout key is `lift:<from>:<to>` (in drawn direction, IDs encoded per section 4.6); the resolved edge lists the model edges it stands for.
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
| `person` | `person` (a head above a rounded body; the label is in the body) |
| `external` | `rounded`, dashed |
| `note` | `document` |
| `group` | container, `free` |
| `lane-set` | container, `stack` vertical, stretch, `label_side = "left"` (a pool: its lanes are stacked right of its label) |
| `lane` | container, `free`, `label_side = "left"` |
| `grid-group` | container, `grid`, 3 columns |
| `decision` | `diamond` |
| `terminal` | `stadium` |
| `state` | container, `free`, `rounded`: a state with substates |
| `simple-state` | `rounded`: a state without substates |
| `initial` | `point`, no label drawn |
| `final` | `bullseye`, no label drawn |
| `table` | `rows = true`, shape `table`, bold label |
| `class` | `rows = true`, shape `table` with compartments, bold label, pale yellow |
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
- decorations without a field their type requires, with a field it does not take, with invalid values, or whose picture cannot be read (section 5.7);
- `of` or `collapse` naming a non-container;
- layout files with no matching view definition.

Warnings:
- layout entries naming IDs that no longer exist, and `lift:` hints of lifted edges the view does not draw (orphans);
- style labels naming undefined kinds; selectors that match nothing;
- stale frames (section 7.1);
- `x`/`y` on nodes a `stack` or `grid` container places (section 7.3);
- `order` lists on containers that are not `stack` or `grid`, and entries of an `order` that name a node twice or a node that is not the container's child;
- pinned containers too small for their children (computed at render time; reported by the editor and by `dg export`).

Information: a kind, edge kind, shape or style overridden by a later pack or the project (section 4.1).

When a model file does not parse, references, selectors and layout entries are not checked: what they name may be in that file, and each would be reported again.

- AC: exit code 0 if no errors, 1 if errors, 2 on internal failure (such as an unreadable project directory); `--strict` makes warnings errors.
- AC: `--json` prints a JSON array of diagnostics, each with `severity` (`error`, `warning` or `info`), `summary`, `detail`, and, when it has a position, `file`, `line`, `column`, `end_line` and `end_column`. `dg serve` (FR-10) sends the same shape, the language server the same diagnostics in the protocol's (FR-17).
- AC: `file` is the project-relative slash path, the same whatever `-C` and the working directory are; files of the built-in pack are `<generic>/spec.hcl` and `<generic>/style.hcl`, which no project path can be. A diagnostic about one element of a list (a pattern, port, tag or selector) points at that element.
- AC: diagnostics are sorted by file, line and column.

**FR-3 Format (`dg fmt`).** Format every writable HCL file the project loads (`dg.hcl`, local packs, project `spec.hcl` and `style.hcl`, model files, `views.hcl`) with `hclwrite.Format`; regenerate layout files in canonical form. Files matched by `read_only` are never written; files that do not parse are left as they are and reported. All changed files are written together (temporary files, then rename), keeping each file's permissions and writing through symbolic links that stay inside the project, and `dg fmt` prints the path of each. If `dg.hcl` has errors, `dg fmt` writes nothing, since `read_only` may be incomplete. `dg fmt --check` lists the files that would change, writes nothing, and exits 1 if there are any.

### B. Resolution (Go)

**FR-4 Resolve a view.** Produce the resolved view: visible nodes with kind, label, merged style (section 8.4), frame, ports, rows, pin data from the layout (relative coordinates, order lists), collapse state, drill-down targets, read-only flags; visible and lifted edges with labels, style, hints; decorations with their defaults; overlays with member IDs and style; breadcrumbs and the view's `direction`.
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
- client → server: `op`, `select-view`, `undo`, `redo`. Selecting a view that is not defined is answered with `error`.
- `dg serve` listens only on a loopback address and answers only requests whose `Host` is one, so another site cannot reach it by rebinding a host name; it has no authentication.
- Undo history belongs to the `dg serve` session and is shared by its clients; `dg op` has none (git is its undo). Undo and redo restore the session's state with the files (the re-slug-once rule, section 6.3). An undo applies only while every file it restores still holds exactly what the operation wrote: after any change to such a file outside the editor, even elsewhere in it, the undo fails with an error and is dropped.
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

**FR-12 Operation API.** One API used by the editor, `dg op`, and agents. Applied by Go, which writes files and returns the result: for each operation the ID of the element it created or changed (after a rename or re-slug, the new ID), and for `move` and `resize` the values stored.

| Operation | Effect |
|---|---|
| `batch {ops}` | apply the operations in order, atomically, as one undo step; validation runs once, on the result |
| `create_node {kind, id?, label?, parent?, view?, x?, y?}` | append `node` block; if `view` given, add `node.<id>` to its `include` when its selectors would not show the node (refused if the view still does not, because of `exclude` or a collapse) and, with `x`,`y`, pin it there (relative to the new node's frame) |
| `delete_node {id, recursive?}` | remove the block, its edges, its layout entries, the decorations attached to it or placed in it, its lifted-edge hints, and its entries in view selectors and `node.<id>` rules; children move to its parent (unpinned), or are deleted too with `recursive`; refused while a view has `of` on it |
| `set_label {id, label}` | change `label` |
| `set_parent {id, parent?}` | set `parent`, or remove it when `parent` is empty or omitted; layout entries keep their frame, which may become stale (section 7.1) |
| `set_kind {id, kind, type?}`, `set_tags {id, tags, type?}` | change the attribute of a node, or of an edge with `type = edge`; an empty `tags` removes it |
| `set_props {id, props}` | replace `props` (strings, numbers, booleans); empty removes it |
| `add_port {id, name, side?}`, `remove_port {id, name}` | instance `port` block; `remove_port` also removes the port's pinned positions, and refuses a kind port or one from a `ports` attribute |
| `set_rows {id, compartments}` | replace rows/compartments; one compartment without a name holds rows declared directly in the node |
| `connect {from, to, id?, kind?, labels?, tags?, view?}` | append `edge` block; `from` and `to` are `{node, port?}` or `{node, row?}`; if `view` given and its `edges` filter would hide the edge, add `edge.<id>` to it (refused if the view does not show both ends) |
| `disconnect {id}` | remove edge, its hints and its view references |
| `set_edge_labels {id, labels?}` | replace labels: `label` for a middle label alone, else `labels`; none removes both |
| `move {view, id, x, y}` / `resize {view, id, w, h}` | update `place` (frame-relative, `in:` set to the node's current frame), values rounded to the grid; refused for a node the view does not show, `move` for a node a `stack`/`grid` container places (section 7.3), `resize` for a node not pinned unless such a container places it |
| `reorder {view, container, order?}` | set the `order` of a `stack`/`grid` container the view draws expanded, or of the view's `of` when its policy places the canvas; `order` lists children the view draws, each once; an empty or omitted `order` removes the `order` of any node, so a stale one can be cleared |
| `unpin {view, id}` | remove the node's position, size and pinned ports from `place`; a container's `order` stays |
| `pin_port {view, id, port, side, offset?}` | update `ports` in `place` of a pinned node, or of one a `stack`/`grid` container places; `offset` kept to two decimals |
| `set_edge_hint {view, id, hint}` | replace the edge's entry in `edge_hints` (`id` may be a `lift:` key); `via` rounded to the grid, `label_pos` to two decimals; an empty hint removes it; refused for an edge or lifted edge the view does not draw, unless the hint is empty |
| `rename_id {type, old, new}` | atomic refactor of a node, edge or view ID (section 6.4) |
| `create_view {id, title?, include?, exclude?, collapse?, edges?, of?, direction?, overlays?, styles?}` | append a view block to `views.hcl` (selectors written as in it, such as `node.api` or `edge_kind.uses`; `overlays` a list of `{id, members, style?, label?}`; `styles` a list of view style rules `{type?: style or edge_style, label, props}`) |
| `update_view {id, title?, include?, exclude?, collapse?, edges?, of?, direction?, overlays?, styles?, unset?}` | set the attributes given; `overlays` and `styles` replace the view's overlay blocks and style rules, keeping a block whose content is unchanged as it is; `unset` names attributes to remove (or `overlays`, `styles`); a new attribute goes before the view's nested blocks |
| `delete_view {id}` | remove the view block and its layout file |
| `add_decoration {view, id?, decoration}` | append a decoration (`decoration` holds its fields as in layout files, `type` required); the ID is generated from the type when omitted; coordinates and sizes rounded to the grid |
| `update_decoration {view, id, decoration?, unset?}` | set the fields given, remove those `unset` names |
| `delete_decoration {view, id}` | remove a decoration |
| `promote_decoration {view, id, kind, new?, label?}` | create a node (ID `new`, else generated), pin it where the decoration was, remove the decoration (section 5.7) |

Operations take JSON objects whose `op` field names the operation and whose other fields are its arguments; an unknown or missing field is an error. Coordinates and sizes lie within ±10⁹. New blocks go at the end of the file of the node they relate to (the parent, or the `from` node) when it is writable, else of the first writable model file, else of a new `model.hcl`, and are written in canonical format. An edit writes only the tokens it changes, spaced canonically and indented like their neighbors, and aligns the `=` signs and trailing comments of the run of attributes it touches as `dg fmt` does, so a canonical file stays canonical; a block on one line is first spread over several lines so it can take more; `rename_id` changes only the tokens of the names. Every other byte stays as it is, except that `hclwrite` keeps leading whitespace as a count of spaces, so a tab in an edited file becomes a space. A layout file left empty is removed.

Writers of one project are serialized: an operation, undo or redo holds a lock from reading the files to writing them, within the process and across processes (an advisory lock on the project directory, on Windows on a file in the user's cache directory named after the project's path, so no lock file is written into the project). A writer waits up to 10 seconds for another, then fails saying so. The check that files are unchanged since read still covers writers that take no lock, such as a text editor.

- AC: model edits use `hclwrite`; comments and formatting elsewhere are unchanged (golden-file tests).
- AC: operations touching read-only files fail with an error naming the file.
- AC: every operation records an inverse for undo: the content each file it wrote had before (or that it did not exist) and has after. Undo writes the contents before, redo those after, each only while every file still has the other content (FR-10).
- AC: multi-file operations are atomic.
- AC: an operation that would add a validation error (FR-2) is refused with that error, an error being added when the operation increases the number of errors with its summary (errors name IDs, which a rename changes); for example `set_kind` to a non-container while the node has children, or `remove_port` while an edge uses the port. Operations are refused while `dg.hcl` has errors or an HCL file does not parse, and an operation that would edit a layout file with errors is refused, since what does not decode would be lost.
- AC: before writing, Go checks that each file is unchanged since it was read; if not, it reloads and re-applies the operation, failing if it no longer applies. This covers `dg op` running beside `dg serve` and saves from a text editor.

**FR-13 File watching.** Watch all project files. After Go writes a file, record its content hash; a change event whose content matches is ignored.
- AC: an external save triggers reload; in-flight operations that reference missing IDs fail with a clear error.

### F. Command line

**FR-14 Export (`dg export --view <id> | --all --format svg|png|gif|json [--out path]`).** Go starts headless Chromium via `chromedp`, loads the embedded app in export mode, waits for geometry and routing, and reads back SVG, a PNG screenshot, a GIF, or JSON geometry (every node's box, port positions, the `y` of each row's line, whose ends are the box's left and right edges, and edge routes in canvas coordinates; and every node's `frame`, omitted for the canvas, and `rel`, its drawn position relative to the frame's content origin, as `move` takes it; a node a `stack` or `grid` container places, which `move` refuses, has `ordered_in`, that container's ID, as `reorder` takes it). With `--all`, `--out` is a directory that receives one `<view>.<format>` file per view, all written together, and nothing is written unless every view resolves. A picture the browser cannot draw fails the export, naming its node.
- SVG keeps edge animation as CSS, in a `<style>` element. PNG and GIF are screenshots of the drawing on white, one pixel per unit; PNG shows the animation at time 0. GIF holds one half-second animation period as 10 frames of 50 milliseconds, looping forever; each frame is captured with the animation clock set to that frame's time, never the wall clock (the app pauses every animation at that time), and encoded with Go's `image/gif`: one palette of the 255 colors most frequent across the frames, each pixel the nearest of them without dithering, and in every frame after the first the pixels that did not change are transparent. The editor runs the same animation in real time.
- AC: output matches the editor (no selection, unpinned nodes not faded, no drill-down badges).
- The app writes the SVG and JSON from its computed geometry, not by copying a DOM, so the output holds no editor state.
- AC: JSON geometry lets an agent pin nodes relative to the drawn layout without opening the editor.
- AC: export works with no network: Chromium resolves no host name, and the app's content security policy allows only its embedded files.

**FR-15 Operations (`dg op <operation> [flags] [--json]`).** Direct mapping of FR-12: each field of an operation is a flag of the same name (`--id`, `--kind`, `--view`, `--x`; lists such as `--tags`, `--include` and `--unset` are repeated; `--props`, `--compartments`, `--overlays`, `--styles` and `--decoration` take JSON), except the ends of `connect` (`--from`, `--from-port`, `--from-row`, `--to`, …), labels (`--start-label`, `--label`, `--end-label`) and the hint of `set_edge_hint` (`--from-side`, `--to-side`, `--via x,y` repeated, `--label-pos`). `dg op batch` reads a JSON list of operations (or one operation) from standard input or `--file`. `dg op --help` lists the operations and `dg op <operation> --help` its flags and JSON form. `--json` prints `{"results", "files", "warnings"}` (warnings the operation added), or `{"error", "diagnostics"}` when it is refused. Exit code 0: applied; 1: refused or invalid; 2: dg failed. `dg vocabulary` prints, as JSON, the kinds (with inherited ports, properties and the `container` and `rows` flags), edge kinds, tags and views the project defines.
- AC: an agent can build a full model and views using only `dg op`, `dg vocabulary`, `dg check`, and `dg export`.

### G. Integrations

**FR-16 MCP server (`dg mcp [-C dir]`).** A Model Context Protocol server over stdio with four tools: `check` (FR-2 diagnostics, as `dg check --json`), `vocabulary` (as `dg vocabulary`), `op` (any FR-12 operation as a JSON object, including `batch`; the result is `dg op --json`'s, an error result when refused), and `export` (FR-14, `{view?, format?}`; PNG, the default, and GIF are returned as image content so the agent sees the drawing, SVG and JSON as text).
- AC: every tool gives the same result as the matching `dg` command.

**FR-17 Language server (`dg lsp [-C dir]`).** Language Server Protocol over stdio for the HCL files of the project in `dir`, working on unsaved editor buffers: the project is loaded from its files with the open buffers in their place, again after a change to a buffer or a file the editor reports (`workspace/didChangeWatchedFiles`), once no more changes are waiting or a request needs it, so a burst of changes loads it once. Requests before `initialize` and after `shutdown` are refused as the protocol says.
- diagnostics from FR-2 for every file of the project, layout files included, with the same positions as `dg check` (converted from byte offsets to the protocol's zero-based UTF-16 positions) and the message `summary: detail`. After a change the server sends the diagnostics of each file whose diagnostics changed, an empty list for a file that has none any more. A diagnostic without a file, or about the built-in pack, which an editor cannot open, is put at the start of `dg.hcl`.
- completion, on text that does not parse too: kinds (a node's `kind`, a kind's `extends`), edge kinds (an edge's `kind`), node IDs in references (after `node.`, quoted IDs in index form), ports after `.port.`, rows after `.row.`, selectors (`all` and the forms of section 4.5, then the kinds, tags, nodes, containers, edge kinds or edges after them), style labels (kinds or edge kinds, `tag.<tag>`, and in a view `node.<id>` or `edge.<id>`), style property names, and the values of the properties that take few (choices, booleans, `none` for colors, shapes with the custom ones). While a file does not parse, the names come from the project as last loaded with every file parsed.
- hover showing a referenced node's kind and label; go to definition and find references for node, edge, kind, edge kind and view IDs: block labels, references and selectors, `kind` and `extends` values, style labels, and the `place` and `edge_hints` keys of layout files; a view's layout file is a reference to it. The kinds of the built-in pack have no definition to go to.
- rename of node, edge and view IDs through `rename_id`: the operation is planned on the buffers over the files and writes nothing; the server returns the files it would write, including layout files, as one workspace edit, a layout file renamed with a view as a file rename, and edits only where the text changes. It refuses as the operation does, naming a read-only file, and refuses to start renaming an ID defined in one.
- formatting through `dg fmt`'s code of a writable file the project loads: an HCL file with `hclwrite.Format`, a layout file regenerated; nothing for a read-only file or one that does not parse, and refused while `dg.hcl` has errors, as `dg fmt` refuses.
- AC: diagnostics match `dg check` on the same files.

**FR-18 VS Code extension (`vscode/`).** Activates in a workspace with a folder that holds `dg.hcl`, and runs its bundled `dg lsp -C <folder>` for the `.hcl` files of each such folder, which it highlights with the HCL TextMate grammar as the language `hcl`. The command "dg: Open diagram" starts `dg serve -C <folder> -open=false -stdin` on a free loopback port and opens a panel beside the text editor: a webview whose only content is a frame of the served app, so the panel has every editor feature that the browser has. Closing the panel stops the server, and `-stdin` stops it when its standard input closes, however the extension host ends. Each VSIX is built for one VS Code platform target with `dg` built for it (`make vsix-<target>`), and holds dg's `LICENSE` and `THIRD_PARTY_NOTICES`, the licenses of every component the binary and the VSIX hold (`make notices` regenerates it; `make lint` fails while it is out of date).
- AC: saving a model file updates the panel within 300 milliseconds (FR-10).
- AC: no network access at runtime (section 11): the panel loads only from the loopback server, and the extension needs nothing it does not bundle.

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
/cmd/dg/                  # serve, check, fmt, export, op, vocabulary, mcp, lsp
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
  /src/canvas/            # JointJS shapes and interactions, the libavoid adapter, text measurement, SVG and JSON output
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

1. **M1 — Walking skeleton.** The thinnest path from file to picture: `node` and `edge` blocks with `kind`, `label` and `parent`; `kind`, `edge_kind`, `style` and `edge_style` with `shape` (`rect`, `rounded`), `fill` and `stroke`; resolve; the embedded web app with the bundled font, elkjs first layout and libavoid routing; `dg export --format svg|json`.
   *Done when:* `examples/hello` exports a golden SVG, and export succeeds with the network blocked (font, WebAssembly and the elk worker are all embedded).
   *Why first:* the risk is in the joins (libavoid WebAssembly and the elk Web Worker inside the embedded app inside headless Chromium), not in any single part.
2. **M2 — Language core.** FR-1, FR-2, FR-3: kind inheritance, properties, ports, `port_range`, `file`, rows, quoted IDs, packs, project-local spec and style, read-only files; `views.hcl` and layout files parsed and validated, and views resolved by `include`, `exclude` and `edges`.
   *Done when:* every FR-2 error and warning has a golden diagnostic with file, line and column; HCL loading and layout YAML are fuzzed; the `dg check` benchmark at 1,000 nodes is under 1 second; `dg fmt` output is golden.
3. **M3 — C4 diagrams.** FR-4 (without collapse and overlays), FR-5, FR-7, FR-8, FR-9 for `free` containers; the full `generic` pack, every built-in shape and custom shapes; the node and edge style properties other than the container policies, markers other than `arrow`, and animation; the project `font`; label wrapping; pinned positions from layout files; `dg serve` read-only (FR-10 `snapshot` and `diagnostics`) with FR-13 reload, so editing HCL in a text editor updates the browser.
   *Done when:* `examples/c4-container` (people, a system boundary holding its containers, external systems, a local `c4` pack) matches its golden SVG with nothing pinned and with a partial layout file (pinned plus spiral-placed nodes); saving a model file updates the browser within 300 milliseconds; a broken file keeps the last snapshot.
4. **M4 — Entity-relationship and class diagrams.** Rows and compartments drawn; rows as ports (section 5.4); every arrow marker including crow's feet; start and end labels; self-loops; several edges between one pair.
   *Done when:* `examples/er` and `examples/class` match their goldens, with each row edge attached to the end of the row facing the other endpoint.
5. **M5 — Animated edges.** `animate = "flow"` (section 4.3) drawn in the browser and in SVG export; `dg export --format png|gif` (FR-14).
   *Done when:* the animated SVG is golden; the GIF is byte-identical across runs and loops without a jump (the drawing at the end of the period equals the first frame); `examples/c4-container` has an animated variant (the `sign-in` view, a dynamic diagram whose interactions are animated).
6. **M6 — Operations and agent surface.** FR-12 except `update_view`, `delete_view`, the decoration operations and `reorder`; `create_view` is pulled forward from M7 so operations can rebuild a project's views; `batch`; inverses and the undo log `dg serve` will use; read-only enforcement; `rename_id`; FR-15 `dg op` and `dg vocabulary`; FR-16 MCP server; frames and frame-relative positions in JSON export.
   *Done when:* every operation has a golden `hclwrite` round-trip; applying an operation then its inverse restores every file byte for byte; a refused operation leaves every file untouched; a file changed between read and write is reloaded and the operation re-applied; `dg op` arguments and MCP requests are fuzzed; `testdata/rebuild/c4-container.sh` rebuilds `examples/c4-container` using only `dg op`, starting from a directory holding only its `dg.hcl` and `c4` pack (operations edit models, views and layouts, not packs), and every view (`main`, `pinned`, `sign-in`) matches its golden SVG; an in-process MCP client builds the same diagram through the MCP tools.
7. **M7 — Views.** Section 8 in full: collapse, edge lifting and merging, drill-down badges, breadcrumbs and view switcher, overlays, view style rules, decorations (including attached), a view's `direction`; the view operations other than `create_view` and the decoration operations, including `promote_decoration`.
   *Done when:* every lifting and merging case in section 8.3 has a golden resolved view; `examples/c4` shows three levels (system context with collapsed systems, a container view with `of` on a system, a component view with `of` on a container) navigable by drill-down; an incident example draws an overlay.
8. **M8 — Container policies.** `stack` and `grid`, `order`, the `reorder` operation; JSON export marks the children of `stack` and `grid` containers, which `move` refuses. A view with `of` on a `stack` or `grid` container lays its children out by that container's policy.
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
| Canvas | JointJS + libavoid | tldraw: production license key required. React Flow: no obstacle-avoiding routing. maxGraph: fallback option, weaker routing. Sprotty: heavier than needed. |
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
| Calling libavoid | `libavoid-js` directly | `@joint/router-avoid`: no `via` checkpoints, no ends at containers (which are not obstacles), orthogonal only, and it routes a JointJS graph that export would build only for it. |
| Edge animation in the page | the SVG's CSS animation in a `<style>` element, allowed by `style-src 'self' 'unsafe-inline'` in the content security policy; a refused style fails the export and shows an error in the editor. Fonts, pictures and connections stay limited to the embedded files (and `data:` pictures), so CSS cannot send anything out. Frames are captured by pausing the page's animations at the frame's time | Constructed style sheets: they pass only because browsers do not apply `style-src` to them, which no specification guarantees. A per-response nonce: `index.html` rewritten for every request in `dg serve` and export, for no gain while nothing else can load. Animating with the Web Animations API or computing each frame's dash offset: a second animation path beside the exported CSS, which can drift from it. |
| Editor drawing | the editor canvas puts the export's node and edge markup (`web/src/canvas/markup.ts`) in JointJS elements, so there is one drawing path | JointJS shapes for the editor and a DOM copy for export: the export would hold editor state. Two renderers kept equal by tests: they drift. |
| Inverse of an operation | the files it wrote, with their content before and after; undo and redo apply only while the files hold what they expect | An inverse operation per operation (`create_node` undone by `delete_node`, …): it cannot restore byte for byte the comments of a deleted block, the formatting of an edited one, a default view block written out or a canonicalized layout file. |
| Operation IDs | an optional `id` on `create_node` and `connect`; generated from the label or ends when omitted | Generated IDs only: an agent cannot refer to a node it creates in the same batch without predicting the generated ID, nor rebuild a model with its IDs. |
| Concurrent writers | serialized by an advisory lock on the project directory (Windows: a file in the user's cache directory), waited for up to 10 seconds | A lock file in the project: litters the user's repository. No lock, relying on the unchanged-since-read check: two writers can both pass it, and parallel `dg op` runs lost changes while reporting success. |
| MCP `op` tool schema | the union of every operation's fields, each described per operation; required fields in the tool's description and checked by dg | `oneOf` with one schema per operation: the Anthropic API refuses `oneOf` at the top of a tool's input schema. |
| PNG and GIF size | one pixel per unit, on white, also for MCP | A `--scale` flag: no use yet; an agent sees the text at the size it is drawn. |
| Overlay outline | the convex hull of the padded member boxes, round-cornered (bubble sets are an open decision) | The bounding rectangle: covers far more that is not a member. The union of the padded boxes: separate islands for members apart, which reads as several overlays. A concave hull: its shape depends on a tuning parameter and jumps as nodes move. |
| Merged lifted edges | one edge labeled `×N`, keyed `lift:<from>:<to>` whatever N | Joining the labels of the merged edges: long and unreadable at three or more. The key of the first edge when N = 1: its hint would follow it out of the lifted edge when a second edge joins. |
| Breadcrumbs | levels of views leading back to where `X` is drawn, a level of several views one crumb with a menu | Only the views with `of` on ancestors: a container view whose system has no such view leads nowhere. Every view of every level in one row: reads as a path through views that are alternatives. |
| Badges in exports | the collapse badge only | Both badges: a drill-down badge does nothing in a picture. Neither: a collapsed box would look like a leaf. |
| Layout direction | a view's `direction` attribute, defaulting to the rules of section 7.4 | The rules only: a view cannot choose. A style property of edge kinds: direction belongs to a view, not to the kinds it shows. |
| First layout of the free containers in a stack (swimlanes) | one elk run over the steps of all lanes, layers across the stack, then rows packed per lane | One elk run per lane: steps start at the left of every lane whatever the edges between lanes, so cross-lane flows zigzag. The whole view in one elk run with lanes as compound nodes: elk places compound nodes side by side in layers, not stacked, and has no swimlane support. |
| Cycles in the first layout | dg reverses the back edges of a depth-first search from the nodes no edge enters; cycles no such node leads to are left to elk | elk's greedy cycle breaking alone: a decision with more edges out than in goes first, so a flowchart with a loop back can be drawn upside down. elk's `DEPTH_FIRST`: starts from the first node by ID when a cycle has no source, which drew `examples/c4-container`'s customer at the bottom. |
| Rotated container label | `label_side = "left"`, centered along the left edge with a line between label and content | Label at the top of each lane: lanes stacked vertically read as rows of boxes, not swimlanes. Unrotated text on the left: a long label makes a wide strip in every lane. |
| Language server protocol layer | JSON-RPC framing, dispatch and the protocol types dg uses, written in `internal/lsp` on the standard library | A library (`go.lsp.dev/protocol` with `go.lsp.dev/jsonrpc2`, `github.com/tliron/glsp`): dependencies and generated types for the whole protocol, to save about a hundred lines. |
| Rename in the language server | `rename_id` planned on the buffers, its files returned as one workspace edit | Renaming the occurrences the server's index finds: a second implementation of section 6.4 that would drift from the operation, and misses the layout files' values and the read-only refusal. |
| Diagram panel in VS Code | a webview framing `dg serve`'s page on a loopback port | The embedded app served as webview resources, with a message bridge to the extension: a second transport beside `dg serve`'s WebSocket for the editor of M10 and M11 to keep equal. |
| Extension JavaScript | compiled by `tsc`, packaged with its production `node_modules` | Bundled by esbuild: one more build dependency for a faster start of an extension of one file (`vsce` warns about the number of files). |
| Spiral clearance | four grid units | Two grid units: with libavoid's buffer around each box no edge fits between two neighbors, and libavoid draws a straight line through them. |
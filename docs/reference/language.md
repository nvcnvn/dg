# Language

dg reads HCL files: the project file, packs (a spec and a style), the model, and views. This page
covers the project file, kinds and the model. [Styles](styles.md) and [views](views.md) have
their own pages.

<!-- begin generated toc: go test ./cmd/dg -run TestDocs -update -->

- [The project file](#the-project-file)
- [Kinds](#kinds)
- [Nodes](#nodes)
  - [Ports](#ports)
  - [Rows and compartments](#rows-and-compartments)
- [Edges](#edges)
- [IDs and references](#ids-and-references)
- [Functions](#functions)
- [Validation](#validation)

<!-- end toc -->

## The project file

`dg.hcl` at the project's root holds at most one `project` block. The file and every field are
optional; these are the defaults, except `font`, which has none:

```hcl
project "my-diagrams" {
  packs      = []
  models     = ["*.hcl"]
  read_only  = []
  views      = "views.hcl"
  layout_dir = "views"
  grid       = 10
  font       = "fonts/Brand.woff2"
}
```

- Paths are slash-separated, relative to the project root, and stay inside it. Symbolic links are
  followed only while they stay inside the project.
- `layout_dir` holds the views' layout files, and `grid` is the step of every coordinate and size
  in them.
- `packs` lists directories, each with an optional `spec.hcl` and `style.hcl`. Listing
  `"generic"` is allowed and changes nothing.
- `models` takes glob patterns without `**`. By default every `*.hcl` file in the root is a model
  file, except `dg.hcl`, the views file, and the `spec.hcl` and `style.hcl` of the project and
  its packs, which never are. A pattern that matches no file is a warning.
- `read_only` files, such as a model generated from a running system, are never written: `dg
  fmt` skips them, operations that would change them are refused naming the file, and the editor
  shows their nodes with a lock (they can still be moved, since positions are in layout files).
- `font` replaces the bundled Inter font for all text. Exported SVG names it by the file's base
  name (`Brand, sans-serif`).

Kinds and styles load in order: the generic pack, the listed packs in order, then the project's
own `spec.hcl` and `style.hcl`. A later `kind`, `edge_kind`, `shape` or `overlay_style` of the same
name replaces the earlier one; a later `style` or `edge_style` with the same label overrides only
the properties it sets. `dg check` reports each override as information.

## Kinds

A spec file declares node kinds and edge kinds:

```hcl
kind "gpu-node" {
  extends           = "node"            # the default
  ports             = ["nic0", "nic1"]  # every node of the kind has them
  default_port_side = "auto"            # for ports declared without one
  property "gpu_count" {
    type    = number                    # string, number or bool
    default = 8
  }
}

kind "rack" {
  container = true                      # nodes may set parent to it
}

kind "table" {
  rows = true                           # nodes may declare rows
}

edge_kind "rdma" {}                     # extends "default"
```

Every kind extends `node`, directly or through `extends`, and every edge kind extends `default`.
A kind inherits `container`, `rows`, `ports`, `default_port_side` and properties, and may add
ports and properties, but not declare one it inherits again. `ports` may call
[`port_range`](#functions).

Styles select kinds including their sub-kinds, and so do the selectors of views: `kind.service`
matches a kind that extends `service`. The [generic pack](styles.md#the-generic-pack) defines the
kinds most diagrams need.

## Nodes

```hcl
node "worker-1" {
  kind   = "gpu-node"             # default "node"
  label  = "Worker 1"             # default: the ID
  parent = node.pool              # a node whose kind is a container
  tags   = ["critical"]
  props  = { gpu_count = 8 }

  port "debug" {                  # an instance port
    side = "bottom"               # top, right, bottom, left or auto
  }
}
```

- `label` may hold `\n` to break lines; long lines wrap at the style's `max_w`.
- `parent` makes the node a child of a container. Containment is a tree.
- `tags` are simple IDs. Styles and selectors use them: `style "tag.critical"`,
  `include = [tag.critical]`.
- `props` sets the properties the kind declares, with values of exactly the declared type.
  Properties left out take the kind's `default`. Properties are data for people and tools; they do
  not change the drawing.

### Ports

A port is a named point on a node's border where edges attach. A node's ports are its kind's
`ports` plus its own, declared with `port` blocks or all at once with
`ports = port_range("p", 1, 48)`. A port's `side` is `top`, `right`, `bottom`, `left` or `auto`
(the side facing the other end of its edge); ports on one side are spread evenly in declaration
order. A custom shape can place ports at its anchors, and the editor can pin a port anywhere along
a side (stored in the layout file).

### Rows and compartments

A node whose kind has `rows = true`, such as `table` and `class`, may declare rows, either all
directly or all grouped in compartments:

```hcl
node "orders" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "PK"
  }
  row "customer_id" {
    type = "uuid"
    key  = "FK"
  }
}

node "user" {
  kind = "class"
  compartment "fields" {
    row "name" {}
  }
  compartment "methods" {
    row "save()" {}
  }
}
```

`type` and `key` are optional strings. A row is drawn as a line of the box, its key in bold, its
name and its type at the right, and compartments are separated by lines. A row is also a port:
an edge to `node.orders.row.customer_id` attaches at the end of that row facing the other end.

## Edges

```hcl
edge "w1-tor" {
  from  = node.worker-1.port.nic0
  to    = node.tor-1.port.p1
  kind  = "rdma"                  # default "default"
  label = "400G"                  # the middle label
  tags  = ["hot"]
}

edge "places" {
  from   = node.Customer
  to     = node.Order
  labels = { start = "1", mid = "places", end = "0..*" }
}
```

- `from` and `to` are `node.<id>`, `node.<id>.port.<port>` or `node.<id>.row.<row>`.
- `label` is short for `labels = { mid = "…" }`; `start` and `end` labels sit beside the ends.
  An edge has one or the other.
- Several edges may join the same two nodes, an edge may join a node to itself, and an edge may
  end at a container.
- Arrowheads, dashes and colors come from the edge kind's style and the edge's tags.

The generic pack's `junction` kind is a small dot for buses and fan-out: edges end at it like at
any node.

## IDs and references

The label of a `node`, `edge`, `view`, `kind`, `port`, `row` or other block is its ID.

- A **simple ID** matches `[A-Za-z_][A-Za-z0-9_-]*` and is referenced as `node.api` or
  `node.worker-1`.
- A **quoted ID** is any other text of up to 200 characters without control characters, as a
  generator might write (`node "kube-system/coredns"`). Reference it with an index:
  `node["kube-system/coredns"]`. Ports and rows may be quoted the same way:
  `node.t.row["save()"]`.
- Kinds, edge kinds, tags, views, overlays and decorations take simple IDs only.
- These words cannot be simple IDs: `all`, `node`, `kind`, `tag`, `children`, `descendants`,
  `edge_kind`, `port`, `row`, `lift`. An edge ID may not start with `lift:`.
- Node IDs and edge IDs are separate: a node and an edge may share an ID. Port and row names are
  unique within their node.

An ID never changes by itself. Renaming one is a refactoring: the editor's "Rename ID…", the
language server's rename, or [`dg op rename_id`](operations.md#rename_id) change the block
and every reference to it in model files, `views.hcl` and layout files at once.

When the editor or an operation creates a node without an ID, the ID is made from the label
(lower case, `a-z`, `0-9` and `-`, such as `orders-db` from "Orders DB"), else from the kind, with
`-2`, `-3`… added when it is taken; a node created without a label gets its ID from the first
label it is given in the same session. An edge's ID is made from its ends, such as `api-db`.

## Functions

Every HCL file can call:

- `port_range(prefix, from, to)`: the list `prefix<from>` … `prefix<to>`, such as
  `port_range("p", 1, 48)` for `p1` to `p48`; at most 10,000 names.
- `file(path)`: the contents of a UTF-8 text file, relative to the file that calls it and inside
  the project. Styles use it for pictures: `icon = file("icons/lock.svg")`.

## Validation

`dg check`, the editor and the language server report the same diagnostics. Errors include
duplicate or invalid IDs; references to nodes, ports, rows, kinds, edge kinds or selectors that do
not exist; `extends` and containment cycles; a `parent` that is not a container; rows on a kind
without `rows = true`; undeclared or mistyped properties; unknown style properties or bad values;
`of` or `collapse` naming a node that is not a container; layout files of views that do not
exist; and files nested past 100 levels, or a statement of more than 100 operators. Warnings include layout entries naming IDs that no longer exist, style labels naming no
kind, selectors that match nothing, and positions stored relative to a container the node is no
longer in.

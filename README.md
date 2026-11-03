# dg

dg draws diagrams from text: you write what exists as an HCL model, arrange it by dragging in a
browser editor, and dg routes the arrows around the boxes every time it draws.

- [Getting started](docs/getting-started.md): build dg, write a first diagram, check it, arrange
  it in the browser and export it.
- [VS Code extension](docs/vscode.md): diagnostics, completion, rename and formatting in the
  editor, and the diagram in a panel beside it.
- [Examples](docs/examples.md): architecture and C4 with drill-down, entity-relationship, class,
  flowchart, swimlane, state machine, network, animated edges, shapes.
- [Reference](docs/reference/README.md): the language, styles, views and layout files, the
  command line, operations, and the editor's gestures.
- [Design and rationale](docs/spec.md): every design decision and why it was made.
- [Development](docs/development.md): building and testing dg, and how this documentation is
  kept current.

These pages are also published as a website at <https://nvcnvn.github.io/dg/>.

## A diagram

From [`examples/hello`](https://github.com/nvcnvn/dg/blob/main/examples/hello/model.hcl):

```hcl
node "backend" {
  kind  = "group"
  label = "Backend"
}

node "api" {
  kind   = "service"
  label  = "API"
  parent = node.backend
}
```

![The hello example, drawn by dg](testdata/export/hello.svg)

The model holds no colors, shapes or positions: kinds such as `service` and `group` decide how
nodes look, positions go in a separate layout file that the editor writes, and edges are never
stored, only routed.

dg is for diagrams of things and how they connect: architecture, infrastructure and networks,
flowcharts and swimlanes, entity-relationship and class diagrams, state machines. It is not for
diagrams where position is data, such as sequence diagrams, Gantt charts and timelines, charts,
or Venn diagrams.

## Status

dg has no releases yet: [build it from source](docs/getting-started.md#install). It is licensed
under the [Apache License 2.0](https://github.com/nvcnvn/dg/blob/main/LICENSE).

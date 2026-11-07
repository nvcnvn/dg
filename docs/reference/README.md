# Reference

- [Language](language.md): the project file, packs, kinds and edge kinds, nodes, edges, ports,
  rows, tags, properties, IDs and references.
- [Styles](styles.md): style rules, their properties, shapes, custom shapes, arrow markers,
  animation, the cascade, and the built-in generic pack.
- [Views and layouts](views.md): views and selectors, collapse and edge lifting, drill-down,
  overlays, view style rules, layout files, pinning, container policies, edge hints and
  decorations.
- [Command line](cli.md): `dg check`, `fmt`, `export`, `serve`, `op`, `vocabulary`, `mcp` and
  `lsp`, with their flags, and the MCP tools.
- [Operations](operations.md): every change `dg op`, the editor and agents make to the files.
- [Editor](editor.md): the gestures and keyboard shortcuts of `dg serve` and the VS Code panel.

[Packs](../packs.md) is the guide to installing packs and writing one.

[Design and rationale](../spec.md), the design specification, gives the reasons behind each rule,
and the exact geometry: how sizes, placement and routing are computed.

## How a project fits together

dg keeps four concerns apart, each in its own files:

| Concern | Files | Holds |
|---|---|---|
| Spec | packs, or the project's `spec.hcl` | kinds of nodes and edges, their ports and properties |
| Style | packs, or the project's `style.hcl` | how kinds and tags are drawn; container policies; custom shapes |
| Model | `*.hcl` model files | nodes and edges: what exists and how it is connected and nested |
| View | `views.hcl` and `views/<view>.layout.yaml` | what each view shows, and where its nodes sit |

A pack is a spec and a style shipped together; the built-in `generic` pack is always loaded.
Most diagrams need only a model, and views once they need more than one picture.

A typical project:

```text
my-diagrams/
├── dg.hcl                  project file (optional)
├── spec.hcl                the project's own kinds (optional)
├── style.hcl               the project's own styles (optional)
├── model.hcl               the model; any number of files
├── views.hcl               views (optional)
├── views/
│   └── main.layout.yaml    where the nodes of view main sit
└── packs/c4/               a local pack (optional)
    ├── spec.hcl
    └── style.hcl
```

Three rules hold everywhere:

1. **An ID is not a label.** Changing a label never changes an ID, and an ID never encodes where
   a node is nested: containment is the `parent` attribute.
2. **A kind is not a shape.** The model says `kind = "database"`; the style draws it as a
   cylinder. The model never holds a color, shape, position, view name or file path.
3. **Arrows are never stored.** Edges are routed around the boxes each time a view is drawn;
   layout files hold only hints such as the side an edge leaves by.

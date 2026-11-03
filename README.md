# dg

Diagram-as-code: an HCL model, a browser-arranged layout, and arrows routed at render time.
`docs/spec.md` is the design.

## Build

Needs Go, Node with npm, and Google Chrome or Chromium (for `dg export` and its tests).

The Go packages embed the web app from `web/dist`, which is built output and not committed, so
build the web app before any `go` command:

```sh
make web     # npm ci and vite build into web/dist
make build   # bin/dg
make test    # go test ./... and the web unit tests
make lint    # gofmt, go vet, staticcheck, govulncheck, tsc --noEmit (web app and extension)
```

After `make web`, plain `go build ./...` and `go test ./...` work. `go test -short ./...` skips the
tests that start Chromium. Golden files are rewritten with
`go test ./cmd/dg ./internal/export ./internal/view -update`: diagnostics of the projects under
`testdata/check`, `dg fmt` output in `testdata/fmt/want`, resolved views and exports of every
view of every example (PNG and GIF exports are not golden: the tests compare two runs).
`go test ./internal/ops -update` rewrites the operation goldens in `testdata/ops/want` (the files
each operation writes to a copy of `testdata/ops/in`). `testdata/lift` has a view for each case of
edge lifting and merging; its resolved views are goldens like the examples'.
`go test ./internal/lsp -update` rewrites the language server's recorded sessions'
goldens: each `testdata/lsp/*.jsonl` names a project on its first line and holds the client's
messages, and its `.golden` file what the server answered.
`go test ./internal/project -bench Load1000` times `dg check` at 1,000 nodes, and
`go test ./internal/lsp -bench Keystroke` one keystroke in the language server at 1,000 nodes, and
`go test ./internal/server -run DragPerformance -v` reads a Chromium performance trace of drags in a
view of 200 nodes and 400 edges (frames per second, longest task); the fuzz targets
are `FuzzLoad` (`internal/project`), `FuzzDecodeLayout` (`internal/view`),
`FuzzDecodeMessage` (`internal/server`, the WebSocket protocol), `FuzzOp` (`internal/ops`,
operations as JSON), `FuzzDiff` and `FuzzMerge3` (`internal/ops`, the line diff and merge of undo), `FuzzOpArgs` (`cmd/dg`, `dg op` command lines), `FuzzTools`
(`internal/mcp`, MCP tool calls), and `FuzzMessage` and `FuzzBuffer` (`internal/lsp`, any message,
and every request at any position of any buffer).

### VS Code extension

`vscode/` is the extension. It bundles `dg`, so each package is built for one platform:

```sh
make vsix                 # bin/dg-<this platform>.vsix, such as bin/dg-darwin-arm64.vsix
make vsix-linux-x64       # one target: darwin-arm64, darwin-x64, linux-arm64, linux-x64,
make vsix-all             #   win32-arm64 or win32-x64; or all of them
make test-vscode          # starts VS Code on a copy of examples/c4 with the extension from vscode/
make test-vsix            # the same test on the package for this platform, installed
code --install-extension bin/dg-darwin-arm64.vsix
make notices              # regenerate THIRD_PARTY_NOTICES after a dependency changes
```

Each package holds `LICENSE` and `THIRD_PARTY_NOTICES`, the licenses of the Go modules, npm packages
and grammar that the binary and the extension hold; `make lint` fails while the file is out of
date.

The tests use VS Code Insiders where macOS installs it, else the VS Code that `DG_VSCODE` names,
else a build `@vscode/test-electron` downloads.

## Use

```sh
bin/dg check -C examples/inference            # --strict: warnings fail; --json: machine-readable
bin/dg fmt -C examples/inference              # rewrites writable files; --check: list, write nothing
bin/dg export -C examples/hello --format svg --out hello.svg
bin/dg export -C examples/inference --view services --format json
bin/dg export -C examples/c4-container --view sign-in --format gif --out sign-in.gif
bin/dg export -C examples/c4-container --all --format png --out pictures   # one file per view
bin/dg serve -C examples/c4-container         # opens the browser; redraws when a file is saved
```

Operations change the files (`dg op --help` lists them; `dg op <operation> --help` shows its
flags and JSON form):

```sh
bin/dg op create_node -C my-diagram --kind service --label "API"        # ID api
bin/dg op create_node -C my-diagram --kind database --id db --label "Orders"
bin/dg op connect -C my-diagram --from api --to db --label "SQL" --json
bin/dg op move -C my-diagram --view main --id db --x 200 --y 0
bin/dg op reorder -C my-diagram --view main --container lanes \
  --order intake --order review                                        # stack or grid order
bin/dg op update_view -C my-diagram --id main \
  --collapse node.backend --unset edges                                # collapse, show every edge
bin/dg op add_decoration -C my-diagram --view main \
  --decoration '{"type": "text", "x": 0, "y": -60, "text": "Title"}'  # a title above the drawing
bin/dg op batch -C my-diagram < ops.json   # a JSON list of operations, applied as one change
bin/dg vocabulary -C my-diagram            # kinds, edge kinds, tags and views, as JSON
bin/dg mcp -C my-diagram                   # MCP server over stdio: check, vocabulary, op, export
bin/dg lsp -C my-diagram                   # language server over stdio, for editors
```

An operation that would add a validation error, or change a `read_only` file, is refused and
writes nothing. `dg op` computes no geometry: `dg export --format json` gives each node's `frame`
and its position `rel` in it, which `move` takes; a node a stack or grid container places also has
`ordered_in`, the container `reorder` takes, since `move` refuses it. The scripts in `testdata/rebuild` rebuild
`examples/c4-container`, `examples/c4`, `examples/incident`, `examples/swimlane`,
`examples/flowchart` and `examples/state-machine` with operations alone.

`dg serve` is the editor. It redraws within tens of milliseconds of a save, and while the
project has errors it lists them and keeps the last good drawing, read-only. A view is chosen from
the switcher, from a node's drill-down badge (the views whose `of` is that node; a menu when there
are several) or from the breadcrumbs of a view with `of`. `-addr` sets the address (default a free
loopback port), `-open=false` skips opening the browser, and `-stdin` stops `dg serve` when its
standard input closes, as when the editor that started it exits.

Layout gestures write the view's layout file, each as one operation (one undo step) sent when it
is dropped:

- select with a click, Shift-click, or a box dragged on empty canvas; Ctrl/⌘+A selects every node;
- drag nodes to move them (in a view with nothing pinned, the first drag pins every node where it
  is drawn); a node stays inside its container, which grows unless its size is pinned;
- drag a corner handle of a selected node to resize it; drag a child of a stack or grid container
  to reorder it; drag a port handle of a selected node along its border to pin the port;
- select an edge to drag an end to another side or its middle label along it; double-click the
  edge to add a via point, which can be dragged, and double-click a via point to remove it;
- Unpin, Align and Distribute act on the selection, from the toolbar or the right-click menu
  (U unpins);
- Undo and Redo (Ctrl/⌘+Z, Ctrl/⌘+Shift+Z) take back the session's changes, whichever browser made
  them; an undo keeps edits made since in a text editor elsewhere in the same files;
- pan with the scroll bars, the middle button or Space+drag; zoom with Ctrl/⌘+wheel, +, − and 0.

`dg lsp` serves the Language Server Protocol for a project's HCL files, on the editor's unsaved
buffers: the diagnostics of `dg check`, completion of kinds, IDs, ports, rows, selectors and
styles, hover, go to definition and find references, rename through `rename_id` (layout files
included), and formatting as `dg fmt`. The VS Code extension runs it in a folder with `dg.hcl`;
its command "dg: Open diagram" opens the `dg serve` editor in a panel beside the text, redrawn
when a file is saved.

`dg export --format` is `svg` (edge animation kept as CSS), `png` (animation at time 0), `gif`
(one half-second animation period in 10 frames, looping) or `json` (geometry).

The examples are `hello`, `inference` (packs, read-only generated files, ports), `c4-container`
(a C4 container diagram with a local `c4` pack, laid out by elk in `main` and from a partial
layout file in `pinned`, and a dynamic diagram `sign-in` whose interactions are animated edges),
`c4` (the three levels of the C4 model: a system context with its software systems collapsed, the
containers of one system and the components of one container, each opened from the drill-down
badge of the level above, relationships written between components and lifted to what a view
draws), `incident` (an overlay outlining a blast radius, view style rules, a collapsed tier, and
decorations of every type around a pinned layout), `shapes` (every built-in shape, custom shapes,
icons, style properties, arrow markers, ports and edge hints), `er` (tables with keyed rows,
foreign keys from row to row with crow's-foot cardinality, a self-reference), `class` (classes
with compartments, inheritance, composition, aggregation, multiplicities, a self-association),
`swimlane` (a lane set, drawn as a pool, stacking lanes in a stored order, rotated lane labels,
steps flowing across the lanes, and a view whose `of` is the lane set), `flowchart` (steps,
decisions, a loop back to the checks held to one side by edge hints, and parallel checks in a grid
with centered columns) and `state-machine` (simple, initial and final states, a self-transition,
and a state whose concurrent regions are stacked inside it).

`dg check` exits 0 without errors, 1 with errors, 2 when dg itself fails.

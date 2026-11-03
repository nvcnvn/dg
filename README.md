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
make lint    # gofmt, go vet, staticcheck, govulncheck, tsc --noEmit
```

After `make web`, plain `go build ./...` and `go test ./...` work. `go test -short ./...` skips the
tests that start Chromium. Golden files are rewritten with
`go test ./cmd/dg ./internal/export ./internal/view -update`: diagnostics of the projects under
`testdata/check`, `dg fmt` output in `testdata/fmt/want`, resolved views and exports of every
view of every example (PNG and GIF exports are not golden: the tests compare two runs).
`go test ./internal/ops -update` rewrites the operation goldens in `testdata/ops/want` (the files
each operation writes to a copy of `testdata/ops/in`). `testdata/lift` has a view for each case of
edge lifting and merging; its resolved views are goldens like the examples'.
`go test ./internal/project -bench Load1000` times `dg check` at 1,000 nodes; the fuzz targets
are `FuzzLoad` (`internal/project`), `FuzzDecodeLayout` (`internal/view`),
`FuzzDecodeMessage` (`internal/server`, the WebSocket protocol), `FuzzOp` (`internal/ops`,
operations as JSON), `FuzzOpArgs` (`cmd/dg`, `dg op` command lines) and `FuzzTools`
(`internal/mcp`, MCP tool calls).

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
```

An operation that would add a validation error, or change a `read_only` file, is refused and
writes nothing. `dg op` computes no geometry: `dg export --format json` gives each node's `frame`
and its position `rel` in it, which `move` takes; a node a stack or grid container places also has
`ordered_in`, the container `reorder` takes, since `move` refuses it. The scripts in `testdata/rebuild` rebuild
`examples/c4-container`, `examples/c4`, `examples/incident`, `examples/swimlane`,
`examples/flowchart` and `examples/state-machine` with operations alone.

`dg serve` shows the views read-only (editing comes later): it redraws within tens of
milliseconds of a save, and while the project has errors it lists them and keeps the last good
drawing. A view is chosen from the switcher, from a node's drill-down badge (the views whose `of`
is that node; a menu when there are several) or from the breadcrumbs of a view with `of`. `-addr`
sets the address (default a free loopback port), `-open=false` skips opening the browser.

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

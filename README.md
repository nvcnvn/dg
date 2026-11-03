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
`go test ./internal/project -bench Load1000` times `dg check` at 1,000 nodes; the fuzz targets
are `FuzzLoad` (`internal/project`), `FuzzDecodeLayout` (`internal/view`) and
`FuzzDecodeMessage` (`internal/server`, the WebSocket protocol).

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

`dg serve` shows the views read-only (editing comes later): it redraws within tens of
milliseconds of a save, and while the project has errors it lists them and keeps the last good
drawing. `-addr` sets the address (default a free loopback port), `-open=false` skips opening the
browser.

`dg export --format` is `svg` (edge animation kept as CSS), `png` (animation at time 0), `gif`
(one half-second animation period in 10 frames, looping) or `json` (geometry).

The examples are `hello`, `inference` (packs, read-only generated files, ports), `c4-container`
(a C4 container diagram with a local `c4` pack, laid out by elk in `main` and from a partial
layout file in `pinned`, and a dynamic diagram `sign-in` whose interactions are animated edges),
`shapes` (every built-in shape, custom shapes, icons, style properties, arrow markers, ports and
edge hints), `er` (tables with keyed rows, foreign keys from row to row with crow's-foot
cardinality, a self-reference) and `class` (classes with compartments, inheritance,
composition, aggregation, multiplicities, a self-association).

`dg check` exits 0 without errors, 1 with errors, 2 when dg itself fails.

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
view of every example. `go test ./internal/project -bench Load1000` times `dg check` at 1,000
nodes; the fuzz targets are `FuzzLoad` (`internal/project`) and `FuzzDecodeLayout`
(`internal/view`).

## Use

```sh
bin/dg check -C examples/inference            # --strict: warnings fail; --json: machine-readable
bin/dg fmt -C examples/inference              # rewrites writable files; --check: list, write nothing
bin/dg export -C examples/hello --format svg --out hello.svg
bin/dg export -C examples/inference --view services --format json
```

`dg check` exits 0 without errors, 1 with errors, 2 when dg itself fails.

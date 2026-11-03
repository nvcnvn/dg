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
`go test ./internal/export ./internal/view -update`.

## Use

```sh
bin/dg export -C examples/hello --format svg --out hello.svg
bin/dg export -C examples/hello --format json
```

# Tech debts

Every workaround, version pin and pre-release dependency, with why it exists and what must be true
to remove it.

## Go toolchain pinned to go1.27.3

- **Where:** `toolchain go1.27.3` in `go.mod`.
- **Why:** go1.27.0 has `net/http` vulnerabilities that `govulncheck` reports (GO-2026-6603,
  GO-2026-6609, GO-2026-6611, GO-2026-6612, GO-2026-6613, GO-2026-6617), fixed in go1.27.2, and
  go1.27.2 has a `crypto/x509` one (GO-2026-6809), fixed in go1.27.3.
- **Remove when:** the `go` directive is raised to 1.27.3 or later.

## Pre-release `github.com/go-json-experiment/json`

- **Where:** indirect dependency of `github.com/chromedp/chromedp` (through `cdproto`), at a
  pseudo-version.
- **Why:** chromedp depends on it; there is no tagged release.
- **Remove when:** chromedp depends on a tagged release or on the standard library's
  `encoding/json/v2`.

## libavoid.wasm imported by path

- **Where:** `web/src/main.ts` imports `../node_modules/libavoid-js/dist/libavoid.wasm?url`.
- **Why:** the `exports` field of `libavoid-js` does not export the `.wasm` file, so Vite cannot
  resolve it by package name.
- **Remove when:** `libavoid-js` exports `./dist/libavoid.wasm`, or `@joint/router-avoid` exposes
  the file's URL.

## Edges that M1 cannot route

- **Where:** `view.Sources.Resolve` refuses self-loops and edges between a container and its own
  descendant with a "Not supported yet" error, so `dg export` fails on them while `dg check`
  accepts them. An edge between a node and an unrelated container is
  accepted, but `web/src/canvas/avoid.ts` gives it router-avoid's right-angle fallback route, which
  ignores obstacles: containers are not libavoid obstacles (spec 7.5), and libavoid connects only
  shapes it tracks.
- **Why:** M1 routes leaf-to-leaf edges only.
- **Remove when:** self-loops route around one corner of the node (spec 7.5, M4), and edges ending
  at containers, including a container's own descendants, are routed by libavoid, for example
  through border pins on a container shape that is not an obstacle (M3).

## router-avoid's WorkerProvider replaced by a stub

- **Where:** the `resolve.alias` in `web/vite.config.ts` points `./providers/WorkerProvider.mjs`
  at `web/src/canvas/worker-provider-stub.ts`.
- **Why:** `@joint/router-avoid` imports its Worker provider unconditionally, so Vite bundled an
  83 KB worker script that dg never runs (dg calls `initAvoidRouter` without `worker`).
- **Remove when:** router-avoid loads the Worker provider lazily or exposes a main-thread-only
  entry point.

## Export goldens are generated on macOS Chrome

- **Where:** `testdata/export/*.svg` and `*.json`.
- **Why:** sizes depend on Chromium's text shaping of the bundled font. Sizes snap to the grid,
  label boxes to the grid and every emitted coordinate is a whole pixel, but whether that hides
  every difference between platforms and Chromium versions is unproven.
- **Remove when:** the goldens pass unchanged on Linux CI and macOS; otherwise keep per-platform
  goldens.

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
- **Remove when:** `libavoid-js` exports `./dist/libavoid.wasm`.

## How dg drives libavoid

- **Where:** `web/src/canvas/avoid.ts` (the local `Avoid` interface, `UNIFY_MAX_EDGES`) and
  `web/src/route/edges.ts` (`STUB`, `join`, `awayFromBorders`).
- **Why:** the package's typings do not match its module (no `destroy`, wrong `ConnEnd`
  overloads), and its WebIDL binds neither `ConnEnd(point, directions)` nor
  `ConnRef::setRoutingCheckpoints` nor `ClusterRef`. Pins with directions on a box's border make
  libavoid about fifty times slower, so every end with a direction is routed from a free point a
  stub outside it; a `via` route is separate legs; segments along container borders are moved in
  dg's own code; and libavoid's unifying nudging step, which keeps parallel segments apart but
  takes minutes on dense views, runs only up to 60 edges.
- **Remove when:** `libavoid-js` ships matching typings and binds the calls above, and directional
  pins and the unifying step are fast at 400 edges.

## Export goldens are generated on macOS Chrome

- **Where:** `testdata/export/*.svg` and `*.json`.
- **Why:** sizes depend on Chromium's text shaping of the bundled font. Sizes snap to the grid,
  label boxes to the grid and every emitted coordinate is a whole pixel, but whether that hides
  every difference between platforms and Chromium versions is unproven.
- **Remove when:** the goldens pass unchanged on Linux CI and macOS; otherwise keep per-platform
  goldens.

## End labels where several edges converge on one side

- **Where:** `examples/class` leaves out a `customer-address` dependency edge.
- **Why:** when three edges meet one side of a node, `placeEndLabel` can still put a start or end
  label beside the wrong line or away from every line; the example omits the edge so its golden
  shows correct labels.
- **Remove when:** end labels stay beside their own edge with three or more edges converging on
  one side; then restore the edge in `examples/class` and regenerate its golden.

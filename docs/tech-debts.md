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
  `web/src/route/edges.ts` (`STUB`, `clearStub`, `join`, `joinLegs`, `awayFromBorders`).
- **Why:** the package's typings do not match its module (no `destroy`, wrong `ConnEnd`
  overloads), and its WebIDL binds neither `ConnEnd(point, directions)` nor
  `ConnRef::setRoutingCheckpoints` nor `ClusterRef`. Pins with directions on a box's border make
  libavoid about fifty times slower, so every end with a direction is routed from a free point a
  stub outside it, cut short where it would land in an obstacle's buffer, from which libavoid
  routes straight through the obstacle; a `via` route is separate legs; segments along container
  borders are moved in dg's own code; and libavoid's unifying nudging step, which keeps parallel
  segments apart but takes minutes on dense views, runs only up to 60 edges.
- **Remove when:** `libavoid-js` ships matching typings and binds the calls above, and directional
  pins and the unifying step are fast at 400 edges.

## libavoid aborts on a leg from a pin to a free point it sees

- **Where:** `seesPin`, `pinToFree`, `current`, `restart` and the reversed legs of `session` in
  `web/src/canvas/avoid.ts`, including `current` reading and `restart` clearing the `avoidLib` field
  that `AvoidLib.load` guards on, which is not public.
- **Why:** `AStarPathPrivate::search` in libavoid's `makepath.cpp` follows an edge from an
  orthogonal connector's free target to a connection pin as if the pin were the target's own, and
  for the connector whose source is that pin it reaches the zero-length edge to the source, so the
  assertion at line 938 fails. A free point sees a pin only along the pin's center lines, as far
  as the first line the sweep draws across them: usually the edge of the buffer, but further where
  neighbors leave a side of the shape without its line. The assertion kills the module for good.
  So a leg from a pin to any point on those lines runs from the point instead, which keeps the
  point exactly, unlike moving it off the line. The live router reverses every leg from a pin to a
  free point, since what is on a center line changes each frame. One other assertion is known, in
  `CmpLineOrder` (`orthogonal.cpp` line 2313), reached with a node lying over another: about once in
  160,000 fuzzed drag frames, and never in fuzzed views without overlapping nodes. `restart` cannot
  prevent that one; it reports it and loads the module again.
- **Remove when:** libavoid skips a pin that is not the target's own there and no longer asserts
  in `CmpLineOrder`, `libavoid-js` ships that build, and the package offers a way to load the module
  again.

## Chrome's sandbox on Ubuntu CI runners

- **Where:** the step that runs `sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` in
  the Linux jobs of `.github/workflows/ci.yml` and `release.yml`.
- **Why:** Ubuntu 24.04 restricts unprivileged user namespaces with AppArmor, and Google Chrome
  installed on GitHub's runners has no AppArmor profile that allows them, so it stops with "No
  usable sandbox!" and every test that starts it fails. Running Chrome without its sandbox
  instead would change what dg does on users' machines.
- **Remove when:** Chrome starts with its sandbox on GitHub's Ubuntu runners without the step.

## Drag frame rates not measured on CI

- **Where:** `TestDragPerformance` in `internal/server/perf_test.go` skips itself when `CI` is
  set.
- **Why:** GitHub's runners are virtual machines that share their CPU and have no GPU; on the
  macOS runner a drag in the dense view draws 27 frames per second, so the test would measure the
  machine. It runs on developers' machines, where the 60 frames per second of section 11 hold.
- **Remove when:** CI has runners with a GPU and dedicated CPUs, where the test passes.

## End labels where several edges converge on one side

- **Where:** `examples/class` leaves out a `customer-address` dependency edge.
- **Why:** when three edges meet one side of a node, `placeEndLabel` can still put a start or end
  label beside the wrong line or away from every line; the example omits the edge so its golden
  shows correct labels.
- **Remove when:** end labels stay beside their own edge with three or more edges converging on
  one side; then restore the edge in `examples/class` and regenerate its golden.

## Writing files with hclwrite token by token

- **Where:** `internal/ops/hcl.go` (`source`, `setAttr`, `newBlock`, `expand`, `removeBlocks`,
  `setLabel`, `attrsBeforeBlocks`).
- **Why:** `hclwrite`'s `File.Bytes` formats the whole file, so dg writes the file's tokens itself
  and spaces each token it adds; there is no API to format one attribute or block, to remove a
  block with the blank line that separated it, to add an attribute to a block on one line or
  before a body's nested blocks (`SetAttributeRaw` appends it after them);
  `Block.SetLabels` drops the space before the label; and the lexer keeps leading whitespace as
  a count of spaces, so a tab in an edited file becomes a space.
- **Remove when:** `hclwrite` writes tokens without formatting, formats a range of tokens, keeps
  whitespace as written, inserts an attribute where it is asked to, and removes blocks and sets
  labels without disturbing their neighbors.

## HCL grammar vendored and modified

- **Where:** `vscode/syntaxes/hcl.tmGrammar.json` and its `LICENSE`, from release v0.7.1 of
  `github.com/hashicorp/syntax`, with `scopeName` changed from `source.hcl` to `source.dg` and a
  `comment` saying so.
- **Why:** the grammar is published as a release file only, with no package to depend on; and VS
  Code registers a grammar under the scope name inside the file, so the grammar of the language
  `dg` must not be `source.hcl`, which HashiCorp's extension also contributes.
- **Remove when:** `hashicorp/syntax` publishes the grammar as a package; then depend on it and
  write the `source.dg` copy at `make vsix`. Until then, a new release is copied by hand and the
  two changes made again.

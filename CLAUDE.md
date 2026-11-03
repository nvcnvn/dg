# dg

Diagram-as-code: HCL model, browser-arranged layout, arrows routed at render time.
`docs/spec.md` is the source of truth; build in the milestone order of its section 13.

## Boundaries
- Go parses, validates, resolves views and writes files. The browser measures text, sizes,
  places, routes and draws. Go never measures text or computes an absolute coordinate; the
  browser never writes a project file. A feature that seems to need either is in the wrong half.
- The model never holds a color, shape, position, view name or file path (spec 2.2). Styling goes
  through kinds, tags and style rules; geometry goes in layout files.
- Everything Go writes goes through `internal/ops`: model edits with `hclwrite` (never text
  splicing or re-encoding a whole file), layout files regenerated in canonical form, multi-file
  writes atomic (temporary files, then rename), every operation with its inverse for undo. A file
  matched by `read_only` is never written; the operation fails naming the file.
- Everything but `cmd/` lives under `internal/`, except `packs/generic`, `web/`, `vscode/`,
  `testdata/` and `examples/` (spec 12).

## Determinism
- The same files produce byte-identical layout writes and exports (spec 11). In Go, never let map
  iteration order reach output: sort the keys. In TypeScript, no `Math.random`, `Date.now` or
  insertion-order-dependent iteration in geometry, placement or routing; unpinned nodes are
  placed in sorted ID order.
- Layout coordinates are integers on the project grid, relative to the node's frame.

## Go
- Before every commit: `gofmt -l`, `go vet ./...`, `staticcheck ./...`, `govulncheck ./...` all
  clean. Fix the code rather than suppressing a finding; when a `//lint:ignore` is right, give its
  reason.
- A corrupt file, bad reference or failed write is a diagnostic or an error to return, with file,
  line and column when there is one. Panic only on a broken invariant of the program's own state.
- Tests are table-driven over real inputs, with no mocks. `hclwrite` edits, layout output and
  resolved views are golden files under `testdata/`. Fuzz every decoder: HCL loading, layout
  YAML, WebSocket messages, `dg op` arguments, MCP and LSP requests.
- `dg check` must stay under 1 second at 1,000 nodes; keep a benchmark for it.

## TypeScript
- `tsc --noEmit` clean with `strict` on.
- `web/src/geometry`, `place` and `route` are pure functions of the resolved view and measured
  text; keep JointJS and the DOM in `canvas/`. Test the pure parts without a browser.
- Dragging stays local; only the drop sends an operation (FR-11). Dragging 200 nodes and 400
  edges must hold 60 frames per second.
- No network at runtime: fonts and WebAssembly are bundled.

## Comments
- Default to no comments. Write code whose names and structure make intent clear.
- Only comment a constraint the code cannot express: a workaround for an external bug,
  a non-obvious invariant, a reason something that looks wrong is intentional.
- Never comment what the next line does, never leave "changed X to Y" or "added for task Z"
  notes. That history goes in the commit message.
- Exported identifiers get a one-line doc comment that starts with their name; nothing more.
- When editing existing code, don't add comments to lines you didn't need to change.

## Documents
- `docs/spec.md` holds every decision and why. Section 15 lists rejected options: don't reopen
  one unless the user asks. When the code must differ from the spec, change the spec in the same
  change.
- Docs and code state current facts, arranged so they are easiest to read and understand.
  Git history keeps the past; don't preserve it in the text. A reversed decision is rewritten
  as the new decision.
- When a doc needs updating, consider rewriting the whole section instead of patching it or
  appending a block. Appended blocks drift into a changelog.
- Before adding content, check for duplication elsewhere in the doc. Merge or remove duplicates,
  and rewrite when that gives a clearer result.
- Each milestone ends with example projects in `examples/` for the diagram types it enables.

## Dependencies and tech debt
- Use the libraries in spec 3.3. A new dependency needs a reason a few lines of code can't
  cover, a license that allows embedding in the binary (no license keys), and a row in that table.
- `docs/tech-debts.md` lists every workaround, version pin and pre-release dependency, each with
  why it exists and what must be true to remove it. Create it with the first entry.
- When you add one, add its entry in the same change. When you touch code or a dependency an
  entry covers, check whether its exit condition is met.
- When an entry is resolved, remove the workaround and delete the entry. Don't mark it done or
  move it to a "resolved" list; git history keeps that.

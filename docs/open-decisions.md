# Open decisions

Questions that do not block the current milestone. Each entry: the question, the options, the
default the code uses now, and the milestone that raised it. Delete an entry once decided (record
the decision in `docs/spec.md`).

## Commit the built web app?

- **Question:** should `web/dist` (the Vite output that `web/embed.go` embeds) be committed, so
  `go install github.com/nvcnvn/dg/cmd/dg@latest` works without Node?
- **Options:** (a) not committed; `make web` builds it before any `go` command; (b) committed and
  checked in CI to match a fresh build; (c) committed only on release tags.
- **Default:** (a). A clean checkout needs Node and `make`.
- **Milestone:** M1.

## Embed the font in exported SVG?

- **Question:** an exported SVG names `Inter, sans-serif` but does not contain the font, so a viewer
  without Inter draws other glyph widths than the ones the layout was measured with.
- **Options:** (a) name only; (b) embed the used Inter subsets as base64 `@font-face` (about 30 KB
  per subset); (c) subset the font to the characters used.
- **Default:** (a).
- **Milestone:** M1.

## Characters Inter does not cover

- **Question:** labels with characters outside Inter's subsets (CJK, emoji, other scripts) are
  measured and drawn with a system fallback font, a known exception to spec 3.4, so their
  geometry depends on the machine.
- **Options:** (a) accept the fallback; (b) bundle fallback fonts such as Noto Sans CJK and Noto
  Emoji (large); (c) report a diagnostic for uncovered characters.
- **Default:** (a).
- **Milestone:** M1; revisited in M3.

## Third-party notices and LGPL relinking

- **Question:** the binary embeds Inter (OFL-1.1, notice required), libavoid (LGPL-2.1+: users must
  be able to replace it), JointJS (MPL-2.0) and elkjs (EPL-2.0), and links the MCP Go SDK
  (Apache-2.0 and MIT, notices required). How does the binary carry
  notices, and how does a user swap libavoid?
- **Options:** (a) a `dg licenses` command plus a flag or environment variable naming a
  directory with a replacement `libavoid.wasm` and its glue `libavoid-*.js`; (b) notices in
  release archives only; (c) both.
- **Default:** none yet; nothing is shipped. The build keeps libavoid replaceable: `libavoid.wasm`
  and the libavoid-js glue are separate files in `web/dist/assets` (`web/vite.config.ts`).
  Decide before the first release.
- **Milestone:** M1.

## Color syntax in styles

- **Question:** which color values do `fill` and `stroke` accept?
- **Options:** (a) `#RGB`, `#RRGGBB` and `none`; (b) also CSS named colors; (c) any CSS color.
- **Default:** (a).
- **Milestone:** M1.

## Layout coordinates off the grid

- **Question:** a hand-edited layout file can hold `x`, `y`, `w` or `h` that are not multiples of
  the project `grid` (or become so when `grid` changes). Should `dg check` warn?
- **Options:** (a) no diagnostic; (b) a warning; (c) `dg fmt` snaps them to the grid.
- **Default:** (a).
- **Milestone:** M2; revisit with layout editing (M10).

## Font name of a project font in exported SVG

- **Question:** an exported SVG names a project font by its file's base name
  (`fonts/Brand.woff2` is `Brand, sans-serif`), which may differ from the family name inside the
  font, so a viewer that has the font installed may not find it.
- **Options:** (a) the file's base name; (b) the family name read from the font's `name` table
  (needs a TrueType/WOFF/WOFF2 reader); (c) a `font_family` field in `dg.hcl`.
- **Default:** (a).
- **Milestone:** M3.

## Gallery views of unconnected nodes

- **Question:** elk lays out nodes without edges in one row, so a view of many unconnected nodes
  (such as `examples/shapes`) is very wide.
- **Options:** (a) leave it; (b) pack disconnected components with elk's `elk.separateConnectedComponents`
  and an aspect ratio; (c) a grid for nodes without edges.
- **Default:** (a).
- **Milestone:** M3.

## Dragging at 60 frames per second

- **Question:** M3 routes a whole view with a new libavoid router per draw, about 3 seconds for
  a dense 200-node, 400-edge view. Dragging at 60 frames per second (FR-8, M10) cannot reroute
  that way.
- **Options:** (a) a persistent router: keep libavoid's shapes and connectors between frames,
  `moveShape` the dragged nodes and `processTransaction`, rerouting only their edges, off the main
  thread if needed; draw a cheap preview (straight or last routes) while a frame would miss its
  budget, and route fully on drop; (b) route only on drop; (c) a simpler router while dragging.
- **Default:** (a) recommended. The `Router` type (`web/src/route/edges.ts`) takes the whole
  obstacle and request set, which a persistent router can diff against its state.
- **Milestone:** M3; decide in M10.

## Layout direction of a view

- **Question:** elk lays out layers top to bottom, or left to right when an edge ends at a row,
  and gives an edge with `triangle-open` at one end only from that end, so the parent is drawn
  above (spec 7.4). Keying the reversal to the marker is a heuristic. Should a view be able to
  choose its direction?
- **Options:** (a) the rules only; (b) a `direction` attribute on `view` in `views.hcl` for the
  first layout, defaulting to the rules; (c) a style property of the edge kinds.
- **Default:** (a) now; (b) recommended, with the rules as its default.
- **Milestone:** M4; decide in M7 (views).

## Self-loops as routing obstacles

- **Question:** a self-loop drawn around a corner (spec 7.5) is not known to libavoid, so another
  edge can be routed across it.
- **Options:** (a) leave it; (b) add each loop's bounding box as an obstacle and route the other
  edges after the loops; (c) route self-loops with libavoid between two stub points.
- **Default:** (a). Loops reach two grid units out (three with the longest markers) and the placer
  keeps four grid units between nodes, so crossings need an edge squeezed between neighbors.
- **Milestone:** M4.

## Colors of GIF exports

- **Question:** a GIF uses one palette of the 255 colors most frequent across its frames, without
  dithering, so a drawing with many colors bands (about 30 fills put the worst channel error at
  47). Should the quantization be better?
- **Options:** (a) as it is; (b) per-frame palettes; (c) median-cut quantization; (d) ordered
  dithering.
- **Default:** (a).
- **Milestone:** M5.

## Undo after an external edit to the same file

- **Question:** an undo restores whole files, so it fails when a file it restores was changed
  outside the editor since the operation, even in an unrelated place (FR-10).
- **Options:** (a) fail, as now; (b) a three-way merge of the content before the operation, after
  it and now, failing only on a conflict; (c) an inverse operation per operation.
- **Default:** (a); (b) recommended.
- **Milestone:** M6; decide in M10.

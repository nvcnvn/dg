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

## Should the extension register dg mcp with VS Code?

- **Question:** VS Code starts the servers of its MCP list outside its terminals, so they need the
  command line installed, even though the extension holds a `dg`.
- **Options:** (a) leave it; (b) register `dg mcp` with `lm.registerMcpServerDefinitionProvider`,
  which needs VS Code 1.101 (the extension supports 1.91) and reaches only the agents that use
  that list.
- **Default:** (a).
- **Milestone:** M13.

## Embed the font in exported SVG?

- **Question:** an exported SVG names `Inter, sans-serif` but does not contain the font, so a viewer
  without Inter draws other glyph widths than the ones the layout was measured with. The pictures
  of the documentation site are such SVG files.
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

## Color syntax in styles

- **Question:** which color values do `fill` and `stroke` accept?
- **Options:** (a) `#RGB`, `#RRGGBB` and `none`; (b) also CSS named colors; (c) any CSS color.
- **Default:** (a).
- **Milestone:** M1.

## Layout coordinates off the grid

- **Question:** a hand-edited layout file can hold `x`, `y`, `w` or `h` that are not multiples of
  the project `grid` (or become so when `grid` changes). Should `dg check` warn?
- **Options:** (a) no diagnostic; (b) a warning; (c) `dg fmt` snaps them to the grid.
- **Default:** (a). The editor and operations write grid values, so only hand edits and a changed
  `grid` leave such values.
- **Milestone:** M2; revisited in M10.

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

## Overlay outline shape

- **Question:** an overlay is drawn as the convex hull of its padded member boxes (spec 5.8), which
  can cover nodes that are not members, such as a node between two members.
- **Options:** (a) convex hull, as now; (b) bubble sets: the padded member boxes joined by thin
  bridges routed around non-members; (c) the convex hull, with non-members inside it marked.
- **Default:** (a); (b) recommended.
- **Milestone:** M7; not blocking.

## Drawing a dense view after a drop

- **Question:** a drop sends one operation, and the new snapshot is drawn from scratch: placing,
  and routing every edge twice with libavoid on the main thread. At 200 nodes and 400 edges, all
  pinned, the editor is editable again 0.16 to 0.17 seconds after a drop of one node, with tasks
  of up to 95 to 130 milliseconds meanwhile (about 0.55 seconds after moving all 200 nodes, most
  of it applying 200 moves); with nothing pinned, 0.22 to 0.66 seconds, with tasks of about 160
  to 180 milliseconds. The moved nodes stay where they were dropped meanwhile.
- **Options:** (a) as now; (b) draw in a Web Worker (text measured with `OffscreenCanvas`, libavoid
  and elk in the worker); (c) after a drop, route again only the edges the operation can have
  changed and keep the other routes, the cheap fix if this ever matters.
- **Default:** (a). `TestDragPerformance` (`internal/server`) fails when one node's drop takes
  more than 0.5 seconds at 200 nodes and 400 edges.
- **Milestone:** M10.

## Undo of a change that created a file, edited since

- **Question:** the first drag in a view without a layout file creates the file. Once a text
  editor changes it, undoing the drag is refused, because the undo would remove a file holding
  someone else's edits (FR-10).
- **Options:** (a) refuse, as now; (b) treat a created file like any other, merging against an
  empty base, which leaves the outside edits in a file the undo would otherwise remove; (c) remove
  the created file only if it is unchanged, otherwise refuse with an explanation of what to do.
- **Default:** (a). Not blocking.
- **Milestone:** M10.

## Hiding an edge from a view

- **Question:** "Hide from view" hides nodes (`hide_node` edits `include` and `exclude`), but a view's
  edges are chosen by one `edges` filter with no `exclude`, so the editor offers no way to hide one
  edge.
- **Options:** (a) not offered; (b) rewrite `edges` to list every other edge the view shows,
  rejected: new edges would silently stop showing; (c) add an edge `exclude` (say `exclude_edges`)
  to views and a `hide_edge` operation, the eventual answer.
- **Default:** (a), until (c).
- **Milestone:** M11; post-v1.

## Copy and paste between windows

- **Question:** copied nodes stay in the page that copied them, so a paste in another browser tab or
  window of the same `dg serve`, or in a VS Code panel from a browser tab of the same project, or in
  another project, has nothing to paste.
- **Options:** (a) in the page only; (b) the system clipboard, through the `copy` and `paste`
  events, with the nodes as JSON under a dg media type, pasted into any project that has their kinds.
- **Default:** (a).
- **Milestone:** M11; early post-v1.

## Navigation of the documentation site

- **Question:** the site uses GitHub Pages' supported `minima` theme, whose header links four
  pages (Getting started, VS Code, Examples, Reference); the long reference pages start with a
  generated table of contents, and there is no sidebar or search.
- **Options:** (a) `minima` with header links and tables of contents, as now; (b) a documentation
  theme with a sidebar and search, such as Just the Docs, loaded with `remote_theme`, which the
  built-in Pages build supports without a workflow; it needs front matter (title, order) on each
  page, which GitHub's file view shows as a table; (c) a static site generator built by a Pages
  workflow.
- **Default:** (a); (b) is the likely choice once the reference outgrows the header.
- **Milestone:** M12.

## Versions of the documentation

- **Question:** the site follows `main`, so once there are releases it may describe features a
  released binary does not have.
- **Options:** (a) one site that follows `main`; (b) publish from a release branch or tag; (c)
  versioned copies of the site per release.
- **Default:** (a), while there are no releases.
- **Milestone:** first release.

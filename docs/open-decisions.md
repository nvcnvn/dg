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

- **Question:** labels with characters outside Inter's subsets (for example CJK) are measured and
  drawn with a system fallback font, against spec 3.4.
- **Options:** (a) accept the fallback; (b) bundle a second font such as Noto Sans CJK (large);
  (c) report a diagnostic for uncovered characters.
- **Default:** (a).
- **Milestone:** M1.

## Third-party notices and LGPL relinking

- **Question:** the binary embeds Inter (OFL-1.1, notice required), libavoid.wasm (LGPL-2.1+, users
  must be able to replace it), JointJS and router-avoid (MPL-2.0) and elkjs (EPL-2.0). How does the
  binary carry notices, and how does a user swap `libavoid.wasm`?
- **Options:** (a) a `dg licenses` command plus a flag or environment variable naming a
  replacement `libavoid.wasm`; (b) notices in release archives only; (c) both.
- **Default:** none yet; nothing is shipped. Decide before the first release.
- **Milestone:** M1.

## Color syntax in styles

- **Question:** which color values do `fill` and `stroke` accept?
- **Options:** (a) `#RGB`, `#RRGGBB` and `none`; (b) also CSS named colors; (c) any CSS color.
- **Default:** (a).
- **Milestone:** M1.

## One drawing path for export and editor

- **Question:** export writes SVG from computed geometry (FR-14), while the editor (M10) draws with
  JointJS. How do the two stay identical?
- **Options:** (a) node and edge SVG markup comes from shared pure functions
  (`web/src/canvas/markup.ts`) that both the export writer and the JointJS canvas use, so there is
  one drawing path; (b) the editor draws with JointJS shapes and export serializes the JointJS DOM;
  (c) two renderers kept equal by tests.
- **Default:** (a) recommended; the M1 code keeps markup apart from the export writer so that it is
  possible.
- **Milestone:** M1; decide by M10.

## Edge label and edge collisions

- **Question:** mid labels sit at the route's midpoint and can overlap other edges, and parallel
  arrowheads can crowd each other.
- **Options:** (a) leave it; (b) shift a label along its route away from other routes and boxes;
  (c) pass labels to libavoid or elk as obstacles.
- **Default:** (a); libavoid's nudging distance is 20 pixels so parallel arrowheads (10 wide) do not
  touch.
- **Milestone:** M1; revisit in M3 with label wrapping.

## PNG pictures through `file()`

- **Question:** spec 4.3 had `icon` and `image` take an SVG or a PNG through `file("…")`, but
  `file()` returns text, and a PNG is not UTF-8.
- **Options:** (a) SVG only; (b) `file()` returns a `data:` URI for non-text files; (c) a second
  function such as `image("…")` that returns a `data:` URI for SVG and PNG alike.
- **Default:** (a); `file()` fails on a file that is not UTF-8 text.
- **Milestone:** M2; decide by M3, which draws icons.

## Layout coordinates off the grid

- **Question:** a hand-edited layout file can hold `x`, `y`, `w` or `h` that are not multiples of
  the project `grid` (or become so when `grid` changes). Should `dg check` warn?
- **Options:** (a) no diagnostic; (b) a warning; (c) `dg fmt` snaps them to the grid.
- **Default:** (a).
- **Milestone:** M2; revisit with layout editing (M10).

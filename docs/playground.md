# Playground

The [playground](https://nvcnvn.github.io/dg/playground/) runs dg in your browser with nothing to
install: write a model and a style on the left, and arrange the diagram on the right. It is the
same editor as `dg serve`, with every gesture, and what you do in one half shows in the other.

- **Examples** opens `hello`, `class` or `er`.
- **Share** copies a link that holds your files in its fragment, the part of a URL that never
  reaches a server. A link over 8,000 characters may be cut by some chat apps.
- **Export** downloads the diagram as SVG or PNG, or the project as a `.zip` that `dg check` and
  `dg serve` open.

Your last project stays in this browser. Nothing leaves it: after the page loads it uses no
network, and it writes only to memory and to the files you download.

A playground project is one view and the kinds of the generic pack: `model.hcl`, `style.hcl` and a
layout that the canvas writes. Packs, more views and your own kinds need the
[VS Code extension or the command line](getting-started.md).

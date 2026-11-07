# Playground

The [playground](https://nvcnvn.github.io/dg/playground/) runs dg in your browser with nothing to
install: write a model and a style on the left, and arrange the diagram on the right. It is the
same editor as `dg serve`, with every gesture, and what you do in one half shows in the other.

- **Examples** opens `hello`, `class` or `er`.
- **Open** opens a project `.zip` you downloaded, with your arrangement as you left it.
- **Share** copies a link that holds your files in its fragment, the part of a URL that never
  reaches a server. A link over 8,000 characters may be cut by some chat apps.
- **Export** downloads the diagram as SVG or PNG. To keep a project, arrangement included, choose
  **Download project (.zip)**: **Open** brings it back here, and `dg check` and `dg serve` open it
  once unzipped.

Your last project stays in this browser. Nothing leaves it: after the page loads it uses no
network, and it writes only to memory and to the files you download.

A playground project uses the kinds of the generic pack: you edit `model.hcl` and `style.hcl`, and
the canvas writes the layout, and any views you add, out of sight. Packs, your own kinds and
editing views as text need the [VS Code extension or the command line](getting-started.md).

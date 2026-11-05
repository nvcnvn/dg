# The VS Code extension

The extension brings dg's language server and its diagram editor into VS Code: errors as you
type, completion, go to definition, rename and formatting in the HCL files, and the diagram in a
panel beside them that you arrange and edit as in the browser.

## Install

Install [dg Diagrams](https://marketplace.visualstudio.com/items?itemName=nvcnvn.dg-diagrams) from
the Marketplace: search for it in the Extensions view, or use VS Code's command line:

```sh
code --install-extension nvcnvn.dg-diagrams
```

The Marketplace gives you the package for your platform. Each package holds its own `dg` binary,
so the extension needs nothing else on the machine, and it never uses the network. To use dg
outside the extension as well, see [The command line and agents](#the-command-line-and-agents).

To build a package yourself, set up a clone of the repository as in
[Getting started](getting-started.md#build-from-source) and install what it builds:

```sh
make vsix
code --install-extension bin/dg-darwin-arm64.vsix
```

`make vsix` writes `bin/dg-<platform>.vsix` for the machine it runs on: `darwin-arm64`,
`darwin-x64`, `linux-arm64`, `linux-x64`, `win32-arm64` or `win32-x64`. Use the file it wrote in
the second command. `make vsix-linux-x64` and the like build a package for another platform, and
`make vsix-all` builds them all. `code` is VS Code's command line; you can also install the file
from the Extensions view's "Install from VSIX…" menu item.

## Which files it handles

A folder is a dg project when it holds `dg.hcl`, or when the setting `dg.projectFolders` lists it.
When a `.hcl` file of a project opens as plain text or as HCL, the extension gives it the language
dg, highlighted with HashiCorp's HCL grammar, and starts `dg lsp` for the project if it is not
running yet. Every other file keeps its language, and dg's language server never sees it: `.hcl`
files outside projects, and files of a project that another extension gave a language of its own,
such as a Terraform test (`.tftest.hcl`) or a Packer template (`.pkr.hcl`).

- Projects are found at any depth in the workspace's folders, so you can open a project's folder
  or a repository that holds several, such as the repository of dg, whose `examples/` each have
  a `dg.hcl`. A file belongs to the nearest project above it, as `dg -C` would load it: the
  files of a project inside another go to the inner project's server only.
- A project without `dg.hcl`, which the command line allows, is listed in the workspace's or the
  folder's settings, by its path from the workspace folder; on macOS and Windows the letter case
  of the path does not matter:

  ```json
  { "dg.projectFolders": ["diagrams", "docs/architecture"] }
  ```

- When a folder stops being a project, because its `dg.hcl` is deleted or the setting no longer
  lists it, its server stops and its open files get back the language they had.
- To read a file of a project as another language, pick it with **Change Language Mode**. The
  extension does not change it back while VS Code keeps the file loaded, which it does after its
  editor closes too; once VS Code is reloaded, the file opens as dg again.

The extension starts when the workspace holds a `dg.hcl`, and otherwise once VS Code has
started, so that it finds the folders of `dg.projectFolders`.

## Writing HCL

The language server works on the text in your editors, saved or not, and reloads the project
after each change:

- **Diagnostics**: the errors and warnings of `dg check`, at the same places, for every file of
  the project, layout files included. A problem that belongs to no file is shown at the start of
  `dg.hcl`.
- **Completion**: kinds after `kind =` and `extends =`, edge kinds, node IDs after `node.`, ports
  after `.port.`, rows after `.row.`, selectors in views, style labels, style property names, and
  the values of properties that take a few (shapes, markers, `true` and `false`). It works while
  the file does not parse, from the project as last loaded.
- **Hover** on a reference to a node shows its kind and label.
- **Go to Definition** and **Find All References** for node, edge, kind, edge kind and view IDs,
  across model files, `views.hcl`, styles and layout files.
- **Rename Symbol** (F2) on a node, edge or view ID renames it everywhere, as
  [`rename_id`](reference/operations.md#rename_id) does: every reference, every layout file, and
  the layout file's name for a view. It is refused when the ID or a reference is in a
  [`read_only`](reference/language.md#the-project-file) file.
- **Format Document** formats a file as `dg fmt` does.

## The diagram panel

Click the **Open diagram** button at the top right of a dg file's editor, as you open Markdown's
preview, or run **dg: Open diagram** from the Command Palette; it is also in the context menu of
the editor's tab and of `dg.hcl` in the Explorer. A panel opens beside the text with the diagram of
that file's project (from the Command Palette, of the file you are editing, else of the first
project in the workspace). One panel is open at a time: opening another project's diagram
replaces it. The button shows only for dg files, so Terraform and other HCL files never have
it. It is the same editor as `dg serve`, with every [gesture](reference/editor.md): drag to arrange,
switch views, create and connect nodes, edit labels and the inspector.

Everything the panel changes is written to the project's files, which the text editors show at
once; Ctrl+Z (⌘Z) in the panel undoes it. Saving a file in a text editor redraws the panel. Click
in the panel before using its keyboard shortcuts, so they reach the diagram rather than VS Code.

The panel follows VS Code's theme. A click on a diagnostic in the panel's diagnostics bar opens its
file at its position. The palette and inspector start hidden in a narrow panel; the toolbar's
Palette and Inspector buttons show them. Closing the panel stops its server.

## The command line and agents

The extension runs the `dg` in its package and no other, so a `dg` you installed yourself never
changes what the panel does. It also puts that `dg` first on the `PATH` of VS Code's terminals.
There, you and any agent you start get the same version as the panel, with nothing to install:

```sh
dg version
dg export --format svg --out hello.svg
```

A terminal that was open when the extension was installed or updated keeps its old `PATH` until you
restart it; VS Code marks it. Agents reach the project with `dg` commands or
[`dg mcp`](reference/cli.md#dg-mcp), while you keep the panel open: what they write appears in your
text editors and the panel redraws, as when you save a file yourself.

Everything that starts outside those terminals needs the
[command line](getting-started.md#command-line) installed: your system's terminal, CI, and MCP
clients that start `dg` themselves, which includes VS Code's own MCP servers.

```sh
curl -fsSL https://raw.githubusercontent.com/nvcnvn/dg/main/install.sh | bash
```

That is a second copy, which you update by running the script again. If it falls behind, a project
that uses a newer feature is an error in that copy: the error says which version it is and to
update it.

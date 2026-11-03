# Command line

<!-- begin generated dg: go test ./cmd/dg -run TestDocs -update -->

```text
usage: dg <command> [flags]

commands:
  check       validate the project
  fmt         format the project's files
  export      draw a view, or every view, as SVG, PNG, GIF or JSON geometry
  serve       show the views in the browser, redrawn when a file changes
  vocabulary  print the kinds, edge kinds, tags and views the project defines, as JSON
  op          apply an operation to the project's files (dg op --help lists them)
  mcp         serve the Model Context Protocol over standard input and output
  lsp         serve the Language Server Protocol over standard input and output
```

<!-- end dg -->

Every command takes `-C <dir>`, the project directory (default: the current directory). Flags
take one dash or two: `-json` and `--json` are the same.

Exit codes are the same for every command: 0 when it succeeded, 1 when the project has errors or
the input is invalid (an operation refused, a flag wrong), and 2 when dg itself failed, such as an
unreadable directory or a browser that does not start.

## dg check

<!-- begin generated dg-check: go test ./cmd/dg -run TestDocs -update -->

| Flag | Value | Meaning |
|---|---|---|
| `-C` | string | project directory (default ".") |
| `--json` |  | print diagnostics as JSON |
| `--strict` |  | treat warnings as errors |

<!-- end dg-check -->

Validates the whole project and prints one line per diagnostic, sorted by file, line and column:

```text
model.hcl:13:12: error: Undefined node: Parent "backnd" of node "api" is not defined.
```

Severities are `error`, `warning` and `info`. `--strict` fails on warnings too, never on
information. `--json` prints a JSON array of objects with `severity`, `summary`, `detail` and,
when there is a position, `file`, `line`, `column`, `end_line` and `end_column`. `file` is relative
to the project; diagnostics about the built-in pack name `<generic>/spec.hcl` or
`<generic>/style.hcl`.

## dg fmt

<!-- begin generated dg-fmt: go test ./cmd/dg -run TestDocs -update -->

| Flag | Value | Meaning |
|---|---|---|
| `-C` | string | project directory (default ".") |
| `--check` |  | list the files that would change and write nothing |

<!-- end dg-fmt -->

Formats every writable HCL file the project loads in HCL's canonical style, and rewrites layout
files in their canonical form, all files together. It prints the path of each file it changed.
`read_only` files are skipped, files that do not parse are reported and left alone, and nothing is
written while `dg.hcl` has errors. `--check` lists the files that would change, writes nothing, and
exits with 1 if there are any.

## dg export

<!-- begin generated dg-export: go test ./cmd/dg -run TestDocs -update -->

| Flag | Value | Meaning |
|---|---|---|
| `-C` | string | project directory (default ".") |
| `--all` |  | export every view into the --out directory, one &lt;view&gt;.&lt;format&gt; file each |
| `--format` | string | output format: svg, png, gif or json (default "svg") |
| `--out` | string | output file, or directory with --all (default standard output) |
| `--view` | string | view to export (default "main") |

<!-- end dg-export -->

Draws a view in headless Chrome or Chromium, which must be installed, exactly as the editor draws
it, without the editor's selection, fading or drill-down badges.

| Format | Output |
|---|---|
| `svg` | an SVG file; animated edges keep their animation as CSS. Text names the font (`Inter, sans-serif`) without embedding it |
| `png` | a picture on white, one pixel per unit, animations at their start |
| `gif` | one half-second animation period in 10 frames, looping |
| `json` | the geometry: each node's box, port positions and row lines, each edge's route, in canvas coordinates; and each node's `frame` and its position `rel` in it, as [`move`](operations.md#move) takes them, or `ordered_in`, the stack or grid that places it |

```sh
dg export -C examples/hello --format svg --out hello.svg
dg export -C examples/c4-container --view sign-in --format gif --out sign-in.gif
dg export -C examples/c4 --all --format png --out pictures
```

`--all` writes one `<view>.<format>` file per view into the `--out` directory, and nothing unless
every view can be drawn. Without `--out`, the picture goes to standard output. Exporting is
deterministic: the same files give the same bytes.

## dg serve

<!-- begin generated dg-serve: go test ./cmd/dg -run TestDocs -update -->

| Flag | Value | Meaning |
|---|---|---|
| `-C` | string | project directory (default ".") |
| `--addr` | string | address to listen on (default "127.0.0.1:0") |
| `--open` |  | open the editor in the browser (default true) |
| `--stdin` |  | stop when standard input closes, as when the editor that started dg serve exits |

<!-- end dg-serve -->

Starts the [editor](editor.md) on a local address, prints it (`dg serve: http://127.0.0.1:…/`) and
opens it in the browser. The editor redraws within a fraction of a second when a project file
changes on disk. While the project has errors it lists them and keeps showing the last good
drawing, read-only. Stop it with Ctrl+C.

`dg serve` listens only on the loopback address and has no authentication. Several browser
windows can edit one project; they share one undo history.

## dg op

Applies one [operation](operations.md) to the project's files: the same changes the editor makes,
for scripts and agents. `dg op --help` lists the operations and `dg op <operation> --help` shows
one's flags.

## dg vocabulary

<!-- begin generated dg-vocabulary: go test ./cmd/dg -run TestDocs -update -->

| Flag | Value | Meaning |
|---|---|---|
| `-C` | string | project directory (default ".") |

<!-- end dg-vocabulary -->

Prints, as JSON, what a project can be built from: its kinds (with their ports, properties and
whether they are containers or hold rows, inherited ones included), edge kinds, tags and views.
Agents read it before writing a model.

## dg mcp

<!-- begin generated dg-mcp: go test ./cmd/dg -run TestDocs -update -->

| Flag | Value | Meaning |
|---|---|---|
| `-C` | string | project directory (default ".") |

<!-- end dg-mcp -->

Serves the [Model Context Protocol](https://modelcontextprotocol.io) over standard input and
output, for agents. It has four tools, each giving the same result as its command:

| Tool | Does |
|---|---|
| `check` | the diagnostics, as `dg check --json` |
| `vocabulary` | the kinds, edge kinds, tags and views, as `dg vocabulary` |
| `op` | applies an [operation](operations.md) given as a JSON object, `batch` included; the result is `dg op --json`'s, an error result when refused |
| `export` | draws a view: `{view?, format?}`; PNG (the default) and GIF are returned as images the agent can see, SVG and JSON as text |

To use it from an MCP client, run `dg mcp -C /path/to/project` as a stdio server.

## dg lsp

<!-- begin generated dg-lsp: go test ./cmd/dg -run TestDocs -update -->

| Flag | Value | Meaning |
|---|---|---|
| `-C` | string | project directory (default ".") |

<!-- end dg-lsp -->

Serves the Language Server Protocol over standard input and output for the project's HCL files,
working on the editor's unsaved text: diagnostics, completion, hover, go to definition, find
references, rename and formatting. The [VS Code extension](../vscode.md) runs it; other editors
can run `dg lsp -C <project>` for `.hcl` files.

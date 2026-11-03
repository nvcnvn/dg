# Development

[`docs/spec.md`](spec.md) is the design and the source of truth: its section 13 is the
build order and section 15 the options already rejected. [`docs/open-decisions.md`](https://github.com/nvcnvn/dg/blob/main/docs/open-decisions.md)
lists the questions still open, each with the default the code uses, and
[`docs/tech-debts.md`](https://github.com/nvcnvn/dg/blob/main/docs/tech-debts.md) every workaround and version pin with what must be
true to remove it.

## Build

Needs Go, Node with npm, and Google Chrome or Chromium (for `dg export` and its tests).

The Go packages embed the web app from `web/dist`, which is built output and not committed, so
build the web app before any `go` command:

```sh
make web     # npm ci and vite build into web/dist
make build   # bin/dg
make test    # go test ./... and the web unit tests
make lint    # gofmt, go vet, staticcheck, govulncheck, tsc --noEmit (web app and extension)
```

After `make web`, plain `go build ./...` and `go test ./...` work. `go test -short ./...` skips the
tests that start Chromium.

## Tests and golden files

Golden files are rewritten with `-update`:

- `go test ./cmd/dg ./internal/export ./internal/view -update`: diagnostics of the projects
  under `testdata/check`, `dg fmt` output in `testdata/fmt/want`, and the resolved views and
  exports of every view of every example in `testdata/export` (PNG and GIF exports are not
  golden: the tests compare two runs). `testdata/lift` has a view for each case of edge lifting
  and merging; its resolved views are goldens like the examples'.
- `go test ./internal/ops -update`: the operation goldens in `testdata/ops/want` (the files each
  operation writes to a copy of `testdata/ops/in`) and `web/src/protocol/ops.ts`, the web app's
  types of the operations, which a test keeps equal to their definitions.
- `go test ./internal/lsp -update`: the language server's recorded sessions. Each
  `testdata/lsp/*.jsonl` names a project on its first line and holds the client's messages, and
  its `.golden` file what the server answered.

Benchmarks and performance tests: `go test ./internal/project -bench Load1000` times `dg check`
at 1,000 nodes, `go test ./internal/lsp -bench Keystroke` one keystroke in the language server at
1,000 nodes, and `go test ./internal/server -run DragPerformance -v` reads a Chromium performance
trace of drags in a view of 200 nodes and 400 edges (frames per second, longest task).

Fuzz targets: `FuzzLoad` (`internal/project`), `FuzzDecodeLayout` (`internal/view`),
`FuzzDecodeMessage` (`internal/server`, the WebSocket protocol), `FuzzOp` (`internal/ops`,
operations as JSON), `FuzzDiff` and `FuzzMerge3` (`internal/ops`, the line diff and merge of
undo), `FuzzOpArgs` (`cmd/dg`, `dg op` command lines), `FuzzTools` (`internal/mcp`, MCP tool
calls), and `FuzzMessage` and `FuzzBuffer` (`internal/lsp`, any message, and every request at any
position of any buffer).

The scripts in `testdata/rebuild` rebuild `examples/c4-container`, `examples/c4`,
`examples/incident`, `examples/swimlane`, `examples/flowchart` and `examples/state-machine` with
operations alone.

## VS Code extension

`vscode/` is the extension. It bundles `dg`, so each package is built for one platform:

```sh
make vsix                 # bin/dg-<this platform>.vsix, such as bin/dg-darwin-arm64.vsix
make vsix-linux-x64       # one target: darwin-arm64, darwin-x64, linux-arm64, linux-x64,
make vsix-all             #   win32-arm64 or win32-x64; or all of them
make test-vscode          # starts VS Code on a copy of examples/c4 with the extension from vscode/
make test-vsix            # the same test on the package for this platform, installed
make notices              # regenerate THIRD_PARTY_NOTICES after a dependency changes
```

Each package holds `LICENSE` and `THIRD_PARTY_NOTICES`, the licenses of the Go modules, npm
packages and grammar that the binary and the extension hold; `make lint` fails while the file is
out of date.

The tests use VS Code Insiders where macOS installs it, else the VS Code that `DG_VSCODE` names,
else a build `@vscode/test-electron` downloads.

## Documentation

The site at <https://nvcnvn.github.io/dg/> is GitHub Pages' built-in Jekyll build of the
repository's root on `main`, with the `minima` theme: `README.md` is the home page and `docs/`
holds the pages. `_config.yml` sets the theme and the header links, and excludes everything
else: source code, the examples, test data other than the pictures, `open-decisions.md` and
`tech-debts.md`. Pages link to each other with relative links to Markdown files, which work on
GitHub and on the site, and to source files with links to their page on GitHub. Publishing is
enabled once, in the repository's Settings → Pages: Source "Deploy from a branch", branch `main`,
folder `/ (root)`. Every push to `main` then rebuilds the site.

The pictures are the export goldens in `testdata/export`, so the export test that keeps them equal
to a fresh `dg export` keeps the documentation's pictures current too. Tests in `cmd/dg` keep the
rest equal to the code:

- `TestDocs` regenerates the sections between `<!-- begin generated … -->` and `<!-- end … -->`
  comments: the tables of contents, the operations and their flags, the commands' flags, the
  generic pack, and the layout file the getting-started guide shows, made by applying its first
  drag to `examples/hello` at the positions `testdata/export/hello.json` gives. It checks that
  the hand-written style property tables (`<!-- begin checked … -->`) list every property with
  the values it takes. `go test ./cmd/dg -run TestDocs -update` rewrites the generated sections.
- `TestGettingStarted` runs the getting-started guide from "Write a model" on in a new directory:
  it saves the files the guide shows and runs its commands other than `dg serve` (`dg export`
  needs Chromium, so `-short` skips it), and compares their output with the output the guide
  shows.
- `TestDocCommands` checks that every `dg` command in the documentation's code names an existing
  operation and existing flags.
- `TestDocExamples` checks that every HCL block of the README, the getting-started guide and the
  examples page is copied from a file of `examples/`.
- `TestDocLinks` checks that every relative link and image names a Markdown page or SVG picture
  the site publishes, every link to a file on GitHub a file of the repository, and every anchor a
  heading.
- `TestDocSite` checks that, with the exclude list of `_config.yml`, the site publishes only
  `README.md`, the pages of `docs/` and the SVG files of `testdata/export`.

To preview the site you need Ruby and Bundler. Install the `github-pages` gem in a directory
outside the repository and build from there, since Jekyll loads the plugins of the `Gemfile` in
its working directory:

```sh
mkdir -p ~/dg-pages && cd ~/dg-pages
printf 'source "https://rubygems.org"\ngem "github-pages", group: :jekyll_plugins\ngem "webrick"\n' > Gemfile
bundle config set --local path vendor/bundle && bundle install
bundle exec jekyll serve --source /path/to/dg --destination _site
```

The site is then at <http://127.0.0.1:4000/>.

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
make playground  # the playground site into dist/playground
make test    # go test ./... and the web unit tests, on a fresh playground
make lint    # gofmt, go vet, staticcheck, govulncheck, tsc --noEmit (web app and extension)
```

After `make web`, plain `go build ./...` and `go test ./...` work; the tests of
`internal/playground` that open the built site run after `make playground`, which `make test`
does, and skip themselves without it. `go test -short ./...` skips the
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

The editor's look is checked by tasks that find controls by their accessible names, contrast
computed from `web/src/theme.css`, axe-core and screenshots a person looks at, which are not
goldens: `DG_SCREENSHOTS=<dir> go test ./internal/server -run Screenshots` writes each example and
editor state, in both themes at both widths, to a directory that must exist.

Benchmarks and performance tests: `go test ./internal/project -bench Load1000` times `dg check`
at 1,000 nodes, `go test ./internal/lsp -bench Keystroke` one keystroke in the language server at
1,000 nodes, and `go test ./internal/server -run DragPerformance -v` reads a Chromium performance
trace of drags in a view of 200 nodes and 400 edges (frames per second, longest task); it skips
itself where the environment variable `CI` is set.

Fuzz targets: `FuzzLoad` (`internal/project`), `FuzzDecodeLayout` (`internal/view`),
`FuzzDecodeMessage` (`internal/session`, the messages of the editor), `FuzzOp` (`internal/ops`,
operations as JSON), `FuzzDiff` and `FuzzMerge3` (`internal/ops`, the line diff and merge of
undo), `FuzzOpArgs` (`cmd/dg`, `dg op` command lines), `FuzzTools` (`internal/mcp`, MCP tool
calls), and `FuzzMessage` and `FuzzBuffer` (`internal/lsp`, any message, and every request at any
position of any buffer).

`TestInstallScript` (`cmd/dg`) pipes `install.sh` to bash against a local server holding fake
releases: the latest, a pinned version, replacing a copy, and the refusals, which leave any
installed `dg` as it was. It runs on every platform of CI, without the network.
`.github/workflows/install.yml` installs the real release from GitHub on each platform, runs
ShellCheck and starts `dg mcp`, when `install.sh` or that workflow changes and after each release.

The scripts in `testdata/rebuild` rebuild `examples/c4-container`, `examples/c4`,
`examples/incident`, `examples/swimlane`, `examples/flowchart` and `examples/state-machine` with
operations alone.

## VS Code extension

`vscode/` is the extension. It bundles `dg`, so each package is built for one platform:

```sh
make vsix                 # bin/dg-<this platform>.vsix, such as bin/dg-darwin-arm64.vsix
make vsix-linux-x64       # one target: darwin-arm64, darwin-x64, linux-arm64, linux-x64,
make vsix-all             #   win32-arm64 or win32-x64; or all of them
make test-vscode          # starts VS Code on copies of examples with the extension from vscode/
make test-vsix            # the same test on the package for this platform, installed
make notices              # regenerate THIRD_PARTY_NOTICES after a dependency changes
```

The extension's icon, `vscode/images/icon.png`, is rendered from `icon.svg` beside it; after
changing the SVG, render it again with Chrome from the repository's root:

```sh
printf '<body style="margin:0"><img src="icon.svg" width="128" height="128" style="display:block">' > vscode/images/icon.html
google-chrome --headless --hide-scrollbars --default-background-color=00000000 \
  --window-size=128,128 --screenshot=vscode/images/icon.png vscode/images/icon.html
rm vscode/images/icon.html
```

Each package holds `LICENSE` and `THIRD_PARTY_NOTICES`, the licenses of the Go modules, npm
packages and grammar that the binary and the extension hold; `make lint` fails while the file is
out of date.

The tests use VS Code Insiders where macOS installs it, else the VS Code that `DG_VSCODE` names,
else a build `@vscode/test-electron` downloads.

## Replacing libavoid

libavoid (LGPL-2.1-or-later) reaches dg through the npm package `libavoid-js`, whose build scripts
compile it to WebAssembly with Emscripten in Docker. To build dg with a modified libavoid you need
Git, Docker, Python 3 and Node with npm:

1. Clone `libavoid-js` at the tag of the version `web/package-lock.json` names, and install its
   build tools:

   ```sh
   git clone -b v0.4.5 https://github.com/Aksem/libavoid-js
   cd libavoid-js
   npm ci
   ```

2. Run `python3 tools/generate.py` once. It clones libavoid's source, the adaptagrams fork at the
   tag its `ADAPTAGRAMS_VERSION` names, into `build/adaptagrams`, patches it, and compiles it.
3. Change libavoid in `build/adaptagrams/cola/libavoid`.
4. Run `python3 tools/generate.py` again, which compiles the changed source (it clones and patches
   only when `build/adaptagrams` is missing), then `npm run build`. `dist/libavoid.wasm` is the
   WebAssembly module and `dist/index.js` the JavaScript glue.
5. In dg, run `make web` once, so that `web/node_modules` exists, then copy the two files over
   `web/node_modules/libavoid-js/dist/libavoid.wasm` and `web/node_modules/libavoid-js/dist/index.js`.
6. Run `make build` for `bin/dg`, or `make vsix` for the extension's package.

The build keeps them files of their own (`web/vite.config.ts`): `web/dist/assets` gets the module
unchanged as `libavoid-<hash>.wasm` and the glue minified as `libavoid-<hash>.js`, and the binary
embeds both. `make` runs `npm ci` again, which puts back the published files, only after
`web/package.json` or `web/package-lock.json` changes; copy yours again then.

## Releasing

Push a tag `vX.Y.Z` on `main`, with the Marketplace's three numbers and no suffix:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

`.github/workflows/release.yml` then runs `make lint test` and `make release VERSION=v0.1.0` on
Linux, runs `dg check` and `dg export` from each archive on its own platform, publishes the
GitHub release, runs `install.sh` against it on each platform, and publishes to each channel below
that is set up. `make release` fills `dist/` with what the release holds, and works on any machine.
It links `VERSION` into each binary (`internal/version`), because the workflow's change to `vscode/package.json` would
make the Go toolchain stamp the version `+dirty`:

- `dg-<version>-<os>-<arch>.tar.gz` (`.zip` for Windows) for darwin, linux and windows on amd64
  and arm64, each holding `dg`, `LICENSE`, `THIRD_PARTY_NOTICES` and `README.md`
  (`make dist-<target>` builds one);
- `dg-<target>.vsix` for each VS Code target, with the version in `vscode/package.json`, which
  the workflow sets from the tag;
- `libavoid-source-….tar.gz`, libavoid's corresponding source: the adaptagrams fork at
  `ADAPTAGRAMS_TAG` (checked against `ADAPTAGRAMS_COMMIT`) and `libavoid-js` at the version
  `web/package-lock.json` names. Any `dg` binary or VSIX distributed to others must have this
  archive beside it (LGPL-2.1, section 6); the release is where the Marketplace, Open VSX and
  Homebrew copies have it;
- `SHA256SUMS`, and `dg.rb`, the formula of `.github/homebrew/dg.rb` with the version and
  checksums filled in, which the release does not hold.

When `libavoid-js` changes version, set `ADAPTAGRAMS_TAG` and `ADAPTAGRAMS_COMMIT` in the
`Makefile` to the tag its `tools/generate.py` clones, update the versions the README's Licensing
section names, and run `make notices`.

### Channels

The install script needs no setup: `install.sh` on `main` finds the latest release from GitHub's
redirect and reads the archive names and `SHA256SUMS` this section describes, so a change to either
must change the script too. Every other channel is set up once, in the repository's Settings →
Secrets and variables → Actions; the workflow skips a channel whose variable is unset.

**VS Code Marketplace** (variable `VS_MARKETPLACE` = `true`). The workflow runs `vsce publish
--oidc`, which trades GitHub's OIDC token for a Marketplace credential; there is no token to store
or renew.

1. Create the publisher `nvcnvn`, the `publisher` of `vscode/package.json`, at
   <https://marketplace.visualstudio.com/manage>, signed in with a Microsoft account.
2. On the publisher's Trusted Publishing tab, create a policy: repository owner `nvcnvn`,
   repository `dg`, workflow `release.yml`, environment `marketplace`.
3. Set the variable `VS_MARKETPLACE` to `true`.

**Open VSX** (variable `OPEN_VSX` = `true`), the registry of VSCodium, Cursor, Windsurf and
Gitpod. Sign in at <https://open-vsx.org> with GitHub, link an Eclipse account and sign the
publisher agreement, create an access token, store it as the secret `OVSX_PAT`, run
`npx ovsx create-namespace nvcnvn -p <token>` once, and claim ownership of the namespace. The
registry trusts a workflow only for an extension that already has a version, so after the first
release register it at <https://open-vsx.org/user-settings/trusted-publishers> (`nvcnvn`, `dg`,
`release.yml`, no environment) and delete the secret; `ovsx` then publishes with the workflow's
OIDC token.

**Homebrew** (variable `HOMEBREW_TAP` = `nvcnvn/homebrew-tap`). Create the public repository
`nvcnvn/homebrew-tap` and a fine-grained token with Contents read and write on it alone, stored
as the secret `HOMEBREW_TAP_TOKEN`. Each release commits `Formula/dg.rb` there, and users run
`brew install nvcnvn/tap/dg`.

## Documentation

The site at <https://nvcnvn.github.io/dg/> is built and deployed by
`.github/workflows/pages.yml` on every push to `main`: GitHub Pages' Jekyll build of the
repository's root, with the `minima` theme, where `README.md` is the home page and `docs/` holds
the pages, and the playground at `/playground/`, which `make playground` builds into
`dist/playground` (spec FR-19). `_config.yml` sets the theme, the plugins and the header links, and excludes
everything else: source code, the examples, test data other than the pictures,
`open-decisions.md` and `tech-debts.md`. `jekyll-sitemap` writes `/dg/sitemap.xml` from the
published pages; search engines get it through Google Search Console and Bing Webmaster Tools,
since a project site has no `robots.txt` of its own at the root of `nvcnvn.github.io`. Pages link
to each other with relative links to Markdown files, which work on GitHub and on the site, and to
source files with links to their page on GitHub. Publishing is enabled once, in the
repository's Settings → Pages: Source "GitHub Actions".

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
- `TestSkillExamples` runs the HCL, the batch and the `dg op` commands of the
  [agent skill](https://github.com/nvcnvn/dg/blob/main/skills/dg/SKILL.md) in order as one project,
  and `TestSkillOperations` checks that every operation it names exists.
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

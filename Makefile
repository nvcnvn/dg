# The Go packages embed web/dist, so build the web app before any go command.

.PHONY: all build web test lint test-vscode test-vsix vsix vsix-all notices

all: lint test build

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	@touch $@

vscode/node_modules: vscode/package.json vscode/package-lock.json
	cd vscode && npm ci
	@touch $@

web: web/node_modules
	cd web && npm run build

build: web
	go build -o bin/dg ./cmd/dg

test: web
	go test ./...
	cd web && npm test

lint: web vscode/node_modules
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	cd web && npm run typecheck
	cd vscode && npm run typecheck
	@$(NOTICES) | cmp -s - THIRD_PARTY_NOTICES || (echo "THIRD_PARTY_NOTICES is out of date: make notices"; exit 1)

# THIRD_PARTY_NOTICES holds the license of every component the dg binary and a VSIX hold: the Go
# standard library and the modules dg links, the npm packages of the web app and of the
# extension, and the HCL grammar. Regenerate it when dependencies change.
notices: web/node_modules vscode/node_modules
	@$(NOTICES) > THIRD_PARTY_NOTICES

# The adaptagrams tag from which tools/generate.py of the libavoid-js version in
# web/package-lock.json builds libavoid (its ADAPTAGRAMS_VERSION).
ADAPTAGRAMS_TAG = v1.0.4 (commit dd3236536399e5297b353d02c0b751eeaaf63aee)

define NOTICES
{ \
	notice() { printf '\n%s\n%s\n\n' '--------------------------------------------------------------------------------' "$$1"; for f in $$2; do cat "$$f"; done; }; \
	licenses() { find "$$1" -maxdepth 1 -type f \( -iname 'licen[cs]e*' -o -iname 'copying*' -o -iname 'notice*' \) | LC_ALL=C sort; }; \
	echo 'Third-party notices for dg'; echo; \
	echo 'The dg binary holds the Go standard library, the Go modules and the npm packages of the web'; \
	echo 'app listed below; the VS Code extension adds its npm packages and the HCL grammar. Each is'; \
	echo 'followed by its license.'; \
	notice "Go standard library $$(go env GOVERSION)" "$$(go env GOROOT)/LICENSE"; \
	go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' ./cmd/dg | LC_ALL=C sort -u | \
		while read -r mod version dir; do notice "$$mod $$version" "$$(licenses "$$dir")"; done; \
	for dir in $$(cd web && npm ls --omit=dev --all --parseable | tail -n +2 | LC_ALL=C sort) $$(cd vscode && npm ls --omit=dev --all --parseable | tail -n +2 | LC_ALL=C sort); do \
		notice "$$(node -p "const p = require('$$dir/package.json'); p.name + ' ' + p.version")" "$$(licenses "$$dir")"; done; \
	printf '\n%s\n%s\n\n' '--------------------------------------------------------------------------------' "libavoid from adaptagrams $(ADAPTAGRAMS_TAG), in libavoid-js $$(node -p "require('./web/node_modules/libavoid-js/package.json').version")"; \
	echo 'Copyright (C) 2004-2015 Monash University. Author: Michael Wybrow.'; \
	echo "Parts of geometry.cpp: Copyright (C) 1998 Joseph O'Rourke."; \
	echo; \
	echo 'Licensed under the GNU Lesser General Public License 2.1 or later, whose text is under'; \
	echo 'libavoid-js above. The corresponding source is the adaptagrams fork at'; \
	echo 'https://github.com/Aksem/adaptagrams at that tag, with the patch and build scripts of'; \
	echo 'https://github.com/Aksem/libavoid-js at the tag of its version.'; \
	notice 'HCL TextMate grammar, github.com/hashicorp/syntax v0.7.1 (vscode/syntaxes), modified by dg: scopeName source.dg' vscode/syntaxes/LICENSE; \
}
endef

# The VS Code extension test starts VS Code (Insiders where macOS installs it, else DG_VSCODE,
# else a build it downloads) on copies of examples, with the extension from vscode/.
test-vscode: web vscode/node_modules
	mkdir -p vscode/bin
	go build -o vscode/bin/dg ./cmd/dg
	cd vscode && npm test

# vsix-<target> packages the extension with dg built for a VS Code platform target into
# bin/dg-<target>.vsix; vsix packages it for this machine, test-vsix installs that package
# in VS Code and runs the extension test on it.
VSIX_TARGETS = darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-arm64 win32-x64
HOST_TARGET := $(subst windows,win32,$(shell go env GOOS))-$(subst amd64,x64,$(shell go env GOARCH))
target_os = $(subst win32,windows,$(word 1,$(subst -, ,$1)))
target_arch = $(subst x64,amd64,$(word 2,$(subst -, ,$1)))

vsix-%: web vscode/node_modules
	rm -rf vscode/bin
	CGO_ENABLED=0 GOOS=$(call target_os,$*) GOARCH=$(call target_arch,$*) \
		go build -trimpath -ldflags '-s -w' -o vscode/bin/dg$(if $(findstring win32,$*),.exe) ./cmd/dg
	cp LICENSE THIRD_PARTY_NOTICES vscode/
	mkdir -p bin
	cd vscode && npm run compile && npx vsce package --target $* -o ../bin/dg-$*.vsix

vsix: vsix-$(HOST_TARGET)

vsix-all: $(addprefix vsix-,$(VSIX_TARGETS))

test-vsix: vsix
	cd vscode && npm run compile && DG_VSIX=../bin/dg-$(HOST_TARGET).vsix node out/test/run.js

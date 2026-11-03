# The Go packages embed web/dist, so build the web app before any go command.

.PHONY: all build web test lint

all: lint test build

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	@touch $@

web: web/node_modules
	cd web && npm run build

build: web
	go build -o bin/dg ./cmd/dg

test: web
	go test ./...
	cd web && npm test

lint: web
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	cd web && npm run typecheck

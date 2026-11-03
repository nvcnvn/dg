// Package web embeds the browser app that `make web` builds into web/dist.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var dist embed.FS

// contentSecurityPolicy keeps the app offline: everything it loads comes from the embedded
// files, except pictures (icons, images and custom shapes), which the app draws from data: URIs
// of the SVG text in the resolved view. Inline styles carry the edge animations; with fonts,
// images and connections limited to the embedded files, CSS cannot send anything out.
const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; worker-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'"

// Handler serves the embedded app.
func Handler() http.Handler {
	app, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(app)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})
}

package lsp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkKeystroke is one keystroke in a model file of 1,000 nodes and 1,000 edges: the
// change, loading the project again and publishing what changed in its diagnostics. With
// errors, every edge names two undefined nodes, so the project has 2,000 diagnostics.
func BenchmarkKeystroke(b *testing.B) {
	for _, errors := range []bool{false, true} {
		b.Run(map[bool]string{false: "clean", true: "errors"}[errors], func(b *testing.B) {
			dir := b.TempDir()
			var model strings.Builder
			model.WriteString("# typed here\n\n")
			for g := range 100 {
				fmt.Fprintf(&model, "node \"g%d\" {\n  kind = \"group\"\n}\n\n", g)
				for c := range 9 {
					fmt.Fprintf(&model, "node \"n%d-%d\" {\n  parent = node.g%d\n  tags   = [\"t%d\"]\n}\n\n", g, c, g, c)
				}
			}
			for e := range 1000 {
				from, to := fmt.Sprintf("n%d-%d", e%100, e%9), fmt.Sprintf("n%d-%d", (e+1)%100, (e+1)%9)
				if errors {
					from, to = "x"+from, "x"+to
				}
				fmt.Fprintf(&model, "edge \"e%d\" {\n  from = node.%s\n  to   = node.%s\n}\n\n", e, from, to)
			}
			if err := os.WriteFile(filepath.Join(dir, "model.hcl"), []byte(model.String()), 0o644); err != nil {
				b.Fatal(err)
			}
			s, err := New(dir)
			if err != nil {
				b.Fatal(err)
			}
			s.out = io.Discard
			s.state = running
			s.docs["model.hcl"] = &document{text: model.String()}
			if err := s.publish(); err != nil {
				b.Fatal(err)
			}
			change := fmt.Sprintf(`{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": {"textDocument": {"uri": %q, "version": 2}, "contentChanges": [{"range": {"start": {"line": 0, "character": 2}, "end": {"line": 0, "character": 2}}, "text": "x"}]}}`, s.uri("model.hcl"))
			for b.Loop() {
				if _, _, err := s.handle([]byte(change)); err != nil {
					b.Fatal(err)
				}
				if err := s.publish(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

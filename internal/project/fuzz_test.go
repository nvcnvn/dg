package project

import (
	"testing"
	"testing/fstest"
)

func FuzzLoad(f *testing.F) {
	f.Add([]byte(`node "a" { kind = "group" }`), []byte(`kind "x" { container = true }`), []byte(`style "x" { shape = "rounded" }`))
	f.Add([]byte("node \"b\" {\n  parent = node[\"a\"]\n}\nedge \"e\" {\n  from = node.b\n  to = node.a.port.p\n}"), []byte(`edge_kind "k" {}`), []byte(`edge_style "k" { stroke = "#123" }`))
	f.Fuzz(func(t *testing.T, model, spec, style []byte) {
		fsys := fstest.MapFS{
			"model.hcl": {Data: model},
			"spec.hcl":  {Data: spec},
			"style.hcl": {Data: style},
		}
		if _, _, err := load(fsys, "."); err != nil {
			t.Fatal(err)
		}
	})
}

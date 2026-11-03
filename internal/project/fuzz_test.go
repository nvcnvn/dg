package project

import (
	"testing"
	"testing/fstest"
)

func FuzzLoad(f *testing.F) {
	f.Add([]byte(`node "a" { kind = "group" }`), []byte(`kind "x" { container = true }`), []byte(`style "x" { shape = "rounded" }`), []byte(``), []byte(``), []byte(``))
	f.Add([]byte("node \"b\" {\n  parent = node[\"a\"]\n  ports = port_range(\"p\", 1, 3)\n  props = { n = 1 }\n}\nedge \"e\" {\n  from = node.b.port.p1\n  to = node.a.row[\"x()\"]\n  labels = { start = \"1\" }\n}"),
		[]byte("kind \"k\" {\n  rows = true\n  ports = [\"a\"]\n  property \"n\" {\n    type = number\n    default = 2\n  }\n}\nedge_kind \"k\" {}"),
		[]byte("edge_style \"k\" { stroke = \"#123\" }\nshape \"s\" {\n  svg = file(\"model.hcl\")\n  anchors = { a = [0, 1] }\n}"),
		[]byte("project \"p\" {\n  models = [\"*.hcl\"]\n  read_only = [\"gen/*\"]\n  grid = 5\n}"),
		[]byte("view \"v\" {\n  include = [kind.k, tag.t, node[\"a\"]]\n  collapse = [children.a]\n  edges = [edge_kind.k]\n}"),
		[]byte("version: 1\nplace:\n  a: {in: b, x: 0, y: 10, ports: {p: {side: top, offset: 0.5}}}\nedge_hints:\n  lift:a:b: {via: [[1, 2]]}\n"))
	f.Add([]byte("node \"a\" {\n  ports = port_range(\"p\", 9223372036854775807, 9223372036854775807)\n}\nnode \"b\" {\n  ports = port_range(\"p\", -9223372036854775807, 9223372036854775807)\n}\nnode \"c\" {\n  ports = port_range(\"p\", -1e9, -999999990)\n}"), []byte(``), []byte(``), []byte("project \"p\" {\n  packs = [\".\"]\n  views = file(\"model.hcl\")\n}"), []byte(``), []byte(``))
	f.Add([]byte("node \"g\" { kind = \"group\" }\nnode \"h\" {\n  kind = \"group\"\n  parent = node.g\n}\nnode \"a\" { parent = node.h }\nnode \"b\" {}\nedge \"e\" {\n  from = node.a\n  to = node.b\n}\nedge \"f\" {\n  from = node.b\n  to = node.a\n  kind = \"async\"\n}\nedge \"s\" {\n  from = node.a\n  to = node.g\n}"), []byte(``), []byte("overlay_style \"red\" { stroke = \"#F00\" }"), []byte(``),
		[]byte("view \"v\" {\n  collapse = [node.h, kind.group]\n  direction = \"up\"\n  overlay \"o\" {\n    members = [node.a, node.b]\n    style = \"red\"\n  }\n  style \"node.b\" { fill = \"#FFF\" }\n  edge_style \"edge.e\" { stroke = \"#000\" }\n}\nview \"w\" { of = node.h }"),
		[]byte("version: 1\nedge_hints:\n  lift:g:b: {to_side: left}\ndecorations:\n  - {id: t, type: text, attach: a, dx: 0, dy: -10, text: x}\n  - {id: i, type: image, src: model.hcl, w: 1, h: 1, in: g}\n"))
	f.Fuzz(func(t *testing.T, model, spec, style, project, views, layout []byte) {
		fsys := fstest.MapFS{
			"model.hcl":              {Data: model},
			"spec.hcl":               {Data: spec},
			"style.hcl":              {Data: style},
			"dg.hcl":                 {Data: project},
			"views.hcl":              {Data: views},
			"views/v.layout.yaml":    {Data: layout},
			"views/main.layout.yaml": {Data: layout},
		}
		p, _, err := load(fsys, ".")
		if err != nil {
			t.Fatal(err)
		}
		for id := range p.Views {
			p.Resolve(id)
		}
	})
}

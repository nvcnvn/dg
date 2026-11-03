package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkLoad1000 loads and validates a project of 1,000 nodes and 1,000 edges, which is the
// work of dg check; spec 11 requires it to take under 1 second.
func BenchmarkLoad1000(b *testing.B) {
	dir := b.TempDir()
	var model, layout strings.Builder
	layout.WriteString("version: 1\nplace:\n")
	for g := range 100 {
		fmt.Fprintf(&model, "node \"g%d\" {\n  kind = \"group\"\n}\n\n", g)
		fmt.Fprintf(&layout, "  g%d: {x: %d, y: 0}\n", g, g*200)
		for c := range 9 {
			fmt.Fprintf(&model, "node \"n%d-%d\" {\n  kind   = \"box\"\n  parent = node.g%d\n  tags   = [\"t%d\"]\n  props  = { size = %d }\n}\n\n", g, c, g, c, c)
			fmt.Fprintf(&layout, "  n%d-%d: {in: g%d, x: 0, y: %d}\n", g, c, g, c*60)
		}
	}
	for e := range 1000 {
		g, c := e%100, e%9
		fmt.Fprintf(&model, "edge \"e%d\" {\n  from = node[\"n%d-%d\"].port.p1\n  to   = node.n%d-%d.port.p2\n}\n\n", e, g, c, (g+1)%100, (c+1)%9)
	}
	files := map[string]string{
		"spec.hcl":               "kind \"box\" {\n  ports = port_range(\"p\", 1, 8)\n  property \"size\" {\n    type = number\n  }\n}\n",
		"style.hcl":              "style \"box\" {\n  shape = \"rounded\"\n}\n\nstyle \"tag.t1\" {\n  fill = \"#EEEEEE\"\n}\n",
		"model.hcl":              model.String(),
		"views.hcl":              "view \"main\" {}\n\nview \"some\" {\n  include  = [kind.box, tag.t1, descendants.g1]\n  exclude  = [children.g2]\n  collapse = [node.g3, kind.group]\n  overlay \"o\" {\n    members = [tag.t2]\n  }\n}\n",
		"views/main.layout.yaml": layout.String(),
	}
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	for b.Loop() {
		p, diags, err := Load(dir)
		if err != nil || len(diags) > 0 || len(p.Model.Nodes) != 1000 {
			b.Fatal(err, diags)
		}
	}
}

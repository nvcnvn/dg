package view_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestResolveExamples(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "examples", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		p, diags, err := project.Load(dir)
		if err != nil || diags.HasErrors() {
			t.Fatalf("load %s: %v %v", dir, err, diags)
		}
		for id := range p.Views {
			name := filepath.Base(dir)
			if id != view.DefaultView {
				name += "-" + id
			}
			resolveGolden(t, p, id, name)
		}
	}
}

func resolveGolden(t *testing.T, p *project.Project, id, name string) {
	t.Run(name, func(t *testing.T) {
		v, diags := p.Resolve(id)
		if diags.HasErrors() {
			for _, d := range diags {
				if d.Summary != "Not supported yet" && d.Severity == diag.Error {
					t.Fatal(diags)
				}
			}
			t.Skipf("drawn by a later milestone: %v", diags)
		}
		got, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, '\n')
		path := filepath.Join("..", "..", "testdata", "resolve", name+".json")
		if *update {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (run go test with -update to create it)", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the output:\n%s", path, got)
		}
	})
}

func TestResolveUndefinedView(t *testing.T) {
	p, _, err := project.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, diags := p.Resolve("nope"); !diags.HasErrors() {
		t.Error("resolving an undefined view succeeded")
	}
}

func TestResolvePins(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"model.hcl":                "node \"g\" {\n  kind = \"group\"\n}\n\nnode \"a\" {\n  parent = node.g\n}\n\nnode \"b\" {}\n",
		"views.hcl":                "view \"all\" {}\n\nview \"leaves\" {\n  include = [node.a, node.b]\n}\n",
		"views/all.layout.yaml":    "version: 1\nplace:\n  a: {x: 1, y: 2}\n  b: {in: g, x: 3, y: 4}\n",
		"views/leaves.layout.yaml": "version: 1\nplace:\n  a: {in: g, x: 1, y: 2}\n  b: {x: 3, y: 4, w: 50}\n",
	}
	for name, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, _, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	pins := func(id string) map[string]string {
		v, diags := p.Resolve(id)
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		out := map[string]string{}
		for _, n := range v.Nodes {
			data, _ := json.Marshal(n.Pin)
			out[n.ID] = string(data)
		}
		return out
	}
	for _, tc := range []struct{ view, node, want string }{
		{"all", "a", `{"in":"","x":1,"y":2}`},
		{"all", "b", `{"in":"g","x":3,"y":4}`},
		{"leaves", "a", `null`},
		{"leaves", "b", `{"x":3,"y":4,"w":50}`},
	} {
		if got := pins(tc.view)[tc.node]; got != tc.want {
			t.Errorf("view %s node %s pin %s, want %s", tc.view, tc.node, got, tc.want)
		}
	}
}

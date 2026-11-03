package view_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestResolveExamples(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "examples", "*"))
	if err != nil {
		t.Fatal(err)
	}
	// testdata/lift has a view for each lifting and merging case of spec 8.3.
	for _, dir := range append(dirs, filepath.Join("..", "..", "testdata", "lift")) {
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
			t.Fatal(diags)
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

// TestResolveOrder checks that a stack container lists its children in its order list, then the
// children the list leaves out in sorted ID order; that a child's stored position is dropped
// and its size kept; and that a view whose of is the container places the canvas by its policy.
func TestResolveOrder(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"model.hcl":                "node \"s\" {\n  kind = \"lane-set\"\n}\n\nnode \"d\" {\n  parent = node.s\n}\n\nnode \"b\" {\n  parent = node.s\n}\n\nnode \"c\" {\n  parent = node.s\n}\n\nnode \"a\" {\n  parent = node.s\n}\n\nnode \"o\" {}\n",
		"views.hcl":                "view \"main\" {}\n\nview \"inside\" {\n  of = node.s\n}\n\nview \"extra\" {\n  of      = node.s\n  include = [descendants.s, node.o]\n}\n",
		"views/extra.layout.yaml":  "version: 1\nplace:\n  a: {x: 10, y: 10}\n  o: {x: 10, y: 10}\n",
		"views/main.layout.yaml":   "version: 1\nplace:\n  b: {in: s, x: 10, y: 20, w: 200}\n  c: {in: s, x: 10, y: 20}\n  s: {order: [c, gone, a, c]}\n",
		"views/inside.layout.yaml": "version: 1\nplace:\n  s: {order: [d]}\n",
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
	main, diags := p.Resolve("main")
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	got := map[string]string{}
	for _, n := range main.Nodes {
		data, _ := json.Marshal(n.Pin)
		got[n.ID] = string(data)
		if n.ID == "s" {
			data, _ = json.Marshal(n.Policy)
			got["policy"] = string(data)
		}
	}
	want := map[string]string{
		"a": "null", "b": `{"x":0,"y":0,"w":200}`, "c": "null", "d": "null", "s": "null",
		"policy": `{"layout":"stack","direction":"vertical","gap":0,"stretch":true,"columns":3,"align":"start","order":["c","a","b","d"]}`,
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: %s, want %s", id, got[id], w)
		}
	}
	inside, diags := p.Resolve("inside")
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	data, _ := json.Marshal(inside.Policy)
	if w := `{"container":"s","layout":"stack","direction":"vertical","gap":0,"stretch":true,"columns":3,"align":"start","order":["d","a","b","c"]}`; string(data) != w {
		t.Errorf("view inside: policy %s, want %s", data, w)
	}
	extra, diags := p.Resolve("extra")
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	data, _ = json.Marshal(extra.Policy.Order)
	if w := `["a","b","c","d"]`; string(data) != w {
		t.Errorf("view extra: order %s, want %s, without o", data, w)
	}
	var ignored []string
	for _, d := range p.ValidateLayouts() {
		if d.Summary == "Ignored position" {
			ignored = append(ignored, d.Subject.Filename+":"+d.Detail)
		}
	}
	if len(ignored) != 3 || !strings.Contains(ignored[0], `extra.layout.yaml:Node "a"`) {
		t.Errorf("ignored positions %q, want a in extra (not o), b and c in main", ignored)
	}
}

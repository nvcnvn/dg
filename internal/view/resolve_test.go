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

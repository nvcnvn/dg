package view

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/nvcnvn/dg/internal/project"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestResolveExamples(t *testing.T) {
	for _, name := range []string{"hello"} {
		t.Run(name, func(t *testing.T) {
			p, diags, err := project.Load(filepath.Join("..", "..", "examples", name))
			if err != nil || diags.HasErrors() {
				t.Fatalf("load: %v %v", err, diags)
			}
			v, err := Resolve(p, DefaultView)
			if err != nil {
				t.Fatal(err)
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
}

func TestResolveUndefinedView(t *testing.T) {
	p, _, err := project.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(p, "nope"); err == nil {
		t.Error("resolving an undefined view succeeded")
	}
}

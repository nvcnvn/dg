package project

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/nvcnvn/dg/internal/style"
)

func TestLoadNotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "model.hcl")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(t.TempDir(), "missing"), file} {
		if _, _, err := Load(dir); err == nil {
			t.Errorf("Load(%s) succeeded", dir)
		}
	}
}

func TestStyleCascade(t *testing.T) {
	fsys := fstest.MapFS{
		"spec.hcl":  {Data: []byte("kind \"zone\" {\n  extends = \"group\"\n}\nedge_kind \"async\" {}\n")},
		"style.hcl": {Data: []byte("style \"zone\" {\n  shape = \"rounded\"\n}\n")},
		"model.hcl": {},
	}
	p, diags, err := load(fsys, ".")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	if got, want := style.DrawnNode(p.Styles.ForNode(p.Spec, "zone", nil)), (style.Node{Shape: "rounded", Fill: "#F8F9FA", Stroke: "#9AA0A6"}); got != want {
		t.Errorf("zone style %+v, want %+v", got, want)
	}
	if got, want := style.DrawnEdge(p.Styles.ForEdge(p.Spec, "async", nil)), (style.Edge{Stroke: "#3C4043", ArrowEnd: "arrow"}); got != want {
		t.Errorf("async style %+v, want %+v", got, want)
	}
	for _, k := range []string{"node", "service", "group"} {
		if st := style.DrawnNode(p.Styles.ForNode(p.Spec, k, nil)); st.Shape == "" || st.Fill == "" || st.Stroke == "" {
			t.Errorf("generic pack leaves a property of %q unset: %+v", k, st)
		}
	}
}

func TestLoadDoesNotFollowLinksOut(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "label.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "model.hcl"), []byte("node \"a\" {\n  label = file(\"label.txt\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, diags, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !diags.HasErrors() || p.Model.Nodes["a"].Label == "secret" {
		t.Errorf("file() read through a link out of the project: %v", diags)
	}
}

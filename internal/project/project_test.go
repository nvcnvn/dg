package project

import (
	"os"
	"path/filepath"
	"slices"
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
	if got := style.DrawnNode(p.Styles.ForNode(p.Spec, "zone", nil)); got.Shape != "rounded" || got.Fill != "#F8F9FA" || got.Padding != 20 || got.FontSize != 14 {
		t.Errorf("zone style %+v, want group's style with shape rounded", got)
	}
	if got := style.DrawnEdge(p.Styles.ForEdge(p.Spec, "async", nil)); got.ArrowEnd != "arrow" || len(got.StrokeDash) != 2 || got.StrokeWidth != 1.5 {
		t.Errorf("async style %+v, want default's style dashed", got)
	}
}

// TestGenericDefaults checks that the generic pack's root styles set every property that has a
// default (spec 8.4); icon and image have none.
func TestGenericDefaults(t *testing.T) {
	p, diags, err := load(fstest.MapFS{"model.hcl": {}}, ".")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	nodes := p.Styles.ForNode(p.Spec, "node", nil)
	for name := range style.NodeProps {
		if _, ok := nodes[name]; !ok && name != "icon" && name != "image" {
			t.Errorf("style \"node\" does not set %s", name)
		}
	}
	edges := p.Styles.ForEdge(p.Spec, "default", nil)
	for name := range style.EdgeProps {
		if _, ok := edges[name]; !ok {
			t.Errorf("edge_style \"default\" does not set %s", name)
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

func TestVocabularyPacks(t *testing.T) {
	fsys := fstest.MapFS{
		"dg.hcl":              {Data: []byte("project \"p\" {\n  packs = [\"packs/zeta\", \"packs/acme\"]\n}\n")},
		"packs/zeta/spec.hcl": {Data: []byte("kind \"zed\" {}\n")},
		"packs/acme/spec.hcl": {Data: []byte("kind \"gear\" {}\nkind \"moved\" {}\n")},
		"spec.hcl":            {Data: []byte("kind \"mine\" {}\nkind \"moved\" {}\n")},
		"model.hcl":           {},
	}
	p, diags, err := load(fsys, ".")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v := p.Vocabulary()
	if want := []string{"generic", "zeta", "acme"}; !slices.Equal(v.Packs, want) {
		t.Errorf("packs %v, want %v", v.Packs, want)
	}
	want := map[string]string{"service": "generic", "zed": "zeta", "gear": "acme", "mine": "", "moved": ""}
	for _, k := range v.Kinds {
		if pack, ok := want[k.Name]; ok && k.Pack != pack {
			t.Errorf("kind %s in pack %q, want %q", k.Name, k.Pack, pack)
		}
	}
}

package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	p, diags, err := LoadFS(fsys)
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
// default of its own (spec 8.4): icon and image have none, and the sub_ properties take the
// label's.
func TestGenericDefaults(t *testing.T) {
	p, diags, err := LoadFS(fstest.MapFS{"model.hcl": {}})
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	nodes := p.Styles.ForNode(p.Spec, "node", nil)
	for name := range style.NodeProps {
		if _, ok := nodes[name]; !ok && name != "icon" && name != "image" && !strings.HasPrefix(name, "sub_") {
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
	p, diags, err := LoadFS(fsys)
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

func TestTooDeep(t *testing.T) {
	deep := func(open string, n int, end string) string {
		return "x = " + strings.Repeat(open, n) + end + "\n"
	}
	tests := []struct {
		name, src string
		want      bool
	}{
		{"lists", deep("[", 101, ""), true},
		{"lists within the limit", deep("[", 100, "1"+strings.Repeat("]", 100)), false},
		{"parentheses", deep("(", 101, ""), true},
		{"blocks", strings.Repeat("a {\n", 101), true},
		{"not", deep("!", 101, "true"), true},
		{"negation", deep("-", 101, "1"), true},
		{"templates", deep(`"${`, 101, ""), true},
		{"sum", "x = 1" + strings.Repeat(" + 1", 101) + "\n", true},
		{"conditions", "x = a" + strings.Repeat(" ? b : a", 101) + "\n", true},
		{"indexes", "x = a" + strings.Repeat("[0]", 101) + "\n", true},
		{"a chain over lines within parentheses", "x = (1" + strings.Repeat("\n+ 1", 101) + ")\n", true},
		{"many statements", strings.Repeat("x = a.b + 1\n", 200), false},
		{"a long list", "x = [" + strings.Repeat("-1, ", 200) + "]\n", false},
		{"brackets in a string", `x = "` + strings.Repeat("[", 200) + `"` + "\n", false},
		{"brackets in a comment", "# " + strings.Repeat("(", 200) + "\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := TooDeep([]byte(tt.src), "model.hcl")
			if d.HasErrors() != tt.want {
				t.Errorf("TooDeep: %v", d)
			}
		})
	}
}

// TestDeepFile checks that a project with a file too deep to parse loads with an error in it.
func TestDeepFile(t *testing.T) {
	p, diags, err := LoadFS(fstest.MapFS{"model.hcl": {Data: []byte("node \"a\" {\n  label = " + strings.Repeat("[", 1_000_000) + "\n}\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if !diags.HasErrors() || !strings.Contains(diags[0].String(), "Nested too deeply") || !slices.Contains(p.Unparsed, "model.hcl") {
		t.Errorf("diagnostics %v, unparsed %v", diags, p.Unparsed)
	}
}

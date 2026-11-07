package project

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nvcnvn/dg/internal/style"
)

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`

// installed returns a project with the pack files given, locked under the name aws.
func installed(t *testing.T, files map[string]string, more ...string) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{"model.hcl": {}}
	for name, src := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(src)}
	}
	pack, err := ReadPack(fsys, "packs/aws")
	if err != nil {
		t.Fatal(err)
	}
	entry := LockEntry{"example.com/aws", "v1.0.0", strings.Repeat("a", 40), PackHash(pack)}
	fsys["dg.lock"] = &fstest.MapFile{Data: (&Lock{Packs: map[string]LockEntry{"aws": entry}}).Encode()}
	fsys["dg.hcl"] = &fstest.MapFile{Data: []byte("project \"p\" {\n  pack \"aws\" {\n    source  = \"example.com/aws\"\n    version = \"v1.0.0\"\n  }\n" + strings.Join(more, "") + "}\n")}
	return fsys
}

func TestInstalledNames(t *testing.T) {
	p, diags, err := LoadFS(installed(t, map[string]string{
		"packs/aws/spec.hcl": `kind "node" {}
kind "vpc" {
  extends  = "group"
  category = "Networking"
}
kind "subnet" {
  extends = "vpc"
}
kind "fn" {
  extends = "service"
}
edge_kind "peering" {}
`,
		"packs/aws/style.hcl": `style "vpc" {
  shape = "hex"
  icon  = file("icons/vpc.svg")
}
style "fn" {
  shape = "rounded"
}
style "tag.legacy" {
  fill = "#EEEEEE"
}
edge_style "peering" {
  stroke = "#FF9900"
}
overlay_style "zone" {
  stroke = "#FF9900"
}
shape "hex" {
  svg = file("icons/hex.svg")
}
`,
		"packs/aws/icons/vpc.svg": testSVG,
		"packs/aws/icons/hex.svg": testSVG,
		"packs/acme/spec.hcl":     "kind \"gear\" {\n  extends = \"aws-vpc\"\n}\n",
		"spec.hcl":                "kind \"mine\" {\n  extends = \"aws-subnet\"\n}\n",
	}, "  packs = [\"packs/acme\"]\n"))
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	for name, extends := range map[string]string{"aws-node": "node", "aws-vpc": "group", "aws-subnet": "aws-vpc", "aws-fn": "service", "gear": "aws-vpc", "mine": "aws-subnet"} {
		if k := p.Spec.Kinds[name]; k == nil || k.Extends != extends {
			t.Errorf("kind %s: %+v, want it to extend %s", name, k, extends)
		}
	}
	if p.Spec.Kinds["vpc"] != nil || p.Spec.EdgeKinds["aws-peering"] == nil || p.Spec.EdgeKinds["peering"] != nil {
		t.Errorf("the short names are defined: %v", slices.Sorted(maps.Keys(p.Spec.Kinds)))
	}
	if !p.Spec.IsContainer("gear") {
		t.Error("a local kind does not inherit from the installed pack's")
	}
	if got := style.DrawnNode(p.Styles.ForNode(p.Spec, "aws-subnet", []string{"legacy"})); got.Shape != "aws-hex" || got.Icon == "" || got.Fill != "#EEEEEE" {
		t.Errorf("aws-subnet style %+v", got)
	}
	if sh := p.Styles.Shapes["aws-hex"]; sh == nil || sh.Name != "aws-hex" {
		t.Errorf("shape %+v", sh)
	}
	if got := style.DrawnEdge(p.Styles.ForEdge(p.Spec, "aws-peering", nil)); got.Stroke != "#FF9900" {
		t.Errorf("aws-peering style %+v", got)
	}
	if i := slices.IndexFunc(p.Styles.Overlay, func(r style.Rule) bool { return r.Label == "aws-zone" }); i < 0 {
		t.Errorf("overlay styles %+v", p.Styles.Overlay)
	}
	if rng := p.Spec.Kinds["aws-vpc"].Range; rng.Filename != "packs/aws/spec.hcl" || rng.Start.Line != 2 || rng.Start.Column != 1 {
		t.Errorf("aws-vpc at %v", rng)
	}

	v := p.Vocabulary()
	if want := []string{"generic", "aws", "acme"}; !slices.Equal(v.Packs, want) {
		t.Errorf("packs %v, want %v", v.Packs, want)
	}
	want := map[string]KindInfo{
		"aws-vpc":    {Pack: "aws", Short: "vpc", Category: "Networking"},
		"aws-subnet": {Pack: "aws", Short: "subnet"},
		"gear":       {Pack: "acme"},
		"mine":       {},
		"service":    {Pack: "generic"},
	}
	for _, k := range v.Kinds {
		if w, ok := want[k.Name]; ok && (k.Pack != w.Pack || k.Short != w.Short || k.Category != w.Category) {
			t.Errorf("kind %+v, want %+v", k, w)
		}
	}
	if data, err := json.Marshal(v.Kinds[slices.IndexFunc(v.Kinds, func(k KindInfo) bool { return k.Name == "aws-vpc" })]); err != nil || !strings.Contains(string(data), `"short":"vpc","category":"Networking"`) {
		t.Errorf("%s, %v", data, err)
	}
}

func TestInstalledIsReadOnly(t *testing.T) {
	p, diags, err := LoadFS(installed(t, map[string]string{"packs/aws/spec.hcl": "kind \"vpc\" {}\n", "packs/awsome/spec.hcl": ""}))
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	for rel, want := range map[string]bool{"packs/aws/spec.hcl": true, "packs/aws/icons/a.svg": true, "packs/AWS/spec.hcl": true, "PACKS/aws/spec.hcl": true, "packs/awsome/spec.hcl": false, "model.hcl": false} {
		if got := p.IsReadOnly(rel); got != want {
			t.Errorf("IsReadOnly(%s) = %v", rel, got)
		}
	}
}

func TestInstalledSymlink(t *testing.T) {
	dir := t.TempDir()
	files := installed(t, map[string]string{"packs/aws/spec.hcl": "kind \"vpc\" {}\n", "secret.svg": testSVG})
	if err := os.CopyFS(dir, files); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../secret.svg", filepath.Join(dir, "packs/aws/link.svg")); err != nil {
		t.Skip(err)
	}
	_, diags, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !diags.HasErrors() || !strings.Contains(diags[0].String(), "link.svg is a symbolic link") {
		t.Errorf("diagnostics %v", diags)
	}
}

func TestReadPack(t *testing.T) {
	file := func(src string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(src)} }
	tests := []struct {
		name  string
		files fstest.MapFS
		want  []string
		err   string
	}{
		{
			name: "the set of spec 4.7",
			files: fstest.MapFS{
				"p/spec.hcl": file(""), "p/style.hcl": file(""), "p/README.md": file(""), "p/LICENSE": file(""), "p/NOTICE.txt": file(""),
				"p/a.svg": file(""), "p/deep/er/b.svg": file(""), "p/run.sh": file(""), "p/model.hcl": file(""),
				"p/deep/spec.hcl": file(""), "p/deep/README.md": file(""), "p/.git/refs/tags/v1.svg": file(""), "other/c.svg": file(""),
			},
			want: []string{"LICENSE", "NOTICE.txt", "README.md", "a.svg", "deep/er/b.svg", "spec.hcl", "style.hcl"},
		},
		{name: "a style alone", files: fstest.MapFS{"p/style.hcl": file("")}, want: []string{"style.hcl"}},
		{name: "neither file", files: fstest.MapFS{"p/a.svg": file("")}, err: "neither"},
		{name: "a file for a directory", files: fstest.MapFS{"p": file("")}, err: "neither"},
		{name: "no directory", files: fstest.MapFS{}, err: "does not exist"},
		{name: "a link", files: fstest.MapFS{"p/spec.hcl": file(""), "p/a.svg": {Mode: 0o777 | os.ModeSymlink}}, err: "a.svg is a symbolic link"},
		{name: "a link to a directory, not followed", files: fstest.MapFS{"p/spec.hcl": file(""), "p/icons": {Mode: 0o777 | os.ModeSymlink}}, want: []string{"spec.hcl"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pack, err := ReadPack(tt.files, "p")
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("error %v, want %s", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Sorted(maps.Keys(pack)); !slices.Equal(got, tt.want) {
				t.Errorf("files %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadPackAtRoot(t *testing.T) {
	pack, err := ReadPack(fstest.MapFS{"spec.hcl": {}, "icons/a.svg": {}, "run.sh": {}}, ".")
	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(maps.Keys(pack)); !slices.Equal(got, []string{"icons/a.svg", "spec.hcl"}) {
		t.Errorf("files %v", got)
	}
}

func TestPackHash(t *testing.T) {
	pack := func(pairs ...string) fstest.MapFS {
		m := fstest.MapFS{}
		for i := 0; i < len(pairs); i += 2 {
			m[pairs[i]] = &fstest.MapFile{Data: []byte(pairs[i+1])}
		}
		return m
	}
	base := pack("a.svg", "one\ntwo\n", "b.svg", "three")
	const want = "sha256:878a125ceb9ff827544e7ca391265c852e31da86c9ca9a8878d2047a5661c2ea"
	if got := PackHash(base); got != want {
		t.Errorf("hash %s, want %s", got, want)
	}
	if PackHash(pack("a.svg", "one\r\ntwo\r\n", "b.svg", "three")) != PackHash(base) {
		t.Error("CRLF changes the hash")
	}
	for name, other := range map[string]fstest.MapFS{
		"content":        pack("a.svg", "one\ntwo\n", "b.svg", "four"),
		"path":           pack("a.svg", "one\ntwo\n", "c.svg", "three"),
		"one more file":  pack("a.svg", "one\ntwo\n", "b.svg", "three", "c.svg", ""),
		"content moved":  pack("a.svg", "one\ntwo\nthree", "b.svg", ""),
		"a lone CR kept": pack("a.svg", "one\rtwo\n", "b.svg", "three"),
	} {
		if PackHash(other) == PackHash(base) {
			t.Errorf("%s: the hash did not change", name)
		}
	}
}

func TestLockEncode(t *testing.T) {
	hash := "sha256:" + strings.Repeat("0", 64)
	commit := "9fceb02d0ae598e95dc970b74767f19372d61af8"
	lock := &Lock{Packs: map[string]LockEntry{
		"aws10": {"github.com/dg-packs/aws", "v1.2.0", commit, hash},
		"aws2":  {"https://example.com/a: b", "#v2", commit, hash},
	}}
	want := "version: 1\npacks:\n" +
		"  aws10:\n    source: github.com/dg-packs/aws\n    version: v1.2.0\n    commit: " + commit + "\n    hash: " + hash + "\n" +
		"  aws2:\n    source: 'https://example.com/a: b'\n    version: '#v2'\n    commit: " + commit + "\n    hash: " + hash + "\n"
	got := lock.Encode()
	if string(got) != want {
		t.Errorf("encoded:\n%s\nwant:\n%s", got, want)
	}
	back, diags := DecodeLock(got, LockFile)
	if diags.HasErrors() || !maps.Equal(back.Packs, lock.Packs) {
		t.Errorf("decoded %+v, %v", back, diags)
	}
	if got := string((&Lock{}).Encode()); got != "version: 1\npacks: {}\n" {
		t.Errorf("an empty lock is %q", got)
	}
}

func TestDecodeLock(t *testing.T) {
	entry := "source: s\n    version: v\n    commit: " + strings.Repeat("a", 40) + "\n    hash: sha256:" + strings.Repeat("0", 64)
	tests := []struct{ name, src, err string }{
		{"valid", "version: 1\npacks:\n  a:\n    " + entry + "\n", ""},
		{"no packs", "version: 1\n", ""},
		{"empty", "", "Missing version"},
		{"no version", "packs: {}\n", "Missing version"},
		{"a SHA-256 commit", "version: 1\npacks:\n  a:\n    source: s\n    version: v\n    commit: " + strings.Repeat("a", 64) + "\n    hash: sha256:" + strings.Repeat("0", 64) + "\n", "not a full commit hash"},
		{"newer", "version: 2\n", "not supported"},
		{"not a mapping", "[]\n", "Expected a mapping"},
		{"two documents", "version: 1\n---\nversion: 1\n", "more than one"},
		{"syntax", "version: [\n", "dg.lock:"},
		{"too deep", strings.Repeat("[", 200), "nests past"},
		{"repeated pack", "version: 1\npacks:\n  a:\n    " + entry + "\n  a:\n    " + entry + "\n", "repeated"},
		{"packs that differ only by case", "version: 1\npacks:\n  a:\n    " + entry + "\n  A:\n    " + entry + "\n", "repeated"},
		{"a version with a fraction", "version: 1.5\n", "not a whole number"},
		{"a version that is a float", "version: 1.0\n", "not a whole number"},
		{"a path for a name", "version: 1\npacks:\n  a/b:\n    " + entry + "\n", "simple ID"},
		{"the entry is not a mapping", "version: 1\npacks:\n  a: b\n", "Expected a mapping"},
		{"a field is missing", "version: 1\npacks:\n  a:\n    source: s\n", "no commit"},
		{"unknown field", "version: 1\npacks:\n  a:\n    " + entry + "\n    url: u\n", `Unknown field "url"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := DecodeLock([]byte(tt.src), LockFile)
			switch {
			case tt.err == "" && len(diags) > 0:
				t.Errorf("diagnostics %v", diags)
			case tt.err != "" && !strings.Contains(diags.Error(), tt.err):
				t.Errorf("diagnostics %v, want %s", diags, tt.err)
			}
		})
	}
}

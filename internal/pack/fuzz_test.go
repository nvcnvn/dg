package pack

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/project"
)

// FuzzArguments checks what arguments that pass can do. A source names a repository by an allowed
// scheme, a directory inside it and a version that cannot be an option; --as is a name that
// stays under packs/; and update, given a project without the pack, refuses before any fetch.
func FuzzArguments(f *testing.F) {
	for _, seed := range []struct{ arg, as, name, version string }{
		{"aws", "", "aws", ""},
		{"aws@v1.0.0", "x", "aws", "v1.0.0"},
		{"github.com/dg-packs/aws", "AWS", "../aws", "--upload-pack=x"},
		{"github.com/a/b//c/d@v1.2.3", "generic", "generic", "a:b"},
		{"ssh://git@example.com/a/b@v1", "a/../b", "a/b", "HEAD^"},
		{"file:///tmp/a", ".", ".", "a..b"},
		{"-x", "-x", "-x", "-x"},
		{"--upload-pack=x", "", "", ""},
		{"ext::sh -c x", "", "", strings.Repeat("a", 40)},
		{"a/b@--upload-pack=x", "", "", ""},
		{"https://u:p@e.com/a", "", "", ""},
		{"x/y//../z", "", "", ""},
		{"https://e.com/a?x#y", "", "", ""},
		{"ssh://-oProxyCommand=x/a", "", "", ""},
	} {
		f.Add(seed.arg, seed.as, seed.name, seed.version)
	}
	empty := f.TempDir()
	f.Fuzz(func(t *testing.T, arg, as, name, version string) {
		if project.LocalNameProblem(as) == "" && strings.ContainsAny(as, "/\\.") {
			t.Errorf("--as %q", as)
		}
		if _, err := Update(empty, name, version); err == nil {
			t.Errorf("update %q %q of a project with no packs", name, version)
		}
		if checkVersion(version) == nil && (strings.HasPrefix(version, "-") || strings.ContainsAny(version, ": ^~*?[\\")) {
			t.Errorf("version %q", version)
		}
		source, version, err := parseAdd(arg)
		if err != nil {
			return
		}
		if strings.HasPrefix(version, "-") {
			t.Errorf("version %q", version)
		}
		if !strings.Contains(source, "/") {
			return
		}
		loc, err := parseSource(source)
		if err != nil {
			return
		}
		if strings.HasPrefix(source, "-") {
			t.Errorf("source %q", source)
		}
		if !strings.HasPrefix(loc.url, "https://") && !strings.HasPrefix(loc.url, "ssh://") && !strings.HasPrefix(loc.url, "file://") {
			t.Errorf("url %q", loc.url)
		}
		if loc.dir != "" && !fs.ValidPath(loc.dir) {
			t.Errorf("dir %q", loc.dir)
		}
		if name := defaultName(source, loc); strings.Contains(name, "/") {
			t.Errorf("name %q", name)
		}
	})
}

// FuzzIndex decodes index.json, and looks every entry up as an argument would be: a name or
// source it holds does what an argument could.
func FuzzIndex(f *testing.F) {
	for _, seed := range []string{
		`{"version": 1, "packs": [{"name": "aws", "source": "github.com/dg-packs/aws", "description": "AWS", "license": "see NOTICE"}]}`,
		`{"version": 1, "packs": [{"name": "\u001b[2J", "source": "--upload-pack=x/y"}]}`,
		`{"version": 2}`, `{`, `null`, `{"version": 1, "packs": null}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		entries, err := decodeIndex(data)
		if err != nil {
			return
		}
		for _, e := range entries {
			if strings.ContainsFunc(e.Name+e.Source+e.Description+e.License, func(r rune) bool { return r < ' ' }) {
				t.Errorf("control character in %q", e)
			}
			if loc, err := parseSource(e.Source); err == nil && strings.HasPrefix(loc.url, "-") {
				t.Errorf("url %q", loc.url)
			}
		}
		match(entries, "a")
	})
}

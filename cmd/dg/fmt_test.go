package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestFmtGolden formats a copy of testdata/fmt/in and compares every file with testdata/fmt/want;
// the read-only generated file is left as it was. A second run changes nothing.
func TestFmtGolden(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../testdata/fmt/in")); err != nil {
		t.Fatal(err)
	}
	for i, wantOut := range []string{"dg.hcl\npacks/net/spec.hcl\nmodel.hcl\nviews.hcl\nviews/main.layout.yaml\n", ""} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"fmt", "-C", dir}, &stdout, &stderr); code != 0 {
			t.Fatalf("run %d: exit code %d; stderr:\n%s", i, code, stderr.String())
		}
		if got := stdout.String(); got != wantOut {
			t.Errorf("run %d printed:\n%s\nwant:\n%s", i, got, wantOut)
		}
	}
	want := os.DirFS("../../testdata/fmt/want")
	err := fs.WalkDir(want, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		w, err := fs.ReadFile(want, path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			return err
		}
		if *update {
			return os.WriteFile(filepath.Join("../../testdata/fmt/want", path), got, 0o644)
		}
		if !bytes.Equal(got, w) {
			t.Errorf("%s differs from testdata/fmt/want/%s:\n%s", path, path, got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestFmtRefusesWithoutValidConfig checks that dg fmt writes nothing when dg.hcl has errors, since
// its read_only patterns may be incomplete, and that --check writes nothing.
func TestFmtRefusesWithoutValidConfig(t *testing.T) {
	const messy = "node \"a\" {\nkind=\"node\"\n}\n"
	tests := []struct {
		name, config string
		args         []string
		code         int
		stdout       string
	}{
		{name: "syntax error", config: "project \"p\" {\n  read_only = [\"gen.hcl\"\n}\n", code: 1},
		{name: "read_only not a list", config: "project \"p\" {\n  read_only = \"gen.hcl\"\n}\n", code: 1},
		{name: "check", config: "project \"p\" {\n  read_only = [\"gen.hcl\"]\n}\n", args: []string{"--check"}, code: 1, stdout: "model.hcl\n"},
		{name: "read-only kept", config: "project \"p\" {\n  read_only = [\"gen.hcl\"]\n}\n", code: 0, stdout: "model.hcl\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{"dg.hcl": tt.config, "gen.hcl": messy, "model.hcl": messy}
			for name, src := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr bytes.Buffer
			if code := run(append([]string{"fmt", "-C", dir}, tt.args...), &stdout, &stderr); code != tt.code {
				t.Fatalf("exit code %d, want %d; stderr:\n%s", code, tt.code, stderr.String())
			}
			if stdout.String() != tt.stdout {
				t.Errorf("stdout %q, want %q", stdout.String(), tt.stdout)
			}
			if got, _ := os.ReadFile(filepath.Join(dir, "gen.hcl")); string(got) != messy {
				t.Errorf("gen.hcl was rewritten:\n%s", got)
			}
			if got, _ := os.ReadFile(filepath.Join(dir, "model.hcl")); (string(got) == messy) != (tt.code != 0) {
				t.Errorf("model.hcl after fmt:\n%s", got)
			}
		})
	}
}

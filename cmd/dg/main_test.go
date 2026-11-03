package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "model.hcl"), []byte("node \"a\" {\n  kind = \"nope\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "hello.svg")
	tests := []struct {
		name       string
		args       []string
		chromium   bool
		code       int
		stderr     string
		file, want string
	}{
		{name: "no command", code: 1, stderr: "usage"},
		{name: "unknown command", args: []string{"draw"}, code: 1, stderr: `unknown command "draw"`},
		{name: "unknown format", args: []string{"export", "--format", "pdf"}, code: 1, stderr: `unknown format "pdf"`},
		{name: "missing project directory", args: []string{"export", "-C", "does-not-exist"}, code: 1, stderr: "does-not-exist"},
		{name: "project with errors", args: []string{"export", "-C", broken}, code: 1, stderr: `model.hcl:1:1: error: Undefined kind: Node "a" has undefined kind "nope".`},
		{
			name:     "export to a file",
			args:     []string{"export", "-C", "../../examples/hello", "--out", out},
			chromium: true,
			file:     out,
			want:     "../../testdata/export/hello.svg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.chromium && testing.Short() {
				t.Skip("starts Chromium")
			}
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != tt.code {
				t.Fatalf("exit code %d, want %d; stderr:\n%s", code, tt.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.stderr) {
				t.Errorf("stderr %q does not contain %q", stderr.String(), tt.stderr)
			}
			if tt.file == "" {
				return
			}
			got, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs from %s", tt.file, tt.want)
			}
		})
	}
}

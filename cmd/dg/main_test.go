package main

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/png"
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
	tmp := t.TempDir()
	out := filepath.Join(tmp, "hello.svg")
	allDir := filepath.Join(tmp, "all")
	tests := []struct {
		name     string
		args     []string
		chromium bool
		code     int
		stderr   string
		// files maps each file the command writes to its golden file, or to "" for a PNG or GIF,
		// which is decoded instead.
		files map[string]string
	}{
		{name: "no command", code: 1, stderr: "usage"},
		{name: "version", args: []string{"--version"}, code: 0},
		{name: "unknown command", args: []string{"draw"}, code: 1, stderr: `unknown command "draw"`},
		{name: "scale out of range", args: []string{"export", "--format", "png", "--scale", "0.5"}, code: 1, stderr: "--scale is a number from 1 to 8"},
		{name: "scale of a vector format", args: []string{"export", "--scale", "2"}, code: 1, stderr: "--scale needs --format png"},
		{name: "unknown format", args: []string{"export", "--format", "pdf"}, code: 1, stderr: `unknown format "pdf"`},
		{name: "missing project directory", args: []string{"export", "-C", "does-not-exist"}, code: 2, stderr: "does-not-exist"},
		{name: "check warnings", args: []string{"check", "-C", "../../testdata/check/no-model"}, code: 0},
		{name: "check strict warnings", args: []string{"check", "--strict", "-C", "../../testdata/check/no-model"}, code: 1},
		{name: "check strict information", args: []string{"check", "--strict", "-C", "../../testdata/check/overrides"}, code: 0},
		{name: "check missing project directory", args: []string{"check", "-C", "does-not-exist"}, code: 2, stderr: "does-not-exist"},
		{name: "check unexpected argument", args: []string{"check", "extra"}, code: 1, stderr: `unexpected argument "extra"`},
		{name: "fmt syntax error", args: []string{"fmt", "-C", "../../testdata/check/syntax"}, code: 1, stderr: "model.hcl:2:11: error: Invalid expression"},
		{name: "serve refuses a non-loopback address", args: []string{"serve", "-C", "../../examples/hello", "-addr", "0.0.0.0:0", "-open=false"}, code: 1, stderr: "not a loopback address"},
		{name: "export undefined view", args: []string{"export", "-C", "../../examples/hello", "--view", "nope"}, code: 1, stderr: `view "nope" is not defined (views: main)`},
		{name: "project with errors", args: []string{"export", "-C", broken}, code: 1, stderr: `model.hcl:1:1: error: Undefined kind: Node "a" has undefined kind "nope".`},
		{
			name:     "export to a file",
			args:     []string{"export", "-C", "../../examples/hello", "--out", out},
			chromium: true,
			files:    map[string]string{out: "../../testdata/export/hello.svg"},
		},
		{
			name:     "export a PNG",
			args:     []string{"export", "-C", "../../examples/hello", "--format", "png", "--out", filepath.Join(tmp, "hello.png")},
			chromium: true,
			files:    map[string]string{filepath.Join(tmp, "hello.png"): ""},
		},
		{
			name:     "export a GIF",
			args:     []string{"export", "-C", "../../examples/c4-container", "--view", "sign-in", "--format", "gif", "--out", filepath.Join(tmp, "sign-in.gif")},
			chromium: true,
			files:    map[string]string{filepath.Join(tmp, "sign-in.gif"): ""},
		},
		{
			name:     "export every view",
			args:     []string{"export", "-C", "../../examples/c4-container", "--all", "--out", allDir},
			chromium: true,
			files: map[string]string{
				filepath.Join(allDir, "main.svg"):    "../../testdata/export/c4-container.svg",
				filepath.Join(allDir, "pinned.svg"):  "../../testdata/export/c4-container-pinned.svg",
				filepath.Join(allDir, "sign-in.svg"): "../../testdata/export/c4-container-sign-in.svg",
			},
		},
		{name: "export every view needs a directory", args: []string{"export", "-C", "../../examples/hello", "--all"}, code: 1, stderr: "--all needs --out"},
		{name: "export every view or one", args: []string{"export", "-C", "../../examples/hello", "--all", "--view", "main", "--out", allDir}, code: 1, stderr: "--all and --view exclude each other"},
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
			for file, golden := range tt.files {
				got, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if golden == "" {
					if _, format, err := image.Decode(bytes.NewReader(got)); err != nil || format != strings.TrimPrefix(filepath.Ext(file), ".") {
						t.Errorf("%s is not a %s picture: %v", file, filepath.Ext(file), err)
					}
					continue
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s differs from %s", file, golden)
				}
			}
		})
	}
}

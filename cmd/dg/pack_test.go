package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// packRepo returns the URL of a new git repository of files, tagged v1.0.0.
func packRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	config := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(config, []byte("[user]\n\tname = t\n\temail = t@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "-A"}, {"commit", "--quiet", "-m", "c"}, {"tag", "v1.0.0"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	return "file://" + filepath.ToSlash(dir)
}

func TestPack(t *testing.T) {
	const spec = "kind \"vpc\" {\n  extends = \"group\"\n}\n"
	licensed := packRepo(t, map[string]string{"spec.hcl": spec, "LICENSE": "l"})
	bare := packRepo(t, map[string]string{"spec.hcl": spec})
	broken := packRepo(t, map[string]string{"spec.hcl": spec + "style \"x\" {}\n"})
	dir := t.TempDir()
	model := "node \"n\" {\n  kind = \"aws-vpc\"\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "model.hcl"), []byte(model), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name           string
		args           []string
		code           int
		stdout, stderr string
	}{
		{name: "help", args: []string{"--help"}, stdout: "dg pack add"},
		{name: "nothing to do", code: 1, stderr: "usage: dg pack"},
		{name: "unknown", args: []string{"frob"}, code: 1, stderr: "unknown command"},
		{name: "too many", args: []string{"remove", "a", "b"}, code: 1, stderr: "wrong number"},
		{name: "too few", args: []string{"add"}, code: 1, stderr: "wrong number"},
		{name: "a source that is an option", args: []string{"add", "-C", dir, "--", "-x"}, code: 1, stderr: "neither"},
		{name: "licenses, with the flags last", args: []string{"add", licensed, "--as", "aws", "-C", dir}, stdout: "license: packs/aws/LICENSE\n"},
		{name: "no license", args: []string{"add", "-as", "bare", "-C", dir, bare}, stderr: `warning: the pack "bare" has no LICENSE`},
		{name: "same name", args: []string{"add", licensed, "--as", "AWS", "-C", dir}, code: 1, stderr: "differs only by case"},
		{name: "check after add", args: []string{"add", broken, "--as", "broken", "-C", dir}, code: 1, stdout: "spec.hcl:4:1: error"},
		{name: "remove", args: []string{"remove", "broken", "-C", dir}},
		{name: "update", args: []string{"update", "aws", "v1.0.0", "-C", dir}, stdout: "license: packs/aws/LICENSE\n"},
		{name: "install", args: []string{"install", "-C", dir}, stdout: "installed aws\ninstalled bare\n"},
		{name: "check after remove", args: []string{"remove", "aws", "-C", dir}, code: 1, stdout: "model.hcl:1:1: error: Undefined kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(append([]string{"pack"}, tt.args...), &stdout, &stderr)
			if code != tt.code {
				t.Errorf("exit %d, want %d; %s%s", code, tt.code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.stdout) {
				t.Errorf("stdout %q, want %q", stdout.String(), tt.stdout)
			}
			if !strings.Contains(stderr.String(), tt.stderr) {
				t.Errorf("stderr %q, want %q", stderr.String(), tt.stderr)
			}
		})
	}
}

// FuzzPackArgs parses dg pack's command lines, with arguments separated by NUL; what the arguments
// themselves may do is internal/pack's FuzzArguments.
func FuzzPackArgs(f *testing.F) {
	for _, seed := range []string{
		"add\x00example.com/a@v1\x00--as\x00aws\x00-C\x00dir", "add\x00--\x00-x", "add\x00--as", "update\x00aws\x00v1", "update\x00a\x00b\x00c",
		"remove\x00aws", "install", "install\x00x", "search\x00text", "frob", "add\x00--upload-pack=x",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		args := strings.Split(line, "\x00")
		pos, _, as, err := parsePack(args, io.Discard)
		if err != nil {
			return
		}
		if count := packCounts[args[0]]; len(pos) < count[0] || len(pos) > count[1] {
			t.Errorf("%q: positional arguments %q", args, pos)
		}
		if as != "" && args[0] != "add" {
			t.Errorf("%q: --as %q", args, as)
		}
	})
}

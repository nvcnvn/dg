package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestCheckGolden runs dg check, as text and as JSON, on every project under testdata/check and
// compares the output and exit code with the project's golden files.
func TestCheckGolden(t *testing.T) {
	cases, err := filepath.Glob("../../testdata/check/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range cases {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			abs, err := filepath.Abs(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(abs)
			for _, c := range []struct{ golden, flag string }{{"check.golden", "--strict=false"}, {"check.json.golden", "--json"}} {
				var stdout, stderr bytes.Buffer
				code := run([]string{"check", c.flag}, &stdout, &stderr)
				got := fmt.Appendf(stdout.Bytes(), "%sexit %d\n", stderr.String(), code)
				golden(t, filepath.Join(abs, c.golden), got)
			}
		})
	}
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
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
}

func TestExamplesCheckStrict(t *testing.T) {
	dirs, err := filepath.Glob("../../examples/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"check", "--strict", "-C", dir}, &stdout, &stderr); code != 0 {
			t.Errorf("dg check --strict %s: exit code %d\n%s%s", dir, code, stdout.String(), stderr.String())
		}
	}
}

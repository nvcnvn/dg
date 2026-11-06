package fsys

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFilesKeepsModeAndLinks(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "private.hcl")
	target := filepath.Join(dir, "target.hcl")
	link := filepath.Join(dir, "link.hcl")
	fresh := filepath.Join(dir, "fresh.hcl")
	for _, f := range []string{private, target} {
		if err := os.WriteFile(f, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("target.hcl", link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFiles(map[string][]byte{private: []byte("a"), link: []byte("b"), fresh: []byte("c")}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{private: 0o600, target: 0o600, fresh: 0o644} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		// Windows keeps no permission bits: Go reports every writable file as 0666.
		if runtime.GOOS != "windows" && info.Mode().Perm() != want {
			t.Errorf("%s has mode %v, want %v", filepath.Base(path), info.Mode().Perm(), want)
		}
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link.hcl is no longer a symbolic link: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "b" {
		t.Errorf("target.hcl holds %q, want the write through the link", got)
	}
}

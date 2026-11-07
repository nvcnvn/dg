package ops

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/project"
)

func testPack(name string) Pack {
	return Pack{
		Name:  name,
		Entry: project.LockEntry{Source: "github.com/dg-packs/" + name, Version: "v1.2.0", Commit: strings.Repeat("a", 40), Hash: "sha256:" + strings.Repeat("b", 64)},
		Files: fstest.MapFS{"spec.hcl": {Data: []byte("kind \"k\" {}\n")}},
	}
}

// TestPackBlocks adds a pack to each dg.hcl under testdata/ops/pack, compares the result with its
// golden file, and removes the pack again: add then remove restores dg.hcl byte for byte (FR-21).
// A case with no dg.hcl holds only dg.hcl.golden.
func TestPackBlocks(t *testing.T) {
	cases, err := filepath.Glob("../../testdata/ops/pack/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	for _, dir := range cases {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			before, err := os.ReadFile(filepath.Join(dir, "dg.hcl"))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			exists := err == nil
			root := t.TempDir()
			config := filepath.Join(root, "dg.hcl")
			if exists {
				if err := os.WriteFile(config, before, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := AddPack(fsys.Disk(root), testPack("aws")); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(config)
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join(dir, "dg.hcl.golden")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
			if p, diags, err := project.Load(root); err != nil {
				t.Fatal(err)
			} else if p.ReadOnlyUnknown || len(p.Installed) == 0 {
				t.Errorf("after add: %v", diags)
			}
			if err := RemovePack(fsys.Disk(root), "aws"); err != nil {
				t.Fatal(err)
			}
			got, err = os.ReadFile(config)
			switch {
			case exists && (err != nil || !bytes.Equal(got, before)):
				t.Errorf("after remove: %q, %v; want %q", got, err, before)
			case !exists && !errors.Is(err, fs.ErrNotExist):
				t.Errorf("after remove there is a dg.hcl: %q, %v", got, err)
			}
			if entries, err := os.ReadDir(root); err != nil || exists && len(entries) != 1 || !exists && len(entries) != 0 {
				t.Errorf("left %v, %v", entries, err)
			}
		})
	}
}

// TestPackOrder removes packs in each order, and the pack before the last leaves dg.lock with the
// other's entry.
func TestPackOrder(t *testing.T) {
	const start = "project \"p\" {}\n"
	for _, tt := range []struct {
		name  string
		order []string
	}{{"added last, first", []string{"b", "a"}}, {"added first, first", []string{"a", "b"}}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			disk := fsys.Disk(root)
			config, lock := filepath.Join(root, "dg.hcl"), filepath.Join(root, "dg.lock")
			if err := os.WriteFile(config, []byte(start), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"a", "b"} {
				if err := AddPack(disk, testPack(name)); err != nil {
					t.Fatal(err)
				}
			}
			if err := RemovePack(disk, tt.order[0]); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(lock)
			if err != nil {
				t.Fatal(err)
			}
			want := (&project.Lock{Packs: map[string]project.LockEntry{tt.order[1]: testPack(tt.order[1]).Entry}}).Encode()
			if !bytes.Equal(data, want) {
				t.Errorf("got %s", data)
			}
			if data, _ := os.ReadFile(config); strings.Count(string(data), "\n\n") != 0 {
				t.Errorf("a blank line is left: %q", data)
			}
			if err := RemovePack(disk, tt.order[1]); err != nil {
				t.Fatal(err)
			}
			if data, _ := os.ReadFile(config); string(data) != start {
				t.Errorf("got %q", data)
			}
			if _, err := os.Stat(lock); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("dg.lock is left: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "packs")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("packs is left: %v", err)
			}
		})
	}
}

// TestRemovePackKeeps checks that remove leaves what is not the pack's, and refuses what is not one.
func TestRemovePackKeeps(t *testing.T) {
	root := t.TempDir()
	disk := fsys.Disk(root)
	if err := os.MkdirAll(filepath.Join(root, "packs", "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dg.hcl"), []byte("project \"p\" {\n  packs = [\"packs/local\"]\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddPack(disk, testPack("aws")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "packs", "aws", "stray"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemovePack(disk, "aws"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "packs", "aws")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the directory is left: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "packs", "local")); err != nil {
		t.Error(err)
	}
	for _, name := range []string{"aws", "local", "../local", "generic"} {
		if err := RemovePack(disk, name); err == nil {
			t.Errorf("removed %q", name)
		}
	}
	lock := (&project.Lock{Packs: map[string]project.LockEntry{"local": testPack("local").Entry}}).Encode()
	if err := os.WriteFile(filepath.Join(root, "dg.lock"), lock, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemovePack(disk, "local"); err == nil || !strings.Contains(err.Error(), "local pack") {
		t.Errorf("got %v", err)
	}
}

// TestPackClashRepair removes a pack from a project whose dg.hcl has an error at its block.
func TestPackClashRepair(t *testing.T) {
	root := t.TempDir()
	disk := fsys.Disk(root)
	if err := AddPack(disk, testPack("aws")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "packs")); err != nil {
		t.Fatal(err)
	}
	p, _, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !p.ConfigInvalid || p.ReadOnlyUnknown {
		t.Fatalf("ConfigInvalid %v, ReadOnlyUnknown %v", p.ConfigInvalid, p.ReadOnlyUnknown)
	}
	if err := RemovePack(disk, "aws"); err != nil {
		t.Fatal(err)
	}
}

// TestPackChecksUnderLock checks that the writes refuse by themselves what the checks before a
// fetch refuse, since the project can change during one.
func TestPackChecksUnderLock(t *testing.T) {
	root := t.TempDir()
	disk := fsys.Disk(root)
	if err := AddPack(disk, testPack("aws")); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, want string }{
		{"aws", "already installed"},
		{"AWS", "differs only by case"},
		{"generic", "built-in"},
	} {
		if err := AddPack(disk, testPack(tt.name)); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got %v", tt.name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "dg.hcl"), []byte("project \"p\" {\n  packs = [\"packs/local\"]\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "packs", "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InstallPacks(disk, []Pack{testPack("local")}); err == nil || !strings.Contains(err.Error(), "local pack") {
		t.Errorf("install over a local pack: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "packs", "local")); err != nil || len(entries) != 0 {
		t.Errorf("wrote %v, %v", entries, err)
	}
}

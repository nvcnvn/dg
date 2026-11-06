package fsys

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

const model, layout = "model.hcl", "views/main.layout.yaml"

// projects opens a project holding model.hcl and views/main.layout.yaml, on disk and in memory.
var projects = []struct {
	name string
	open func(t *testing.T) FS
}{
	{"disk", func(t *testing.T) FS {
		dir := t.TempDir()
		err := errors.Join(os.Mkdir(filepath.Join(dir, "views"), 0o755),
			os.WriteFile(filepath.Join(dir, model), []byte("model"), 0o644),
			os.WriteFile(filepath.Join(dir, filepath.FromSlash(layout)), []byte("layout"), 0o644))
		if err != nil {
			t.Fatal(err)
		}
		return Disk(dir)
	}},
	{"memory", func(t *testing.T) FS {
		m, err := NewMemory(map[string][]byte{model: []byte("model"), layout: []byte("layout")})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}},
}

func tree(t *testing.T, fsys fs.FS) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		out[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestWrite checks that a write does the same on disk and in memory: what it leaves, and that a
// refused write leaves everything as it was.
func TestWrite(t *testing.T) {
	original := map[string]string{model: "model", layout: "layout"}
	tests := []struct {
		name    string
		files   map[string][]byte
		want    map[string]string
		refused bool
	}{
		{"nothing", nil, original, false},
		{"replace", map[string][]byte{model: []byte("new")}, map[string]string{model: "new", layout: "layout"}, false},
		{"empty, not removed", map[string][]byte{model: {}}, map[string]string{model: "", layout: "layout"}, false},
		{"new directories", map[string][]byte{"a/b/c.hcl": []byte("c")}, map[string]string{model: "model", layout: "layout", "a/b/c.hcl": "c"}, false},
		{"remove", map[string][]byte{model: nil}, map[string]string{layout: "layout"}, false},
		{"remove what is not there", map[string][]byte{"gone.hcl": nil}, original, false},
		{"replace and remove", map[string][]byte{model: nil, layout: []byte("new")}, map[string]string{layout: "new"}, false},
		{"through a file", map[string][]byte{"a.hcl": []byte("a"), model + "/b.hcl": []byte("b")}, original, true},
		{"over a directory", map[string][]byte{"a.hcl": []byte("a"), "views": []byte("b")}, original, true},
		{"out of the project", map[string][]byte{"a.hcl": []byte("a"), "../b.hcl": []byte("b")}, original, true},
	}
	for _, tt := range tests {
		for _, p := range projects {
			t.Run(tt.name+"/"+p.name, func(t *testing.T) {
				files := p.open(t)
				if err := files.Write(tt.files); (err != nil) != tt.refused {
					t.Fatalf("Write: %v", err)
				}
				if got := tree(t, files); !maps.Equal(got, tt.want) {
					t.Errorf("got %v, want %v", got, tt.want)
				}
			})
		}
	}
}

func TestRead(t *testing.T) {
	for _, p := range projects {
		t.Run(p.name, func(t *testing.T) {
			files := p.open(t)
			if _, err := fs.ReadFile(files, "gone.hcl"); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("a missing file: %v", err)
			}
			if _, err := fs.ReadFile(files, "../outside"); err == nil {
				t.Error("read a path out of the project")
			}
			if err := files.Write(map[string][]byte{"b.hcl": {}, "a.hcl": {}}); err != nil {
				t.Fatal(err)
			}
			entries, err := fs.ReadDir(files, ".")
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			if want := []string{"a.hcl", "b.hcl", model, "views"}; !slices.Equal(names, want) {
				t.Errorf("listed %v, want %v", names, want)
			}
		})
	}
}

// TestLock checks that writers hold the lock one at a time: the race detector reports them if not.
func TestLock(t *testing.T) {
	for _, p := range projects {
		t.Run(p.name, func(t *testing.T) {
			files := p.open(t)
			var wg sync.WaitGroup
			count := 0
			for range 8 {
				wg.Go(func() {
					unlock, err := files.Lock()
					if err != nil {
						t.Error(err)
						return
					}
					defer unlock()
					count++
				})
			}
			wg.Wait()
			if count != 8 {
				t.Errorf("%d writers ran, want 8", count)
			}
		})
	}
}

func TestMemoryKeepsItsOwnCopy(t *testing.T) {
	data := []byte("model")
	m, err := NewMemory(map[string][]byte{model: data})
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	if got := tree(t, m)[model]; got != "model" {
		t.Errorf("holds %q", got)
	}
}

// TestDiskLinkOutside checks that a write through a link out of the project is refused before
// anything is written or created.
func TestDiskLinkOutside(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "views")); err != nil {
		t.Fatal(err)
	}
	err := Disk(dir).Write(map[string][]byte{model: []byte("x"), "views/new/main.layout.yaml": []byte("y")})
	if err == nil || !strings.Contains(err.Error(), "links outside the project") {
		t.Fatalf("Write: %v", err)
	}
	for _, d := range []string{dir, outside} {
		entries, _ := os.ReadDir(d)
		if len(entries) > 1 || d == outside && len(entries) > 0 {
			t.Errorf("%s holds %v", d, entries)
		}
	}
}

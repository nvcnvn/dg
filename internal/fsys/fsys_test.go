package fsys

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
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
		{"remove a directory", map[string][]byte{"a.hcl": []byte("a"), "views": nil}, original, true},
		{"remove through a file", map[string][]byte{"a.hcl": []byte("a"), model + "/b.hcl": nil}, map[string]string{model: "model", layout: "layout", "a.hcl": "a"}, false},
		{"under a file it removes", map[string][]byte{model: nil, model + "/b.hcl": []byte("b")}, original, true},
		{"over a directory it empties", map[string][]byte{"views": []byte("a"), layout: nil}, original, true},
		{"a file and what is under it", map[string][]byte{"a": []byte("a"), "a/b.hcl": []byte("b")}, original, true},
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
			if _, err := fs.ReadFile(files, "../outside"); !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("a path out of the project: %v", err)
			}
			// Windows reports a path through a file as a missing one; other systems do not.
			if _, err := fs.ReadFile(files, model+"/b.hcl"); err == nil || errors.Is(err, fs.ErrNotExist) != errors.Is(syscall.ENOTDIR, fs.ErrNotExist) {
				t.Errorf("a path through a file: %v", err)
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

func TestLock(t *testing.T) {
	for _, p := range projects {
		t.Run(p.name, func(t *testing.T) {
			files := p.open(t)
			unlock, err := files.Lock()
			if err != nil {
				t.Fatal(err)
			}
			second := make(chan func(), 1)
			go func() {
				unlock, err := files.Lock()
				if err != nil {
					t.Error(err)
				}
				second <- unlock
			}()
			select {
			case <-second:
				t.Fatal("a second writer got the lock")
			case <-time.After(50 * time.Millisecond):
			}
			unlock()
			select {
			case unlock := <-second:
				unlock()
			case <-time.After(5 * time.Second):
				t.Fatal("the second writer did not get the lock")
			}
		})
	}
}

// TestDiskStat checks that a file is described without being opened: a file nobody may read
// still exists.
func TestDiskStat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, model)
	if err := errors.Join(os.WriteFile(path, nil, 0o644), os.Chmod(path, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(Disk(dir), model); err != nil {
		t.Error(err)
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

// TestWriteDirsStale checks that a stale directory that cannot go stops the write before anything
// changes, so dg.hcl and a pack's files cannot end up of different versions.
func TestWriteDirsStale(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root removes what permissions keep")
	}
	root := t.TempDir()
	stale := filepath.Join(root, "packs", ".dg-old-a")
	config := filepath.Join(root, "dg.hcl")
	if err := errors.Join(os.MkdirAll(filepath.Join(root, "packs", "a"), 0o755), os.MkdirAll(stale, 0o755),
		os.WriteFile(config, []byte("old"), 0o644), os.WriteFile(filepath.Join(stale, "x"), nil, 0o644), os.Chmod(stale, 0o500)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(stale, 0o755) })
	err := Disk(root).WriteDirs(map[string][]byte{"dg.hcl": []byte("new")}, map[string]map[string][]byte{"packs/a": {"new.svg": []byte("n")}})
	if err == nil {
		t.Fatal("no error")
	}
	if data, err := os.ReadFile(config); err != nil || string(data) != "old" {
		t.Errorf("dg.hcl: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "packs", "a", "new.svg")); err == nil {
		t.Error("the new content was written")
	}
}

// TestWriteDirs checks what replacing a directory leaves on disk, whatever the old one held.
func TestWriteDirs(t *testing.T) {
	const dir = "packs/a"
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := func(data string) map[string]map[string][]byte {
		return map[string]map[string][]byte{dir: {"new.svg": []byte(data)}}
	}
	tests := []struct {
		name    string
		before  map[string]string
		links   map[string]string
		files   map[string][]byte
		dirs    map[string]map[string][]byte
		want    map[string]string
		refused bool
	}{
		{name: "new, with its parent", dirs: file("n"), want: map[string]string{"packs/a/new.svg": "n"}},
		{
			name:   "replaced, leaving nothing of the old",
			before: map[string]string{"packs/a/old.svg": "o", "packs/a/sub/old.svg": "o", "packs/b/keep": "k"},
			dirs:   file("n"),
			want:   map[string]string{"packs/a/new.svg": "n", "packs/b/keep": "k"},
		},
		{
			name:   "removed, with files",
			before: map[string]string{"packs/a/old.svg": "o", "dg.lock": "l", "dg.hcl": "h"},
			files:  map[string][]byte{"dg.lock": nil, "dg.hcl": []byte("new")},
			dirs:   map[string]map[string][]byte{dir: nil},
			want:   map[string]string{"dg.hcl": "new"},
		},
		{
			name:   "removed, keeping a parent in use",
			before: map[string]string{"packs/a/old.svg": "o", "packs/b/keep": "k"},
			dirs:   map[string]map[string][]byte{dir: nil},
			want:   map[string]string{"packs/b/keep": "k"},
		},
		{
			name:   "what a crash left",
			before: map[string]string{"packs/.dg-new-a/stale.svg": "s", "packs/.dg-old-a/stale.svg": "s", "packs/a/old.svg": "o"},
			dirs:   file("n"),
			want:   map[string]string{"packs/a/new.svg": "n"},
		},
		{name: "removing what is not there", dirs: map[string]map[string][]byte{dir: nil}, want: map[string]string{}},
		{
			name:   "a file where a directory goes",
			before: map[string]string{"packs/a/x.svg": "o"},
			dirs:   map[string]map[string][]byte{dir: {"x.svg/y.svg": []byte("n")}},
			want:   map[string]string{"packs/a/x.svg/y.svg": "n"},
		},
		{
			name:   "a link in the old directory goes, not through",
			before: map[string]string{"packs/a/old.svg": "o"},
			links:  map[string]string{"packs/a/out": outside},
			dirs:   map[string]map[string][]byte{dir: {"out/secret": []byte("n")}},
			want:   map[string]string{"packs/a/out/secret": "n"},
		},
		{
			name:   "a directory that is a link",
			before: map[string]string{"packs/local/keep": "k"},
			links:  map[string]string{"packs/a": "local"},
			dirs:   file("n"),
			want:   map[string]string{"packs/local/keep": "k", "packs/a/new.svg": "n"},
		},
		{
			name:    "a refused file refuses the directory",
			before:  map[string]string{"packs/a/old.svg": "o", "sub/x": "x"},
			files:   map[string][]byte{"sub": []byte("over a directory")},
			dirs:    file("n"),
			want:    map[string]string{"packs/a/old.svg": "o", "sub/x": "x"},
			refused: true,
		},
		{
			name:    "a parent that is a file",
			before:  map[string]string{"packs": "file"},
			dirs:    file("n"),
			want:    map[string]string{"packs": "file"},
			refused: true,
		},
		{
			name:    "out of the project through a link",
			links:   map[string]string{"packs": outside},
			dirs:    file("n"),
			refused: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, data := range tt.before {
				p := filepath.Join(root, filepath.FromSlash(name))
				if err := errors.Join(os.MkdirAll(filepath.Dir(p), 0o755), os.WriteFile(p, []byte(data), 0o644)); err != nil {
					t.Fatal(err)
				}
			}
			for name, target := range tt.links {
				p := filepath.Join(root, filepath.FromSlash(name))
				if err := errors.Join(os.MkdirAll(filepath.Dir(p), 0o755), os.Symlink(target, p)); err != nil {
					t.Skip(err)
				}
			}
			err := Disk(root).WriteDirs(tt.files, tt.dirs)
			if (err != nil) != tt.refused {
				t.Fatalf("WriteDirs: %v", err)
			}
			if data, err := os.ReadFile(secret); err != nil || string(data) != "secret" {
				t.Errorf("what is outside changed: %q, %v", data, err)
			}
			if tt.want == nil {
				return
			}
			got := map[string]string{}
			err = filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(root, name)
				if d.Type()&fs.ModeSymlink != 0 {
					return nil
				}
				data, err := os.ReadFile(name)
				got[filepath.ToSlash(rel)] = string(data)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			entries, _ := os.ReadDir(filepath.Join(root, "packs"))
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".") {
					t.Errorf("left %s", e.Name())
				}
			}
		})
	}
}

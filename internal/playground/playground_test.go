package playground

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/project"
)

const testModel = "node \"a\" {\n  label = \"Alpha\"\n}\n"

// visitor is what the page gets from a playground.
type visitor struct {
	p        *Playground
	messages []json.RawMessage
}

func open(t *testing.T) *visitor {
	t.Helper()
	pg := &visitor{}
	pg.p = New(func(data []byte) { pg.messages = append(pg.messages, bytes.Clone(data)) })
	return pg
}

func (pg *visitor) receive(m any) {
	data, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	pg.p.Receive(context.Background(), data)
}

// got returns the messages since the last call, by type, the last of each.
func (pg *visitor) got(t *testing.T) (types []string, last map[string]message) {
	t.Helper()
	types, last = []string{}, map[string]message{}
	for _, data := range pg.messages {
		var m message
		_ = json.Unmarshal(data, &m)
		types = append(types, m.Type)
		last[m.Type] = m
	}
	pg.messages = nil
	return types, last
}

func load(files map[string]string) map[string]any {
	return map[string]any{"type": "load", "files": files}
}

func TestLoad(t *testing.T) {
	pg := open(t)
	pg.p.Ready()
	pg.receive(load(map[string]string{ModelFile: testModel}))
	types, last := pg.got(t)
	if want := []string{"ready", "files", "loaded"}; !slices.Equal(types, want) {
		t.Errorf("got %v, want %v", types, want)
	}
	if got := last["files"].Files; !maps.Equal(got, map[string]string{ModelFile: testModel}) {
		t.Errorf("files: %v", got)
	}
	for _, files := range []map[string]string{{"dg.hcl": ""}, {"views/a/b.layout.yaml": ""}, {"../x": ""}, {}} {
		pg.receive(load(files))
		if types, _ := pg.got(t); !slices.Equal(types, []string{"error"}) {
			t.Errorf("%v: got %v", files, types)
		}
	}
}

// TestViewGestures checks that what hiding a node and making a view write can be loaded again.
func TestViewGestures(t *testing.T) {
	pg := open(t)
	pg.receive(load(map[string]string{ModelFile: testModel}))
	pg.got(t)
	pg.receive(map[string]any{"type": "op", "op": map[string]any{"op": "create_view", "id": "other", "title": "Other"}})
	pg.receive(map[string]any{"type": "op", "op": map[string]any{"op": "move", "view": "other", "id": "a", "x": 40, "y": 40}})
	pg.receive(map[string]any{"type": "op", "op": map[string]any{"op": "hide_node", "view": "main", "id": "a"}})
	_, last := pg.got(t)
	files := last["files"].Files
	if _, ok := files[ViewsFile]; !ok || files["views/other.layout.yaml"] == "" {
		t.Fatalf("files: %v", files)
	}
	pg.receive(load(files))
	if types, last := pg.got(t); !slices.Equal(types, []string{"files", "loaded"}) || !maps.Equal(last["files"].Files, files) {
		t.Errorf("got %v %v", types, last)
	}
}

// TestSameInMemory checks that each example of the menu resolves in memory, without its dg.hcl, as
// it does on disk: the geometry and the SVG are functions of that view.
func TestSameInMemory(t *testing.T) {
	for _, name := range []string{"hello", "class", "er"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("../../examples", name)
			want, _, err := project.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{}
			for _, f := range want.Files {
				if f.Path != project.ProjectFile {
					if files[f.Path], err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path))); err != nil {
						t.Fatal(err)
					}
				}
			}
			memory, err := fsys.NewMemory(files)
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := project.LoadFS(memory)
			if err != nil {
				t.Fatal(err)
			}
			a, _ := got.Resolve("main")
			b, _ := want.Resolve("main")
			x, err := json.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			y, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(x, y) {
				t.Errorf("in memory %s, on disk %s", x, y)
			}
		})
	}
}

func TestNoProject(t *testing.T) {
	for _, m := range []map[string]any{{"type": "select-view"}, {"type": "write-file", "path": ModelFile, "text": ""}, {"type": "zip"}} {
		pg := open(t)
		pg.receive(m)
		if types, _ := pg.got(t); !slices.Equal(types, []string{"error"}) {
			t.Errorf("%v: got %v", m, types)
		}
	}
}

// TestWriteFile checks that a save from the text editor redraws and is not sent back, and is
// taken as an outside save: undo keeps it.
func TestWriteFile(t *testing.T) {
	pg := open(t)
	pg.receive(load(map[string]string{ModelFile: testModel}))
	pg.receive(map[string]any{"type": "select-view"})
	pg.got(t)

	pg.receive(map[string]any{"type": "op", "seq": 1, "op": map[string]any{"op": "set_label", "id": "a", "label": "Beta"}})
	types, last := pg.got(t)
	if !slices.Contains(types, "files") || !strings.Contains(last["files"].Files[ModelFile], "Beta") {
		t.Fatalf("an operation: %v %v", types, last["files"])
	}

	text := last["files"].Files[ModelFile] + "\nnode \"b\" {}\n"
	pg.receive(map[string]any{"type": "write-file", "path": ModelFile, "text": text})
	types, _ = pg.got(t)
	if want := []string{"model", "snapshot", "diagnostics"}; !slices.Equal(types, want) {
		t.Errorf("a save: got %v, want %v", types, want)
	}

	pg.receive(map[string]any{"type": "undo", "seq": 2})
	_, last = pg.got(t)
	got := last["files"].Files[ModelFile]
	if !strings.Contains(got, "Alpha") || !strings.Contains(got, `node "b"`) {
		t.Errorf("undo after a save: %q", got)
	}

	for _, m := range []map[string]any{
		{"type": "write-file", "path": LayoutFile, "text": ""},
		{"type": "write-file", "path": "dg.hcl", "text": ""},
		{"type": "write-file", "path": ModelFile},
	} {
		pg.receive(m)
		if types, _ := pg.got(t); !slices.Equal(types, []string{"error"}) {
			t.Errorf("%v: got %v", m, types)
		}
	}
}

// TestNewFile checks that the canvas's first move shows the page the layout file it creates.
func TestNewFile(t *testing.T) {
	pg := open(t)
	pg.receive(load(map[string]string{ModelFile: testModel}))
	pg.got(t)
	pg.receive(map[string]any{"type": "op", "op": map[string]any{"op": "move", "view": "main", "id": "a", "x": 40, "y": 40}})
	_, last := pg.got(t)
	if !strings.Contains(last["files"].Files[LayoutFile], "x: 40") {
		t.Errorf("files: %v", last["files"].Files)
	}
}

func TestZip(t *testing.T) {
	files := map[string]string{ModelFile: testModel, StyleFile: "", LayoutFile: "version: 1\n"}
	archive := func() []byte {
		pg := open(t)
		pg.receive(load(files))
		pg.got(t)
		pg.receive(map[string]any{"type": "zip", "seq": 5})
		_, last := pg.got(t)
		if last["zip"].Seq != 5 {
			t.Errorf("seq %d", last["zip"].Seq)
		}
		data, err := base64.StdEncoding.DecodeString(last["zip"].Data)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	data := archive()
	if !bytes.Equal(data, archive()) {
		t.Error("two archives of the same files differ")
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		text, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		got[f.Name] = string(text)
	}
	if !maps.Equal(got, files) {
		t.Errorf("got %v", got)
	}
}

// TestExamples keeps the page's menu equal to the examples a playground project can hold: a model,
// perhaps a style and the layout of main, and a dg.hcl without fields.
func TestExamples(t *testing.T) {
	const examples = "../../examples"
	entries, err := os.ReadDir(examples)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, e := range entries {
		if fits(t, filepath.Join(examples, e.Name())) {
			want = append(want, e.Name())
		}
	}
	src, err := os.ReadFile("../../web/src/playground/examples.ts")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`exampleNames = \[(.*?)\]`).FindSubmatch(src)
	if match == nil {
		t.Fatal("examples.ts has no exampleNames")
	}
	got := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(string(match[1]), -1)
	var names []string
	for _, g := range got {
		names = append(names, g[1])
	}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Errorf("the menu holds %v, want %v", names, want)
	}
}

func fits(t *testing.T, dir string) bool {
	t.Helper()
	var extra bool
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		switch filepath.ToSlash(rel) {
		case ModelFile, StyleFile, LayoutFile:
		case project.ProjectFile:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			extra = extra || !regexp.MustCompile(`^project "[^"]*" \{\s*\}\s*$`).Match(data)
		default:
			extra = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return !extra
}

func FuzzReceive(f *testing.F) {
	for _, s := range []string{
		`{"type":"load","files":{"model.hcl":"node \"a\" {}\n"}}`, `{"type":"load"}`, `{"type":"load","files":{"../x":""}}`,
		`{"type":"write-file","path":"model.hcl","text":"node {"}`, `{"type":"write-file"}`, `{"type":"zip"}`, `{"type":"select-view"}`, `[]`, `{"files":3}`, `{"type":"load","files":{}}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		pg := open(t)
		pg.receive(load(map[string]string{ModelFile: testModel}))
		pg.messages = nil
		pg.p.Receive(context.Background(), data)
		var m struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &m) == nil && len(pg.messages) == 0 && m.Type != "write-file" {
			t.Fatalf("no answer to %s", data)
		}
		for _, reply := range pg.messages {
			var r message
			if err := json.Unmarshal(reply, &r); err != nil {
				t.Fatalf("answer %s: %v", reply, err)
			}
			if r.Type == "files" && len(r.Files) == 0 {
				t.Fatalf("a project without files after %s", data)
			}
		}
	})
}

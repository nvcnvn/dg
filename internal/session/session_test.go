package session

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/fsys"
)

const testModel = `node "a" {
  label = "Alpha"
}

node "b" {
  label = "Beta"
}

edge "ab" {
  from = node.a
  to   = node.b
}
`

// projects are the two places a session's files live, each holding model.hcl.
var projects = []struct {
	name string
	open func(t *testing.T) fsys.FS
}{
	{"disk", func(t *testing.T) fsys.FS {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "model.hcl"), []byte(testModel), 0o644); err != nil {
			t.Fatal(err)
		}
		return fsys.Disk(dir)
	}},
	{"memory", func(t *testing.T) fsys.FS {
		m, err := fsys.NewMemory(map[string][]byte{"model.hcl": []byte(testModel)})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}},
}

// inbox is a client's messages, as they were sent.
type inbox struct{ messages [][]byte }

func (b *inbox) write(_ context.Context, data []byte) {
	b.messages = append(b.messages, bytes.Clone(data))
}

// types returns the types of the messages since the last call, and forgets them.
func (b *inbox) types(t *testing.T) []string {
	t.Helper()
	out := []string{}
	for _, data := range b.messages {
		var m struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m.Type)
	}
	b.messages = nil
	return out
}

// TestMessages sends a client's messages one at a time and checks the types of the answers, and
// that they are byte for byte the same on disk and in memory.
func TestMessages(t *testing.T) {
	steps := []struct {
		name, send string
		want       []string
	}{
		{"select the default view", `{"type":"select-view"}`, []string{"model", "snapshot", "diagnostics", "history"}},
		{"select it again", `{"type":"select-view","view":"main"}`, []string{"diagnostics", "history"}},
		{"an undefined view", `{"type":"select-view","view":"nope","seq":1}`, []string{"error"}},
		{"not a message", `{"type":"nope","seq":2}`, []string{"error"}},
		{"an operation", `{"type":"op","seq":3,"op":{"op":"create_node","kind":"service","label":"Gamma"}}`, []string{"model", "snapshot", "diagnostics", "history", "ack"}},
		{"a refused operation", `{"type":"op","seq":4,"op":{"op":"create_node","kind":"nope"}}`, []string{"history", "error"}},
		{"a move, which only pins", `{"type":"op","seq":5,"op":{"op":"move","view":"main","id":"a","x":40,"y":40}}`, []string{"snapshot", "diagnostics", "history", "ack"}},
		{"undo", `{"type":"undo","seq":6}`, []string{"snapshot", "diagnostics", "history", "ack"}},
		{"undo the node", `{"type":"undo","seq":7}`, []string{"model", "snapshot", "diagnostics", "history", "ack"}},
		{"nothing to undo", `{"type":"undo","seq":8}`, []string{"history", "error"}},
		{"redo", `{"type":"redo","seq":9}`, []string{"model", "snapshot", "diagnostics", "history", "ack"}},
	}
	got := map[string][][]byte{}
	for _, p := range projects {
		t.Run(p.name, func(t *testing.T) {
			s := New(p.open(t))
			var in inbox
			c := s.Connect(in.write)
			ctx := context.Background()
			for _, step := range steps {
				s.Receive(ctx, c, []byte(step.send))
				got[p.name] = append(got[p.name], in.messages...)
				if types := in.types(t); !slices.Equal(types, step.want) {
					t.Errorf("%s: got %v, want %v", step.name, types, step.want)
				}
			}
		})
	}
	if !slices.EqualFunc(got["disk"], got["memory"], bytes.Equal) {
		t.Error("the messages differ between disk and memory")
	}
}

// TestBrokenProject checks that a client keeps the last good drawing and sees the errors, and that
// a project that cannot be loaded at all says so.
func TestBrokenProject(t *testing.T) {
	files, err := fsys.NewMemory(map[string][]byte{"model.hcl": []byte(testModel)})
	if err != nil {
		t.Fatal(err)
	}
	s := New(files)
	var in inbox
	c := s.Connect(in.write)
	ctx := context.Background()
	s.Receive(ctx, c, []byte(`{"type":"select-view"}`))
	in.types(t)
	if err := files.Write(map[string][]byte{"model.hcl": []byte(testModel + "node {")}); err != nil {
		t.Fatal(err)
	}
	s.Reload(ctx)
	if !bytes.Contains(in.messages[len(in.messages)-1], []byte(`"severity":"error"`)) {
		t.Errorf("no error in %s", in.messages)
	}
	if types := in.types(t); !slices.Equal(types, []string{"diagnostics"}) {
		t.Errorf("got %v, want the diagnostics alone: the snapshot is the last good one", types)
	}

	missing := New(fsys.Disk(filepath.Join(t.TempDir(), "gone")))
	c = missing.Connect(in.write)
	missing.Receive(ctx, c, []byte(`{"type":"select-view"}`))
	if !strings.Contains(string(in.messages[0]), "cannot load the project") {
		t.Errorf("got %s", in.messages[0])
	}
}

// TestClients checks that a change reaches every client, and a disconnected one no more.
func TestClients(t *testing.T) {
	files, err := fsys.NewMemory(map[string][]byte{"model.hcl": []byte(testModel)})
	if err != nil {
		t.Fatal(err)
	}
	s := New(files)
	var first, second inbox
	a, b := s.Connect(first.write), s.Connect(second.write)
	ctx := context.Background()
	s.Receive(ctx, a, []byte(`{"type":"select-view"}`))
	s.Receive(ctx, b, []byte(`{"type":"select-view"}`))
	first.types(t)
	second.types(t)
	s.Receive(ctx, a, []byte(`{"type":"op","op":{"op":"set_label","id":"a","label":"Omega"}}`))
	if got, want := first.types(t), []string{"model", "snapshot", "diagnostics", "history", "ack"}; !slices.Equal(got, want) {
		t.Errorf("the sender: got %v, want %v", got, want)
	}
	if got, want := second.types(t), []string{"model", "snapshot", "diagnostics", "history"}; !slices.Equal(got, want) {
		t.Errorf("the other client: got %v, want %v", got, want)
	}
	s.Disconnect(b)
	s.Receive(ctx, a, []byte(`{"type":"undo"}`))
	if got := second.types(t); len(got) != 0 {
		t.Errorf("a disconnected client got %v", got)
	}
}

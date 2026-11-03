package ops

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/project"
)

var update = flag.Bool("update", false, "rewrite the golden files of testdata/ops/want")

const fixture = "../../testdata/ops/in"

// readTree returns every file under dir by slash path.
func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func copyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sameTree(t *testing.T, got, want map[string][]byte, when string) {
	t.Helper()
	for _, name := range slices.Sorted(maps.Keys(want)) {
		if !bytes.Equal(got[name], want[name]) {
			t.Errorf("%s: %s differs:\n%s", when, name, got[name])
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s: %s exists", when, name)
		}
	}
}

func mustDecode(t *testing.T, src string) Op {
	t.Helper()
	op, err := DecodeOps([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return op
}

// TestGolden applies each operation to a copy of testdata/ops/in and compares the files it
// writes with testdata/ops/want/<case> (a removed file is <path>.removed); every other file must
// be unchanged. Undo then restores every file byte for byte, and redo writes the same files
// again.
func TestGolden(t *testing.T) {
	tests := []struct{ name, op string }{
		{"create_node", `{"op": "create_node", "kind": "service", "label": "Billing API"}`},
		{"create_node-in-view", `{"op": "create_node", "kind": "worker", "label": "Worker 3", "parent": "pool", "view": "calls", "x": 24, "y": 36}`},
		{"create_node-in-one-line-view", `{"op": "create_node", "kind": "service", "label": "Solo", "view": "solo"}`},
		{"create_node-quoted-id", `{"op": "create_node", "id": "ns/cache", "kind": "database"}`},
		{"create_node-from-kind", `{"op": "create_node", "kind": "service"}`},
		{"delete_node", `{"op": "delete_node", "id": "worker-1"}`},
		{"delete_node-children", `{"op": "delete_node", "id": "rack"}`},
		{"delete_node-in-list", `{"op": "delete_node", "id": "worker-2"}`},
		{"delete_node-recursive", `{"op": "delete_node", "id": "rack", "recursive": true}`},
		{"set_label", `{"op": "set_label", "id": "gateway", "label": "Front Door\n(public)"}`},
		{"set_parent", `{"op": "set_parent", "id": "gateway", "parent": "pool"}`},
		{"set_parent-remove", `{"op": "set_parent", "id": "worker-2"}`},
		{"set_kind", `{"op": "set_kind", "id": "kube-system/dns", "kind": "database"}`},
		{"set_kind-edge", `{"op": "set_kind", "type": "edge", "id": "w1-w2", "kind": "default"}`},
		{"set_tags", `{"op": "set_tags", "id": "worker-2", "tags": ["critical", "new"]}`},
		{"set_tags-remove", `{"op": "set_tags", "id": "gateway", "tags": []}`},
		{"set_tags-edge", `{"op": "set_tags", "type": "edge", "id": "dns-gw", "tags": ["hot"]}`},
		{"set_props", `{"op": "set_props", "id": "worker-2", "props": {"gpus": 4}}`},
		{"set_props-remove", `{"op": "set_props", "id": "worker-1", "props": {}}`},
		{"add_port", `{"op": "add_port", "id": "worker-2", "name": "nic0", "side": "left"}`},
		{"add_port-quoted", `{"op": "add_port", "id": "gateway", "name": "http/2"}`},
		{"add_port-one-line", `{"op": "add_port", "id": "cache", "name": "sql"}`},
		{"set_label-one-line", `{"op": "set_label", "id": "cache", "label": "Cache"}`},
		{"set_parent-new", `{"op": "set_parent", "id": "cache", "parent": "rack"}`},
		{"remove_port", `{"op": "remove_port", "id": "worker-1", "name": "nic1"}`},
		{"set_rows", `{"op": "set_rows", "id": "orders", "compartments": [{"rows": [{"name": "id", "type": "uuid", "key": "pk"}, {"name": "total"}]}]}`},
		{"set_rows-compartments", `{"op": "set_rows", "id": "orders", "compartments": [{"name": "keys", "rows": [{"name": "id", "key": "pk"}]}, {"name": "methods", "rows": [{"name": "save()"}]}]}`},
		{"connect", `{"op": "connect", "from": {"node": "gateway"}, "to": {"node": "orders", "row": "id"}, "kind": "async", "labels": {"start": "1", "end": "0..*"}, "tags": ["hot"]}`},
		{"connect-in-view", `{"op": "connect", "id": "gw-pool", "from": {"node": "gateway"}, "to": {"node": "pool"}, "view": "calls"}`},
		{"connect-port", `{"op": "connect", "from": {"node": "worker-2"}, "to": {"node": "worker-1", "port": "nic1"}, "labels": {"mid": "sync"}}`},
		{"disconnect", `{"op": "disconnect", "id": "gw-w1"}`},
		{"set_edge_labels", `{"op": "set_edge_labels", "id": "w1-w2", "labels": {"mid": "jobs", "end": "*"}}`},
		{"set_edge_labels-remove", `{"op": "set_edge_labels", "id": "gw-w1"}`},
		{"move", `{"op": "move", "view": "main", "id": "gateway", "x": 105, "y": -3}`},
		{"move-new", `{"op": "move", "view": "main", "id": "worker-2", "x": 40, "y": 120}`},
		{"move-new-layout", `{"op": "move", "view": "pool", "id": "worker-2", "x": 0, "y": 0}`},
		{"resize", `{"op": "resize", "view": "main", "id": "pool", "w": 455, "h": 300}`},
		{"unpin", `{"op": "unpin", "view": "main", "id": "worker-1"}`},
		{"unpin-last", `{"op": "unpin", "view": "calls", "id": "gateway"}`},
		{"pin_port", `{"op": "pin_port", "view": "main", "id": "worker-1", "port": "nic0", "side": "bottom", "offset": 0.333}`},
		{"set_edge_hint", `{"op": "set_edge_hint", "view": "main", "id": "w1-w2", "hint": {"to_side": "top", "via": [[101, 49]], "label_pos": 0.25}}`},
		{"set_edge_hint-remove", `{"op": "set_edge_hint", "view": "main", "id": "gw-w1", "hint": {}}`},
		{"set_edge_hint-lift", `{"op": "set_edge_hint", "view": "main", "id": "lift:gateway:pool", "hint": {"to_side": "left"}}`},
		{"rename_id-node", `{"op": "rename_id", "type": "node", "old": "worker-1", "new": "worker-a"}`},
		{"rename_id-quoted", `{"op": "rename_id", "type": "node", "old": "pool", "new": "gpu/pool"}`},
		{"rename_id-unquote", `{"op": "rename_id", "type": "node", "old": "kube-system/dns", "new": "dns"}`},
		{"rename_id-edge", `{"op": "rename_id", "type": "edge", "old": "gw-w1", "new": "gateway-rpc"}`},
		{"rename_id-view", `{"op": "rename_id", "type": "view", "old": "calls", "new": "rpc"}`},
		{"create_view", `{"op": "create_view", "id": "storage", "title": "Storage", "include": ["kind.storage", "node[\"kube-system/dns\"]"], "edges": []}`},
		{"batch", `[{"op": "create_node", "kind": "service", "view": "calls"}, {"op": "set_label", "id": "service", "label": "Search"}, {"op": "connect", "from": {"node": "search"}, "to": {"node": "gateway"}, "view": "calls"}]`},
	}
	original := readTree(t, fixture)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := copyFixture(t)
			var h History
			c, err := Apply(dir, mustDecode(t, tt.op), NewSession())
			if err != nil {
				t.Fatal(err)
			}
			if want := tt.name == "set_parent"; want != (len(c.Warnings) > 0) || want && !strings.Contains(c.Warnings[0].String(), "Stale frame") {
				t.Errorf("warnings: %v", c.Warnings)
			}
			h.Record(c)
			after := readTree(t, dir)
			wantDir := filepath.Join("../../testdata/ops/want", tt.name)
			if *update {
				if err := os.RemoveAll(wantDir); err != nil {
					t.Fatal(err)
				}
			}
			want := map[string][]byte{}
			for _, fc := range c.Files {
				name, data := fc.Path, fc.After
				if data == nil {
					name, data = name+".removed", []byte{}
				}
				if *update {
					path := filepath.Join(wantDir, filepath.FromSlash(name))
					if err := errors.Join(os.MkdirAll(filepath.Dir(path), 0o755), os.WriteFile(path, data, 0o644)); err != nil {
						t.Fatal(err)
					}
				}
			}
			for name, data := range readTree(t, wantDir) {
				want[name] = data
			}
			expect := maps.Clone(original)
			for name, data := range want {
				if removed, ok := strings.CutSuffix(name, ".removed"); ok {
					delete(expect, removed)
					continue
				}
				expect[name] = data
			}
			sameTree(t, after, expect, "after the operation")
			if _, err := h.Undo(dir, nil); err != nil {
				t.Fatal(err)
			}
			sameTree(t, readTree(t, dir), original, "after undo")
			if _, err := h.Redo(dir, nil); err != nil {
				t.Fatal(err)
			}
			sameTree(t, readTree(t, dir), after, "after redo")
			if _, diags, err := project.Load(dir); err != nil || diags.HasErrors() {
				t.Errorf("the project has errors after the operation: %v %v", err, diags)
			}
		})
	}
}

// TestRefused checks that refused operations name their reason and leave every file untouched.
func TestRefused(t *testing.T) {
	tests := []struct{ name, op, err string }{
		{"non-container with children", `{"op": "set_kind", "id": "pool", "kind": "service"}`, `Parent is not a container`},
		{"port in use", `{"op": "remove_port", "id": "worker-1", "name": "nic0"}`, `Undefined port`},
		{"kind port", `{"op": "remove_port", "id": "orders", "name": "id"}`, `node "orders" has no port "id"`},
		{"view of the node", `{"op": "delete_node", "id": "pool", "recursive": true}`, `view "pool" has of = node.pool`},
		{"edge in a read-only file", `{"op": "delete_node", "id": "gateway"}`, `generated.hcl is read-only`},
		{"rename referenced from a read-only file", `{"op": "rename_id", "type": "node", "old": "gateway", "new": "gw"}`, `generated.hcl is read-only`},
		{"read-only node", `{"op": "set_label", "id": "ext", "label": "External"}`, `generated.hcl is read-only`},
		{"undefined kind", `{"op": "create_node", "kind": "nope"}`, `Undefined kind`},
		{"undefined parent", `{"op": "set_parent", "id": "gateway", "parent": "nope"}`, `Undefined node`},
		{"cycle", `{"op": "set_parent", "id": "pool", "parent": "pool"}`, `Containment cycle`},
		{"wrong property type", `{"op": "set_props", "id": "worker-1", "props": {"gpus": "8"}}`, `Wrong property type`},
		{"undefined property", `{"op": "set_props", "id": "worker-1", "props": {"cores": 8}}`, `Undefined property`},
		{"invalid tag", `{"op": "set_tags", "id": "gateway", "tags": ["no tag"]}`, `Invalid ID`},
		{"duplicate port", `{"op": "add_port", "id": "worker-1", "name": "nic0"}`, `Duplicate port`},
		{"invalid side", `{"op": "add_port", "id": "worker-2", "name": "p", "side": "up"}`, `"up"`},
		{"row in use", `{"op": "set_rows", "id": "gateway", "compartments": [{"rows": [{"name": "x"}]}]}`, `Rows not allowed`},
		{"missing node", `{"op": "connect", "from": {"node": "nope"}, "to": {"node": "gateway"}}`, `node "nope" does not exist`},
		{"missing port", `{"op": "connect", "from": {"node": "gateway", "port": "nope"}, "to": {"node": "pool"}}`, `Undefined port`},
		{"duplicate node", `{"op": "create_node", "id": "pool", "kind": "group"}`, `node "pool" already exists`},
		{"reserved ID", `{"op": "create_node", "id": "all", "kind": "group"}`, `is reserved`},
		{"rename to an existing ID", `{"op": "rename_id", "type": "node", "old": "pool", "new": "rack"}`, `node "rack" already exists`},
		{"rename to an invalid view ID", `{"op": "rename_id", "type": "view", "old": "calls", "new": "a/b"}`, `not a simple ID`},
		{"not shown", `{"op": "move", "view": "calls", "id": "worker-2", "x": 0, "y": 0}`, `view "calls" does not show node "worker-2"`},
		{"not pinned", `{"op": "resize", "view": "main", "id": "worker-2", "w": 10, "h": 10}`, `not pinned`},
		{"not a port", `{"op": "pin_port", "view": "main", "id": "worker-1", "port": "nope", "side": "top"}`, `no port "nope"`},
		{"edge not shown", `{"op": "set_edge_hint", "view": "calls", "id": "w1-w2", "hint": {"to_side": "top"}}`, `does not show edge`},
		{"end not shown", `{"op": "connect", "from": {"node": "orders"}, "to": {"node": "gateway"}, "view": "calls"}`, `does not show node "orders"`},
		{"far away", `{"op": "move", "view": "main", "id": "gateway", "x": 2000000000, "y": 0}`, `within`},
		{"existing view", `{"op": "create_view", "id": "pool"}`, `view "pool" already exists`},
		{"bad selector", `{"op": "create_view", "id": "v", "include": ["edge.gw-w1"]}`, `not a selector`},
		{"batch", `[{"op": "set_label", "id": "gateway", "label": "G"}, {"op": "set_label", "id": "nope", "label": "N"}]`, `operation 2 (set_label): node "nope" does not exist`},
		{"batch with an error", `[{"op": "set_label", "id": "gateway", "label": "G"}, {"op": "set_kind", "id": "pool", "kind": "service"}]`, `Parent is not a container`},
	}
	original := readTree(t, fixture)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := copyFixture(t)
			_, err := Apply(dir, mustDecode(t, tt.op), NewSession())
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Fatalf("error %v, want one containing %q", err, tt.err)
			}
			sameTree(t, readTree(t, dir), original, "after the refusal")
		})
	}
}

// TestChangedBetweenReadAndWrite changes a file after the operation read the project: the
// operation is applied again to the new content, or fails when it no longer applies.
func TestChangedBetweenReadAndWrite(t *testing.T) {
	const added = "\nnode \"added\" {\n  kind = \"service\"\n}\n"
	tests := []struct {
		name   string
		op     string
		change func(dir string) error
		err    string
		want   []string
	}{
		{
			name: "re-applied",
			op:   `{"op": "set_label", "id": "gateway", "label": "Front"}`,
			change: func(dir string) error {
				f, err := os.OpenFile(filepath.Join(dir, "model.hcl"), os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					return err
				}
				_, err = f.WriteString(added)
				return errors.Join(err, f.Close())
			},
			want: []string{`label = "Front"`, `node "added"`},
		},
		{
			name: "no longer applies",
			op:   `{"op": "set_label", "id": "orders", "label": "Orders"}`,
			change: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "model.hcl"), []byte("node \"gateway\" {\n  kind = \"service\"\n}\n"), 0o644)
			},
			err: `node "orders" does not exist`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := copyFixture(t)
			calls := 0
			beforeWrite = func() {
				calls++
				if calls == 1 {
					if err := tt.change(dir); err != nil {
						t.Fatal(err)
					}
				}
			}
			defer func() { beforeWrite = nil }()
			_, err := Apply(dir, mustDecode(t, tt.op), NewSession())
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("error %v, want one containing %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Errorf("read the project %d times, want 2", calls)
			}
			got, _ := os.ReadFile(filepath.Join(dir, "model.hcl"))
			for _, w := range tt.want {
				if !strings.Contains(string(got), w) {
					t.Errorf("model.hcl lacks %q:\n%s", w, got)
				}
			}
		})
	}
}

// TestUndoAfterExternalChange checks that an undo whose files changed since is dropped.
func TestUndoAfterExternalChange(t *testing.T) {
	dir := copyFixture(t)
	var h History
	c, err := Apply(dir, mustDecode(t, `{"op": "set_label", "id": "gateway", "label": "Front"}`), NewSession())
	if err != nil {
		t.Fatal(err)
	}
	h.Record(c)
	if err := os.WriteFile(filepath.Join(dir, "model.hcl"), []byte("# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Undo(dir, nil); err == nil || !strings.Contains(err.Error(), "model.hcl changed since") {
		t.Fatalf("undo: %v", err)
	}
	if h.CanUndo() || h.CanRedo() {
		t.Error("the change was kept")
	}
}

func TestIDs(t *testing.T) {
	taken := func(ids ...string) func(string) bool {
		return func(id string) bool { return slices.Contains(ids, id) }
	}
	tests := []struct {
		kind, label string
		taken       []string
		want        string
		fromKind    bool
	}{
		{"service", "API Gateway", nil, "api-gateway", false},
		{"service", "API Gateway", []string{"api-gateway", "api-gateway-2"}, "api-gateway-3", false},
		{"service", "  --Ünïcode & Co.--  ", nil, "n-code-co", false},
		{"service", "2nd service", nil, "n-2nd-service", false},
		{"service", "", nil, "service", true},
		{"service", "!!!", []string{"service"}, "service-2", true},
		{"group", "All", nil, "all-2", false},
		{"service", strings.Repeat("ab ", 40), nil, strings.TrimSuffix(strings.Repeat("ab-", 22)[:64], "-"), false},
	}
	for _, tt := range tests {
		got, fromKind := nodeID(tt.kind, tt.label, taken(tt.taken...))
		if got != tt.want || fromKind != tt.fromKind {
			t.Errorf("nodeID(%q, %q) = %q, %v; want %q, %v", tt.kind, tt.label, got, fromKind, tt.want, tt.fromKind)
		}
	}
	if got := edgeID("kube-system/dns", "API", taken("kube-system-dns-api")); got != "kube-system-dns-api-2" {
		t.Errorf("edgeID = %q", got)
	}
}

func TestDecodeOp(t *testing.T) {
	tests := []struct{ src, err string }{
		{`{"kind": "x"}`, `needs "op"`},
		{`{"op": "draw"}`, `unknown operation "draw"`},
		{`{"op": "create_node"}`, `missing field "kind"`},
		{`{"op": "create_node", "kind": "x", "w": 3}`, `unknown field "w"`},
		{`{"op": "move", "view": "v", "id": "a", "x": 1.5, "y": 0}`, `move: x must be a whole number, not number 1.5`},
		{`{"op": "batch", "ops": [{"op": "unpin", "view": "v"}]}`, `batch operation 1: unpin: missing field "id"`},
		{`[1]`, `batch operation 1`},
		{`{"op": "batch", "ops": [{"op": "batch", "ops": []}]}`, ``},
	}
	for _, tt := range tests {
		_, err := DecodeOps([]byte(tt.src))
		if tt.err == "" && err != nil || tt.err != "" && (err == nil || !strings.Contains(err.Error(), tt.err)) {
			t.Errorf("DecodeOps(%s): %v, want %q", tt.src, err, tt.err)
		}
	}
}

// TestExistingErrors checks that errors the project already has do not refuse an operation,
// even when it renames what they name, while another error with the same summary does.
func TestExistingErrors(t *testing.T) {
	tests := []struct{ op, err string }{
		{`{"op": "rename_id", "type": "node", "old": "bad", "new": "good"}`, ""},
		{`{"op": "set_label", "id": "bad", "label": "Still bad"}`, ""},
		{`{"op": "set_kind", "id": "ok", "kind": "nope"}`, `Node "ok" has undefined kind "nope"`},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		src := "node \"bad\" {\n  kind = \"nope\"\n}\n\nnode \"ok\" {\n  kind = \"service\"\n}\n"
		if err := os.WriteFile(filepath.Join(dir, "model.hcl"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Apply(dir, mustDecode(t, tt.op), NewSession())
		if tt.err == "" && err != nil || tt.err != "" && (err == nil || !strings.Contains(err.Error(), tt.err)) {
			t.Errorf("%s: %v, want %q", tt.op, err, tt.err)
		}
	}
}

// TestUndoSession checks that undo and redo restore the re-slug-once state with the files.
func TestUndoSession(t *testing.T) {
	dir := copyFixture(t)
	sess := NewSession()
	var h History
	c, err := Apply(dir, mustDecode(t, `{"op": "create_node", "kind": "service"}`), sess)
	if err != nil {
		t.Fatal(err)
	}
	h.Record(c)
	if !sess.kindIDs["service"] {
		t.Fatal("the kind-derived ID is not in the session")
	}
	if _, err := h.Undo(dir, sess); err != nil || sess.kindIDs["service"] {
		t.Fatalf("after undo: %v, session %v", err, sess.kindIDs)
	}
	if _, err := h.Redo(dir, sess); err != nil || !sess.kindIDs["service"] {
		t.Fatalf("after redo: %v, session %v", err, sess.kindIDs)
	}
}

// TestConcurrentWriters applies operations from many goroutines at once: each that succeeds is
// in the files.
func TestConcurrentWriters(t *testing.T) {
	dir := copyFixture(t)
	const n = 16
	errs := make(chan error, n)
	for i := range n {
		go func() {
			_, err := Apply(dir, mustDecode(t, fmt.Sprintf(`{"op": "create_node", "id": "n%d", "kind": "service"}`, i)), NewSession())
			errs <- err
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	p, _, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		if p.Model.Nodes[fmt.Sprintf("n%d", i)] == nil {
			t.Errorf("node n%d is missing", i)
		}
	}
}

// TestCommitOutside checks that a write through a link out of the project is refused before
// anything is written or created.
func TestCommitOutside(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "views")); err != nil {
		t.Fatal(err)
	}
	err := commit(dir, []FileChange{{Path: "model.hcl", After: []byte("x")}, {Path: "views/new/main.layout.yaml", After: []byte("y")}}, func(fc FileChange) []byte { return fc.After })
	if err == nil || !strings.Contains(err.Error(), "links outside the project") {
		t.Fatalf("commit: %v", err)
	}
	for _, d := range []string{dir, outside} {
		entries, _ := os.ReadDir(d)
		if len(entries) > 1 || d == outside && len(entries) > 0 {
			t.Errorf("%s holds %v", d, entries)
		}
	}
}

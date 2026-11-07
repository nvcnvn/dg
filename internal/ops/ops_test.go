package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
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

var update = flag.Bool("update", false, "rewrite the golden files of testdata/ops/want")

const fixture = "../../testdata/ops/in"

// readTree returns every file under dir by slash path.
func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	return readFS(t, os.DirFS(dir))
}

func readFS(t *testing.T, fsys fs.FS) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		out[path], err = fs.ReadFile(fsys, path)
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

// backends are the two places a project's files live; each opens a copy of the fixture and
// returns a function that reads it back.
var backends = []struct {
	name string
	open func(t *testing.T) (fsys.FS, func() map[string][]byte)
}{
	{"disk", func(t *testing.T) (fsys.FS, func() map[string][]byte) {
		dir := copyFixture(t)
		return fsys.Disk(dir), func() map[string][]byte { return readTree(t, dir) }
	}},
	{"memory", func(t *testing.T) (fsys.FS, func() map[string][]byte) {
		m, err := fsys.NewMemory(readTree(t, fixture))
		if err != nil {
			t.Fatal(err)
		}
		return m, func() map[string][]byte { return readFS(t, m) }
	}},
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

// goldenOps are the operations TestGolden applies, each named after its golden directory.
var goldenOps = []struct{ name, op string }{
	{"create_node", `{"op": "create_node", "kind": "service", "label": "Billing API"}`},
	{"create_node-in-view", `{"op": "create_node", "kind": "worker", "label": "Worker 3", "parent": "pool", "view": "calls", "x": 24, "y": 36}`},
	{"create_node-in-one-line-view", `{"op": "create_node", "kind": "service", "label": "Solo", "view": "solo"}`},
	{"create_node-quoted-id", `{"op": "create_node", "id": "ns/cache", "kind": "database"}`},
	{"create_node-from-kind", `{"op": "create_node", "kind": "service"}`},
	{"create_node-from-pack-kind", `{"op": "create_node", "kind": "aws-dynamodb", "view": "calls", "x": 24, "y": 36}`},
	{"create_node-from-pack-kind-taken", `[{"op": "create_node", "kind": "aws-dynamodb"}, {"op": "create_node", "kind": "aws-dynamodb"}]`},
	{"create_node-from-pack-kind-labeled", `[{"op": "create_node", "kind": "aws-dynamodb"}, {"op": "set_label", "id": "dynamodb", "label": "Orders table"}]`},
	{"create_node-from-own-kind-with-a-pack-prefix", `{"op": "create_node", "kind": "aws-own"}`},
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
	{"set_view_label", `{"op": "set_view_label", "view": "calls", "id": "gateway", "label": "Gateway\n2 replicas"}`},
	{"set_view_label-replace", `[{"op": "set_view_label", "view": "calls", "id": "gateway", "label": "One"}, {"op": "set_view_label", "view": "calls", "id": "gateway", "label": "Two"}]`},
	{"set_view_label-one-line", `{"op": "set_view_label", "view": "main", "id": "kube-system/dns", "label": "DNS"}`},
	{"rename_id-view-label", `[{"op": "set_view_label", "view": "main", "id": "cache", "label": "One"}, {"op": "rename_id", "type": "node", "old": "cache", "new": "store"}]`},
	{"delete_node-view-label", `[{"op": "set_view_label", "view": "main", "id": "cache", "label": "One"}, {"op": "delete_node", "id": "cache"}]`},
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
	{"reorder", `[{"op": "unpin", "view": "main", "id": "worker-1"}, {"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "reorder", "view": "pool", "container": "pool", "order": ["worker-2", "worker-1"]}]`},
	{"reorder-in-view", `[{"op": "unpin", "view": "main", "id": "worker-1"}, {"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "reorder", "view": "main", "container": "pool", "order": ["worker-2"]}, {"op": "resize", "view": "main", "id": "worker-2", "w": 205, "h": 60}, {"op": "unpin", "view": "main", "id": "pool"}]`},
	{"reorder-remove", `[{"op": "unpin", "view": "main", "id": "worker-1"}, {"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "reorder", "view": "pool", "container": "pool", "order": ["worker-2"]}, {"op": "reorder", "view": "pool", "container": "pool"}]`},
	{"reorder-clear-stale", `[{"op": "unpin", "view": "main", "id": "worker-1"}, {"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "reorder", "view": "main", "container": "pool", "order": ["worker-2"]}, {"op": "set_kind", "id": "pool", "kind": "group"}, {"op": "reorder", "view": "main", "container": "pool"}]`},
	{"delete_node-last-ordered", `[{"op": "unpin", "view": "main", "id": "worker-1"}, {"op": "unpin", "view": "main", "id": "pool"}, {"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "reorder", "view": "main", "container": "pool", "order": ["worker-2"]}, {"op": "delete_node", "id": "worker-2"}]`},
	{"unpin", `{"op": "unpin", "view": "main", "id": "worker-1"}`},
	{"unpin-last", `{"op": "unpin", "view": "calls", "id": "gateway"}`},
	{"pin_port", `{"op": "pin_port", "view": "main", "id": "worker-1", "port": "nic0", "side": "bottom", "offset": 0.333}`},
	{"set_edge_hint", `{"op": "set_edge_hint", "view": "main", "id": "w1-w2", "hint": {"to_side": "top", "via": [[101, 49]], "label_pos": 0.25}}`},
	{"set_edge_hint-remove", `{"op": "set_edge_hint", "view": "main", "id": "gw-w1", "hint": {}}`},
	{"set_edge_hint-lift", `[{"op": "update_view", "id": "main", "collapse": ["node.pool"]}, {"op": "set_edge_hint", "view": "main", "id": "lift:gateway:pool", "hint": {"to_side": "left"}}]`},
	{"rename_id-node", `{"op": "rename_id", "type": "node", "old": "worker-1", "new": "worker-a"}`},
	{"rename_id-quoted", `{"op": "rename_id", "type": "node", "old": "pool", "new": "gpu/pool"}`},
	{"rename_id-unquote", `{"op": "rename_id", "type": "node", "old": "kube-system/dns", "new": "dns"}`},
	{"rename_id-special", `[{"op": "rename_id", "type": "node", "old": "kube-system/dns", "new": "a%b:c#d$e\\f"}, {"op": "move", "view": "main", "id": "a%b:c#d$e\\f", "x": 0, "y": 500}]`},
	{"rename_id-from-special", `[{"op": "rename_id", "type": "node", "old": "kube-system/dns", "new": "a%b:c#d$e\\f"}, {"op": "move", "view": "main", "id": "a%b:c#d$e\\f", "x": 0, "y": 500}, {"op": "rename_id", "type": "node", "old": "a%b:c#d$e\\f", "new": "dns"}]`},
	{"rename_id-edge", `{"op": "rename_id", "type": "edge", "old": "gw-w1", "new": "gateway-rpc"}`},
	{"rename_id-view", `{"op": "rename_id", "type": "view", "old": "calls", "new": "rpc"}`},
	{"create_view", `{"op": "create_view", "id": "storage", "title": "Storage", "include": ["kind.storage", "node[\"kube-system/dns\"]"], "edges": []}`},
	{"create_view-overlays", `{"op": "create_view", "id": "incident", "direction": "right", "collapse": ["node.rack"], "overlays": [{"id": "blast", "members": ["tag.edge", "node[\"worker-1\"]"], "style": "hull", "label": "Blast radius"}], "styles": [{"label": "tag.edge", "props": {"fill": "#FCE8E6"}}]}`},
	{"update_view", `{"op": "update_view", "id": "calls", "title": "Calls", "collapse": ["node.pool"], "direction": "right", "overlays": [{"id": "hot", "members": ["node.gateway"], "label": "Hot"}], "unset": ["edges"]}`},
	{"update_view-styles", `{"op": "update_view", "id": "calls", "styles": [{"label": "node.gateway", "props": {"fill": "#FFEEEE"}}, {"label": "tag.edge", "props": {"stroke_dash": [4, 2], "stroke_width": 2}}, {"type": "edge_style", "label": "edge.gw-w1", "props": {"stroke": "#D93025", "animate": "flow"}}]}`},
	{"update_view-one-line", `{"op": "update_view", "id": "main", "collapse": ["node.rack"]}`},
	{"update_view-unset", `{"op": "update_view", "id": "pool", "unset": ["of", "title"]}`},
	{"delete_view", `{"op": "delete_view", "id": "calls"}`},
	{"add_decoration", `{"op": "add_decoration", "view": "solo", "decoration": {"type": "band", "x": -21, "y": -19, "w": 200, "h": 100, "z": -2, "fill": "#FCE8E6", "stroke_dash": [4, 2]}}`},
	{"add_decoration-new-layout", `{"op": "add_decoration", "view": "pool", "id": "hello", "decoration": {"type": "text", "text": "Hi", "attach": "worker-1", "dx": 0, "dy": -40}}`},
	{"update_decoration", `{"op": "update_decoration", "view": "solo", "id": "note", "decoration": {"x": 10, "y": 20, "font_size": 12}, "unset": ["attach", "dx", "dy"]}`},
	{"delete_decoration", `{"op": "delete_decoration", "view": "solo", "id": "flow"}`},
	{"promote_decoration", `{"op": "promote_decoration", "view": "solo", "id": "title", "kind": "note"}`},
	{"promote_decoration-in-frame", `[{"op": "update_view", "id": "solo", "include": ["node.gateway", "node.pool"]}, {"op": "promote_decoration", "view": "solo", "id": "shelf", "kind": "group", "new": "shelf", "label": "Shelf"}]`},
	{"create_node-before", `[{"op": "unpin", "view": "main", "id": "worker-1"}, {"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "create_node", "kind": "worker", "parent": "pool", "view": "main", "before": "worker-2"}]`},
	{"create_node-last", `[{"op": "unpin", "view": "main", "id": "worker-1"}, {"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "create_node", "id": "a-worker", "kind": "worker", "parent": "pool", "view": "main"}]`},
	{"show_node", `{"op": "show_node", "view": "calls", "id": "worker-2"}`},
	{"show_node-excluded", `[{"op": "update_view", "id": "main", "exclude": ["node.gateway"]}, {"op": "show_node", "view": "main", "id": "gateway"}]`},
	{"hide_node", `{"op": "hide_node", "view": "main", "id": "cache"}`},
	{"hide_node-included", `{"op": "hide_node", "view": "calls", "id": "worker-1"}`},
	{"hide_node-also-selected", `{"op": "hide_node", "view": "pool", "id": "worker-2"}`},
	{"hide_node-first", `{"op": "hide_node", "view": "listed", "id": "gateway"}`},
	{"hide_node-last-commented", `{"op": "hide_node", "view": "pool", "id": "gateway"}`},
	{"hide_node-every", `[{"op": "hide_node", "view": "listed", "id": "gateway"}, {"op": "hide_node", "view": "listed", "id": "cache"}, {"op": "hide_node", "view": "listed", "id": "pool"}]`},
	{"hide_node-container", `{"op": "hide_node", "view": "main", "id": "pool"}`},
	{"show_node-after-comment", `[{"op": "hide_node", "view": "listed", "id": "pool"}, {"op": "show_node", "view": "listed", "id": "worker-2"}]`},
	{"show_node-container", `[{"op": "hide_node", "view": "main", "id": "pool"}, {"op": "show_node", "view": "main", "id": "pool"}]`},
	{"set_parent-before-ports", `[{"op": "add_port", "id": "cache", "name": "sql"}, {"op": "set_parent", "id": "cache", "parent": "rack"}]`},
	{"batch", `[{"op": "create_node", "kind": "service", "view": "calls"}, {"op": "set_label", "id": "service", "label": "Search"}, {"op": "connect", "from": {"node": "search"}, "to": {"node": "gateway"}, "view": "calls"}]`},
}

// TestGolden applies each operation to a copy of testdata/ops/in, on disk and in memory, and
// compares the files it writes with testdata/ops/want/<case> (a removed file is <path>.removed);
// every other file must be unchanged. Undo then restores every file byte for byte, and redo
// writes the same files again.
func TestGolden(t *testing.T) {
	tests := goldenOps
	original := readTree(t, fixture)
	for _, tt := range tests {
		for _, b := range backends {
			t.Run(tt.name+"/"+b.name, func(t *testing.T) {
				files, read := b.open(t)
				var h History
				c, err := Apply(files, mustDecode(t, tt.op), NewSession())
				if err != nil {
					t.Fatal(err)
				}
				if want := tt.name == "set_parent"; want != (len(c.Warnings) > 0) || want && !strings.Contains(c.Warnings[0].String(), "Stale frame") {
					t.Errorf("warnings: %v", c.Warnings)
				}
				h.Record(c)
				after := read()
				wantDir := filepath.Join("../../testdata/ops/want", tt.name)
				rewrite := *update && b.name == "disk"
				if rewrite {
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
					if rewrite {
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
				if _, err := h.Undo(files, nil); err != nil {
					t.Fatal(err)
				}
				sameTree(t, read(), original, "after undo")
				if _, err := h.Redo(files, nil); err != nil {
					t.Fatal(err)
				}
				sameTree(t, read(), after, "after redo")
				if _, diags, err := project.LoadFS(files); err != nil || diags.HasErrors() {
					t.Errorf("the project has errors after the operation: %v %v", err, diags)
				}
			})
		}
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
		{"view label of an undefined node", `{"op": "set_view_label", "view": "main", "id": "nope", "label": "N"}`, `node "nope" does not exist`},
		{"label in an undefined view", `{"op": "set_view_label", "view": "nope", "id": "cache", "label": "N"}`, `view "nope" does not exist`},
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
		{"lift: edge ID", `{"op": "connect", "id": "lift:a:b", "from": {"node": "gateway"}, "to": {"node": "pool"}}`, `starts with lift:`},
		{"rename edge to lift:", `{"op": "rename_id", "type": "edge", "old": "gw-w1", "new": "lift:x"}`, `starts with lift:`},
		{"reserved ID", `{"op": "create_node", "id": "all", "kind": "group"}`, `is reserved`},
		{"rename to an existing ID", `{"op": "rename_id", "type": "node", "old": "pool", "new": "rack"}`, `node "rack" already exists`},
		{"rename to an invalid view ID", `{"op": "rename_id", "type": "view", "old": "calls", "new": "a/b"}`, `not a simple ID`},
		{"not shown", `{"op": "move", "view": "calls", "id": "worker-2", "x": 0, "y": 0}`, `view "calls" does not show node "worker-2"`},
		{"not pinned", `{"op": "resize", "view": "main", "id": "worker-2", "w": 10, "h": 10}`, `not pinned`},
		{"zero size", `{"op": "resize", "view": "main", "id": "pool", "w": 0, "h": 0}`, `w and h are positive`},
		{"negative size", `{"op": "resize", "view": "main", "id": "pool", "w": 100, "h": -10}`, `w and h are positive`},
		{"zero decoration", `{"op": "add_decoration", "view": "main", "decoration": {"type": "band", "x": 0, "y": 0, "w": 0, "h": 10}}`, `w and h are positive`},
		{"reorder a free container", `{"op": "reorder", "view": "main", "container": "pool", "order": ["worker-1"]}`, `"pool" is not a stack or grid container in view "main"`},
		{"reorder a node not in the container", `[{"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "reorder", "view": "main", "container": "pool", "order": ["gateway"]}]`, `node "gateway" is not a child of "pool" drawn in view "main"`},
		{"reorder twice", `[{"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "reorder", "view": "pool", "container": "pool", "order": ["worker-1", "worker-1"]}]`, `in the order twice`},
		{"reorder a hidden container", `{"op": "reorder", "view": "solo", "container": "pool", "order": ["worker-1"]}`, `view "solo" does not show node "pool"`},
		{"move an ordered node", `[{"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "move", "view": "main", "id": "worker-2", "x": 0, "y": 0}]`, `node "worker-2" is placed in order by "pool", a stack container; reorder it instead`},
		{"not a port", `{"op": "pin_port", "view": "main", "id": "worker-1", "port": "nope", "side": "top"}`, `no port "nope"`},
		{"lifted edge not drawn", `{"op": "set_edge_hint", "view": "main", "id": "lift:gateway:pool", "hint": {"to_side": "left"}}`, `draws no lifted edge from "gateway" to "pool"`},
		{"edge not shown", `{"op": "set_edge_hint", "view": "calls", "id": "w1-w2", "hint": {"to_side": "top"}}`, `does not show edge`},
		{"end not shown", `{"op": "connect", "from": {"node": "orders"}, "to": {"node": "gateway"}, "view": "calls"}`, `does not show node "orders"`},
		{"far away", `{"op": "move", "view": "main", "id": "gateway", "x": 2000000000, "y": 0}`, `within`},
		{"existing view", `{"op": "create_view", "id": "pool"}`, `view "pool" already exists`},
		{"bad selector", `{"op": "create_view", "id": "v", "include": ["edge.gw-w1"]}`, `not a selector`},
		{"unset unknown", `{"op": "update_view", "id": "main", "unset": ["color"]}`, `unset names attributes of a view`},
		{"set and unset", `{"op": "update_view", "id": "main", "title": "T", "unset": ["title"]}`, `title is both set and unset`},
		{"update undefined view", `{"op": "update_view", "id": "nope", "title": "T"}`, `view "nope" does not exist`},
		{"before a node not in the order", `[{"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "create_node", "kind": "worker", "parent": "pool", "view": "main", "before": "gateway"}]`, `node "gateway" is not a sibling`},
		{"before in a free container", `{"op": "create_node", "kind": "worker", "parent": "pool", "view": "main", "before": "worker-1"}`, `before does not apply`},
		{"show a node in a collapsed container", `[{"op": "update_view", "id": "main", "collapse": ["node.pool"]}, {"op": "show_node", "view": "main", "id": "worker-1"}]`, `inside a collapsed container`},
		{"show an excluded kind", `[{"op": "update_view", "id": "main", "exclude": ["kind.worker"]}, {"op": "show_node", "view": "main", "id": "worker-1"}]`, `it is excluded`},
		{"hide a node not shown", `{"op": "hide_node", "view": "calls", "id": "cache"}`, `view "calls" does not show node "cache"`},
		{"bad direction", `{"op": "update_view", "id": "main", "direction": "sideways"}`, `direction is one of`},
		{"bad style value", `{"op": "update_view", "id": "main", "styles": [{"label": "node.gateway", "props": {"fill": "red"}}]}`, `Invalid style value`},
		{"unknown style property", `{"op": "update_view", "id": "main", "styles": [{"label": "node.gateway", "props": {"colour": "#FFF"}}]}`, `Unsupported argument`},
		{"bad overlay member", `{"op": "update_view", "id": "main", "overlays": [{"id": "o", "members": ["edge.gw-w1"]}]}`, `not a selector`},
		{"collapse a leaf", `{"op": "update_view", "id": "main", "collapse": ["node.gateway"]}`, `Not a container`},
		{"delete undefined view", `{"op": "delete_view", "id": "nope"}`, `view "nope" does not exist`},
		{"decoration without size", `{"op": "add_decoration", "view": "solo", "decoration": {"type": "rect"}}`, `needs w`},
		{"duplicate decoration", `{"op": "add_decoration", "view": "solo", "id": "title", "decoration": {"type": "text", "text": "T"}}`, `already has a decoration "title"`},
		{"decoration bad color", `{"op": "update_decoration", "view": "solo", "id": "title", "decoration": {"fill": "red"}}`, `Invalid layout`},
		{"attach to nothing", `{"op": "add_decoration", "view": "solo", "decoration": {"type": "text", "text": "T", "attach": "nope"}}`, `node "nope" does not exist`},
		{"undefined decoration", `{"op": "delete_decoration", "view": "solo", "id": "nope"}`, `no decoration "nope"`},
		{"promote attached", `{"op": "promote_decoration", "view": "solo", "id": "note", "kind": "note"}`, `attached to node "gateway"`},
		{"promote from a hidden frame", `{"op": "promote_decoration", "view": "solo", "id": "shelf", "kind": "group"}`, `which view "solo" does not draw expanded`},
		{"promote from a collapsed frame", `[{"op": "update_view", "id": "solo", "include": ["node.gateway", "node.pool"], "collapse": ["node.pool"]}, {"op": "promote_decoration", "view": "solo", "id": "shelf", "kind": "group"}]`, `does not draw expanded`},
		{"promote from a leaf", `[{"op": "update_decoration", "view": "solo", "id": "shelf", "decoration": {"in": "gateway"}}, {"op": "promote_decoration", "view": "solo", "id": "shelf", "kind": "group"}]`, `"gateway", which is not a container`},
		{"promote arrow", `{"op": "promote_decoration", "view": "solo", "id": "flow", "kind": "note"}`, `has no box`},
		{"batch", `[{"op": "set_label", "id": "gateway", "label": "G"}, {"op": "set_label", "id": "nope", "label": "N"}]`, `operation 2 (set_label): node "nope" does not exist`},
		{"batch with an error", `[{"op": "set_label", "id": "gateway", "label": "G"}, {"op": "set_kind", "id": "pool", "kind": "service"}]`, `Parent is not a container`},
	}
	original := readTree(t, fixture)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := copyFixture(t)
			_, err := Apply(fsys.Disk(dir), mustDecode(t, tt.op), NewSession())
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
			_, err := Apply(fsys.Disk(dir), mustDecode(t, tt.op), NewSession())
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

// TestUndoAfterExternalChange checks that undo and redo keep changes made outside the editor
// elsewhere in the files they restore, by a three-way merge, and that an undo whose change was
// edited since is dropped.
func TestUndoAfterExternalChange(t *testing.T) {
	for _, tc := range []struct {
		name     string
		edit     func(string) string
		conflict string
	}{
		{"elsewhere", func(s string) string { return "# edited\n" + s }, ""},
		{"replaced", func(string) string { return "# edited\n" }, "model.hcl was changed since in the same place"},
		{"same line", func(s string) string { return strings.Replace(s, `"Front"`, `"Back"`, 1) }, "model.hcl was changed since in the same place"},
		{"removed", nil, "model.hcl was removed since"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := copyFixture(t)
			path := filepath.Join(dir, "model.hcl")
			orig, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var h History
			c, err := Apply(fsys.Disk(dir), mustDecode(t, `{"op": "set_label", "id": "gateway", "label": "Front"}`), NewSession())
			if err != nil {
				t.Fatal(err)
			}
			h.Record(c)
			after, _ := os.ReadFile(path)
			if tc.edit == nil {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, []byte(tc.edit(string(after))), 0o644)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = h.Undo(fsys.Disk(dir), nil)
			if tc.conflict != "" {
				if err == nil || !strings.Contains(err.Error(), tc.conflict) {
					t.Fatalf("undo: %v", err)
				}
				if h.CanUndo() || h.CanRedo() {
					t.Error("the change was kept")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(path); string(got) != tc.edit(string(orig)) {
				t.Errorf("after undo:\n%s\nwant:\n%s", got, tc.edit(string(orig)))
			}
			if _, err := h.Redo(fsys.Disk(dir), nil); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(path); string(got) != tc.edit(string(after)) {
				t.Errorf("after redo:\n%s\nwant:\n%s", got, tc.edit(string(after)))
			}
		})
	}
}

// TestUndoMergeInvalid checks that an undo whose merge is clean line by line but would make the
// project invalid is dropped and writes nothing: the operation removed the edge_hints section,
// and an outside edit added another one.
func TestUndoMergeInvalid(t *testing.T) {
	dir := copyFixture(t)
	path := filepath.Join(dir, "views", "main.layout.yaml")
	var h History
	c, err := Apply(fsys.Disk(dir), mustDecode(t, `{"op": "set_edge_hint", "view": "main", "id": "gw-w1", "hint": {}}`), NewSession())
	if err != nil {
		t.Fatal(err)
	}
	h.Record(c)
	after, _ := os.ReadFile(path)
	edited := strings.Replace(string(after), "version: 1\n", "version: 1\nedge_hints:\n  w1-w2: {to_side: top}\n", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Undo(fsys.Disk(dir), nil); err == nil || !strings.Contains(err.Error(), "would add errors") {
		t.Fatalf("undo: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != edited {
		t.Errorf("the refused undo wrote:\n%s", got)
	}
	if h.CanUndo() {
		t.Error("the change was kept")
	}
}

// TestUndoCreatedFile checks that undoing an operation that created a file, changed since, is
// refused with a message saying so.
func TestUndoCreatedFile(t *testing.T) {
	dir := copyFixture(t)
	var h History
	c, err := Apply(fsys.Disk(dir), mustDecode(t, `{"op": "move", "view": "pool", "id": "gateway", "x": 0, "y": 0}`), NewSession())
	if err != nil {
		t.Fatal(err)
	}
	h.Record(c)
	path := filepath.Join(dir, "views", "pool.layout.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the move created no layout file: %v", err)
	}
	if err := os.WriteFile(path, append([]byte("# kept\n"), data...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Undo(fsys.Disk(dir), nil); err == nil || !strings.Contains(err.Error(), "views/pool.layout.yaml, which the change created, was changed since") {
		t.Fatalf("undo: %v", err)
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

// TestSlug checks slug against testdata/ids/slug.json, which the web app's tests check its slug
// against too, so both make the same IDs.
func TestSlug(t *testing.T) {
	data, err := os.ReadFile("../../testdata/ids/slug.json")
	if err != nil {
		t.Fatal(err)
	}
	var table []struct{ In, Slug string }
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatal(err)
	}
	for _, tt := range table {
		if got := slug(tt.In); got != tt.Slug {
			t.Errorf("slug(%q) = %q, want %q", tt.In, got, tt.Slug)
		}
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
		{`{"op": "connect", "from": {"node": "a"}, "to": {"node": "b"}, "label": "x"}`, `connect: unknown field "label"; it takes from: {node, port?, row?}, to: {node, port?, row?}, id?, kind?, labels?: {start?, mid?, end?}, tags?, view?`},
		{`{"op": "connect", "from": {"node": "a"}, "to": {"node": "b"}, "labels": {"middle": "x"}}`, `connect: labels: unknown field "middle"; it takes {start?, mid?, end?}`},
		{`{"op": "connect", "from": "a", "to": {"node": "b"}}`, `connect: from must be an object, not string; it takes {node, port?, row?}`},
		{`{"op": "connect", "from": {"id": "a"}, "to": {"node": "b"}}`, `connect: from: unknown field "id"; it takes {node, port?, row?}`},
		{`{"op": "set_edge_labels", "id": "e", "labels": {"mid": 1}}`, `set_edge_labels: labels.mid must be a string, not number; it takes {start?, mid?, end?}`},
		{`{"op": "set_edge_hint", "view": "v", "id": "e", "hint": {"side": "top"}}`, `set_edge_hint: hint: unknown field "side"; it takes {from_side?, to_side?, via?: [[x, y]], label_pos?}`},
		{`{"op": "set_rows", "id": "a", "compartments": [{"rows": [{"column": "id"}]}]}`, `set_rows: compartments: unknown field "column"; it takes [{name?, rows: [{name, type?, key?}]}]`},
		{`{"op": "update_view", "id": "v", "overlays": [{"id": "o", "members": [], "color": "red"}]}`, `update_view: overlays: unknown field "color"; it takes [{id, members, style?, label?}]`},
		{`{"op": "update_view", "id": "v", "styles": [{"label": "a", "props": {}, "kind": "x"}]}`, `update_view: styles: unknown field "kind"; it takes [{type?, label, props}]`},
		{`{"op": "add_decoration", "view": "v", "decoration": {"type": "text", "colour": "red"}}`, `add_decoration: decoration: unknown field "colour"; it takes {type, in?`},
		{`{"op": "update_decoration", "view": "v", "id": "d", "decoration": {"colour": "red"}}`, `update_decoration: decoration: unknown field "colour"; it takes {type?, in?`},
		{`{"op": "batch", "ops": [{"op": "connect", "from": {"node": "a"}, "to": {"node": "b"}, "labels": {"label": "x"}}]}`, `batch operation 1: connect: labels: unknown field "label"; it takes`},
	}
	for _, tt := range tests {
		_, err := DecodeOps([]byte(tt.src))
		if tt.err == "" && err != nil || tt.err != "" && (err == nil || !strings.Contains(err.Error(), tt.err)) {
			t.Errorf("DecodeOps(%s): %v, want %q", tt.src, err, tt.err)
		}
	}
}

// TestShapes checks that the shape of every nested field names exactly the keys its schema has.
func TestShapes(t *testing.T) {
	via := regexp.MustCompile(`\[\[x, y\]\]`)
	name := regexp.MustCompile(`[a-z_]+`)
	for _, d := range Defs {
		for _, f := range d.Fields {
			var schema any
			if err := json.Unmarshal([]byte(f.Schema), &schema); err != nil {
				t.Fatal(err)
			}
			want := schemaKeys(schema)
			slices.Sort(want)
			got := name.FindAllString(via.ReplaceAllString(f.Shape, ""), -1)
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("%s %s: shape %q names %v, schema %v", d.Name, f.Name, f.Shape, got, want)
			}
		}
	}
}

// schemaKeys returns the names of the properties in a parsed JSON schema, at any depth.
func schemaKeys(schema any) []string {
	s, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	keys := schemaKeys(s["items"])
	props, _ := s["properties"].(map[string]any)
	for k, v := range props {
		keys = append(append(keys, k), schemaKeys(v)...)
	}
	return keys
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
		_, err := Apply(fsys.Disk(dir), mustDecode(t, tt.op), NewSession())
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
	c, err := Apply(fsys.Disk(dir), mustDecode(t, `{"op": "create_node", "kind": "service"}`), sess)
	if err != nil {
		t.Fatal(err)
	}
	h.Record(c)
	if !sess.kindIDs["service"] {
		t.Fatal("the kind-derived ID is not in the session")
	}
	if _, err := h.Undo(fsys.Disk(dir), sess); err != nil || sess.kindIDs["service"] {
		t.Fatalf("after undo: %v, session %v", err, sess.kindIDs)
	}
	if _, err := h.Redo(fsys.Disk(dir), sess); err != nil || !sess.kindIDs["service"] {
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
			_, err := Apply(fsys.Disk(dir), mustDecode(t, fmt.Sprintf(`{"op": "create_node", "id": "n%d", "kind": "service"}`, i)), NewSession())
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

// TestInstalledPackReadOnly checks that an operation fails naming a file of an installed pack,
// which no read_only pattern lists.
func TestInstalledPackReadOnly(t *testing.T) {
	dir := t.TempDir()
	lock := project.Lock{Packs: map[string]project.LockEntry{"aws": {
		Source: "example.com/aws", Version: "v1.0.0", Commit: strings.Repeat("a", 40), Hash: "sha256:" + strings.Repeat("0", 64),
	}}}
	files := map[string]string{
		"dg.hcl":             "project \"p\" {\n  models = [\"packs/aws/more.hcl\"]\n  pack \"aws\" {\n    source  = \"example.com/aws\"\n    version = \"v1.0.0\"\n  }\n}\n",
		"dg.lock":            string(lock.Encode()),
		"packs/aws/spec.hcl": "kind \"vpc\" {}\n",
		"packs/aws/more.hcl": "node \"a\" {\n  kind = \"aws-vpc\"\n}\n",
	}
	for name, src := range files {
		if err := errors.Join(os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755), os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644)); err != nil {
			t.Fatal(err)
		}
	}
	before := readTree(t, dir)
	_, err := Apply(fsys.Disk(dir), mustDecode(t, `{"op": "set_label", "id": "a", "label": "A"}`), NewSession())
	if err == nil || !strings.Contains(err.Error(), "packs/aws/more.hcl is read-only") {
		t.Fatalf("error %v, want packs/aws/more.hcl is read-only", err)
	}
	sameTree(t, readTree(t, dir), before, "after the refusal")
}

// TestReadOnlyViews checks that view and decoration operations refuse to write a read-only
// views.hcl or layout file, naming it, and write nothing.
func TestReadOnlyViews(t *testing.T) {
	tests := []struct{ readOnly, op, file string }{
		{"views.hcl", `{"op": "create_view", "id": "v"}`, "views.hcl"},
		{"views.hcl", `{"op": "update_view", "id": "main", "title": "T"}`, "views.hcl"},
		{"views.hcl", `{"op": "delete_view", "id": "main"}`, "views.hcl"},
		{"views.hcl", `{"op": "set_view_label", "view": "main", "id": "a", "label": "A"}`, "views.hcl"},
		{"views/*", `{"op": "delete_view", "id": "main"}`, "views/main.layout.yaml"},
		{"views/*", `{"op": "add_decoration", "view": "main", "decoration": {"type": "text", "text": "T"}}`, "views/main.layout.yaml"},
		{"views/*", `{"op": "update_decoration", "view": "main", "id": "t", "decoration": {"x": 10}}`, "views/main.layout.yaml"},
		{"views/*", `{"op": "delete_decoration", "view": "main", "id": "t"}`, "views/main.layout.yaml"},
		{"views/*", `{"op": "promote_decoration", "view": "main", "id": "t", "kind": "note"}`, "views/main.layout.yaml"},
		{"model.hcl", `{"op": "promote_decoration", "view": "main", "id": "t", "kind": "note"}`, "model.hcl"},
		{"views/*", `{"op": "reorder", "view": "main", "container": "s", "order": ["c"]}`, "views/main.layout.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{
				"dg.hcl":                 fmt.Sprintf("project \"p\" {\n  models    = [\"model.hcl\"]\n  read_only = [%q]\n}\n", tt.readOnly),
				"model.hcl":              "node \"a\" {}\n\nnode \"s\" {\n  kind = \"lane-set\"\n}\n\nnode \"c\" {\n  parent = node.s\n}\n",
				"views.hcl":              "view \"main\" {}\n",
				"views/main.layout.yaml": "version: 1\ndecorations:\n  - {id: t, type: text, x: 0, y: 0, text: T}\n",
			}
			for name, src := range files {
				if err := errors.Join(os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755), os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644)); err != nil {
					t.Fatal(err)
				}
			}
			before := readTree(t, dir)
			_, err := Apply(fsys.Disk(dir), mustDecode(t, tt.op), NewSession())
			if err == nil || !strings.Contains(err.Error(), tt.file+" is read-only") {
				t.Fatalf("error %v, want %s is read-only", err, tt.file)
			}
			sameTree(t, readTree(t, dir), before, "after the refusal")
		})
	}
}

// TestRemoveViewLabel checks that removing a view's label leaves the views file as it was, and
// that removing one the view does not have writes nothing.
func TestRemoveViewLabel(t *testing.T) {
	dir := copyFixture(t)
	before := readTree(t, dir)
	remove := `{"op": "set_view_label", "view": "calls", "id": "gateway"}`
	c, err := Apply(fsys.Disk(dir), mustDecode(t, remove), NewSession())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 0 {
		t.Errorf("removing no label wrote %v", c.Files)
	}
	if _, err := Apply(fsys.Disk(dir), mustDecode(t, `{"op": "set_view_label", "view": "calls", "id": "gateway", "label": "One"}`), NewSession()); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(fsys.Disk(dir), mustDecode(t, remove), NewSession()); err != nil {
		t.Fatal(err)
	}
	sameTree(t, readTree(t, dir), before, "after the removal")
}

// TestUpdateViewKeepsOverlays checks that update_view leaves an overlay block it does not change
// as it is written, comments included, and replaces the others.
func TestUpdateViewKeepsOverlays(t *testing.T) {
	dir := t.TempDir()
	views := "view \"v\" {\n  overlay \"keep\" {\n    members = [node.a] # the first\n  }\n\n  overlay \"gone\" {\n    members = [node.a]\n  }\n}\n"
	for name, src := range map[string]string{"model.hcl": "node \"a\" {}\n", "views.hcl": views} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	op := `{"op": "update_view", "id": "v", "overlays": [{"id": "keep", "members": ["node.a"]}, {"id": "new", "members": ["all"], "label": "All"}]}`
	if _, err := Apply(fsys.Disk(dir), mustDecode(t, op), NewSession()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "views.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	want := "view \"v\" {\n  overlay \"keep\" {\n    members = [node.a] # the first\n  }\n  overlay \"new\" {\n    members = [all]\n    label   = \"All\"\n  }\n}\n"
	if string(got) != want {
		t.Errorf("views.hcl:\n%s\nwant:\n%s", got, want)
	}
}

// TestStaysCanonical applies each golden operation to the fixture in canonical form (dg fmt)
// and checks that every file it writes is still canonical.
func TestStaysCanonical(t *testing.T) {
	for _, tt := range goldenOps {
		t.Run(tt.name, func(t *testing.T) {
			dir := copyFixture(t)
			p, _, err := project.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := Format(fsys.Disk(dir), p, false); err != nil {
				t.Fatal(err)
			}
			if _, err := Apply(fsys.Disk(dir), mustDecode(t, tt.op), NewSession()); err != nil {
				t.Fatal(err)
			}
			if p, _, err = project.Load(dir); err != nil {
				t.Fatal(err)
			}
			changed, _, err := Format(fsys.Disk(dir), p, true)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range changed {
				data, _ := os.ReadFile(filepath.Join(dir, name))
				t.Errorf("%s is not canonical after the operation:\n%s", name, data)
			}
		})
	}
}

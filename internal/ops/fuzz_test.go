package ops

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"testing"
	"testing/fstest"
)

// FuzzOp decodes an operation and applies it, without writing, to testdata/ops/in with its
// model.hcl replaced by the fuzzed one (empty: kept).
func FuzzOp(f *testing.F) {
	base := fstest.MapFS{}
	err := fs.WalkDir(os.DirFS(fixture), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(os.DirFS(fixture), path)
		base[path] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{
		`{"op": "create_node", "kind": "service", "label": "Billing API", "view": "calls", "x": 10, "y": 10}`,
		`{"op": "delete_node", "id": "pool", "recursive": true}`,
		`{"op": "delete_node", "id": "rack"}`,
		`{"op": "set_label", "id": "gateway", "label": "${x}"}`,
		`{"op": "set_props", "id": "worker-1", "props": {"gpus": 1e400}}`,
		`{"op": "set_rows", "id": "orders", "compartments": [{"name": "a", "rows": [{"name": "x\"y"}]}]}`,
		`{"op": "connect", "from": {"node": "gateway"}, "to": {"node": "orders", "row": "id"}, "view": "calls"}`,
		`{"op": "rename_id", "type": "node", "old": "worker-1", "new": "a[\"b\"]"}`,
		`{"op": "rename_id", "type": "view", "old": "main", "new": "calls"}`,
		`{"op": "set_edge_hint", "view": "main", "id": "lift:a%3Ab:pool", "hint": {"via": [[1, 2]]}}`,
		`{"op": "pin_port", "view": "main", "id": "worker-1", "port": "nic1", "side": "left", "offset": 1}`,
		`{"op": "create_view", "id": "v", "include": ["descendants[\"pool\"]"], "edges": ["edge_kind.async"], "of": "pool"}`,
		`[{"op": "create_node", "kind": "group"}, {"op": "set_label", "id": "group", "label": "Rack 2"}, {"op": "move", "view": "main", "id": "rack-2", "x": -5, "y": 5}]`,
	} {
		f.Add([]byte(seed), []byte{})
	}
	for _, model := range []string{
		"node \"gateway\" { kind = \"service\" }\nnode \"pool\" {}\nnode \"worker-1\" {\n  kind = /* c */ \"worker\" # t\n  parent = node.pool\n}\n",
		"node \"gateway\" {\n  label = <<-EOT\n    G\n    EOT\n}\nedge \"gw-w1\" {\n  from = node.gateway\n  to = node[\"worker-1\"]\n}\nnode \"worker-1\" {}",
	} {
		f.Add([]byte(`{"op": "set_label", "id": "gateway", "label": "x"}`), []byte(model))
		f.Add([]byte(`{"op": "add_port", "id": "pool", "name": "p"}`), []byte(model))
		f.Add([]byte(`{"op": "delete_node", "id": "worker-1"}`), []byte(model))
		f.Add([]byte(`{"op": "rename_id", "type": "node", "old": "gateway", "new": "g/w"}`), []byte(model))
	}
	f.Fuzz(func(t *testing.T, data, model []byte) {
		fsys := base
		if len(model) > 0 {
			fsys = maps.Clone(base)
			fsys["model.hcl"] = &fstest.MapFile{Data: model}
		}
		op, err := DecodeOps(data)
		if err != nil {
			return
		}
		again, err := json.Marshal(op)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeOp(again); err != nil {
			t.Fatalf("%s does not decode again: %v", again, err)
		}
		plan(fsys, ".", op, NewSession())
	})
}

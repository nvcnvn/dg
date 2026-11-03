package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func copyProject(t testing.TB, src string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	return dir
}

func connect(t *testing.T, dir string) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	if _, err := New(dir).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestTools(t *testing.T) {
	cs := connect(t, copyProject(t, "../../testdata/ops/in"))
	ctx := context.Background()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if got := strings.Join(names, " "); got != "check export op vocabulary" {
		t.Errorf("tools %q", got)
	}
	tests := []struct {
		tool  string
		args  any
		isErr bool
		want  string
	}{
		{"check", map[string]any{}, false, "[]\n"},
		{"check", map[string]any{"strict": true}, true, `unknown field "strict"`},
		{"vocabulary", nil, false, `"name": "worker",`},
		{"vocabulary", nil, false, `"name": "gpus",
          "type": "number"`},
		{"vocabulary", nil, false, `"tags": [
    "edge",`},
		{"op", map[string]any{"op": "set_label", "id": "gateway", "label": "Front"}, false, `"files": [
    "model.hcl"
  ]`},
		{"op", map[string]any{"op": "set_kind", "id": "pool", "kind": "service"}, true, `"summary": "Parent is not a container"`},
		{"op", map[string]any{"op": "set_label", "id": "ext", "label": "x"}, true, `generated.hcl is read-only`},
		{"op", map[string]any{"op": "fly"}, true, `unknown operation \"fly\"`},
		{"op", []any{1}, true, `an operation is a JSON object`},
		{"export", map[string]any{"view": "nope"}, true, `view "nope" is not defined`},
		{"export", map[string]any{"format": "pdf"}, true, `unknown format "pdf"`},
	}
	for _, tt := range tests {
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: tt.tool, Arguments: tt.args})
		if err != nil {
			t.Fatalf("%s %v: %v", tt.tool, tt.args, err)
		}
		text := res.Content[0].(*sdk.TextContent).Text
		if res.IsError != tt.isErr || !strings.Contains(text, tt.want) {
			t.Errorf("%s %v: error %v, %s\nwant error %v and %q", tt.tool, tt.args, res.IsError, text, tt.isErr, tt.want)
		}
	}
}

func TestExportPNG(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	cs := connect(t, copyProject(t, "../../examples/hello"))
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "export", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	img, ok := res.Content[0].(*sdk.ImageContent)
	if !ok || img.MIMEType != "image/png" {
		t.Fatalf("content %+v is not a PNG image", res.Content[0])
	}
	if _, err := png.Decode(bytes.NewReader(img.Data)); err != nil {
		t.Fatal(err)
	}
}

// FuzzTools calls the check, vocabulary and op tools with arbitrary arguments on a copy of
// testdata/ops/in, restored before each call.
func FuzzTools(f *testing.F) {
	for _, seed := range []struct {
		tool uint8
		args string
	}{
		{0, `{}`}, {1, `null`}, {2, `{"op": "create_node", "kind": "service", "view": "calls", "x": 1, "y": 2}`},
		{2, `{"op": "batch", "ops": [{"op": "delete_node", "id": "rack", "recursive": true}]}`},
		{2, `{"op": "rename_id", "type": "view", "old": "calls", "new": "main"}`}, {2, `[]`}, {0, `{"x": 1}`},
		{2, `{"op": "batch", "ops": [{"op": "set_kind", "id": "pool", "kind": "lane-set"}, {"op": "reorder", "view": "pool", "container": "pool", "order": ["worker-2"]}]}`},
	} {
		f.Add(seed.tool, []byte(seed.args))
	}
	src := os.DirFS("../../testdata/ops/in")
	dir := f.TempDir()
	s := &Server{Dir: dir}
	tools := s.Tools()
	f.Fuzz(func(t *testing.T, tool uint8, args []byte) {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.CopyFS(dir, src); err != nil {
			t.Fatal(err)
		}
		call := tools[int(tool)%3].Call
		if res := call(context.Background(), json.RawMessage(args)); res == nil || len(res.Content) == 0 {
			t.Fatalf("no result for %s", args)
		}
	})
}

// TestConcurrentOps calls the op tool from many goroutines at once: every call succeeds and is
// in the files.
func TestConcurrentOps(t *testing.T) {
	dir := copyProject(t, "../../testdata/ops/in")
	cs := connect(t, dir)
	const n = 20
	errs := make(chan string, n)
	for i := range n {
		go func() {
			res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "op", Arguments: map[string]any{"op": "create_node", "kind": "service", "id": fmt.Sprintf("n%d", i)}})
			switch {
			case err != nil:
				errs <- err.Error()
			case res.IsError:
				errs <- res.Content[0].(*sdk.TextContent).Text
			default:
				errs <- ""
			}
		}()
	}
	for range n {
		if e := <-errs; e != "" {
			t.Error(e)
		}
	}
	model, err := os.ReadFile(filepath.Join(dir, "model.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		if !strings.Contains(string(model), fmt.Sprintf("node \"n%d\"", i)) {
			t.Errorf("node n%d is missing", i)
		}
	}
}

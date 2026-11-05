package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nvcnvn/dg/internal/mcp"
	"github.com/nvcnvn/dg/internal/ops"
)

// TestMain lets the test binary act as dg, so scripts can run it: with DG_TEST_AS_DG set it runs
// the command line it is given, and with DG_TEST_RECORD set too, dg op appends each operation
// as a JSON line to that file instead of applying it.
func TestMain(m *testing.M) {
	if os.Getenv("DG_TEST_AS_DG") != "" {
		if record := os.Getenv("DG_TEST_RECORD"); record != "" {
			applyOp = func(_ string, op ops.Op, _ *ops.Session) (*ops.Change, error) {
				data, err := json.Marshal(op)
				if err != nil {
					return nil, err
				}
				f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					return nil, err
				}
				_, err = f.Write(append(data, '\n'))
				return &ops.Change{}, errors.Join(err, f.Close())
			}
		}
		os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func TestOp(t *testing.T) {
	dir := t.TempDir()
	steps := []struct {
		args   []string
		stdin  string
		code   int
		stdout string
		stderr string
	}{
		{args: []string{"op"}, code: 1, stderr: "create_node"},
		{args: []string{"op", "--help"}, code: 0, stderr: "set_edge_hint"},
		{args: []string{"op", "draw"}, code: 1, stderr: `unknown operation "draw"`},
		{args: []string{"op", "connect", "--help"}, code: 0, stderr: "-from-port"},
		{args: []string{"op", "connect", "--help"}, code: 0, stderr: "fields: from: {node, port?, row?}, to: {node, port?, row?}, id?, kind?, labels?: {start?, mid?, end?}, tags?, view?\n"},
		{args: []string{"op", "set_edge_hint", "--help"}, code: 0, stderr: "hint: {from_side?, to_side?, via?: [[x, y]], label_pos?}\n"},
		{args: []string{"op", "batch"}, stdin: `{"op": "connect", "from": {"node": "a"}, "to": {"node": "b"}, "labels": {"middle": "x"}}`, code: 1, stderr: `labels: unknown field "middle"; it takes {start?, mid?, end?}`},
		{args: []string{"vocabulary"}, stdout: `"name": "lane-set",`},
		{args: []string{"op", "create_node", "--label", "API"}, code: 1, stderr: `missing field "kind"`},
		{args: []string{"op", "create_node", "--kind", "service", "--label", "API", "--x", "1"}, code: 1, stderr: "x and y together"},
		{args: []string{"op", "create_node", "--kind", "service", "--label", "API"}, stdout: "create_node api\nwrote model.hcl\n"},
		{args: []string{"op", "create_node", "--kind", "group", "--id", "pool", "--view", "main", "--x", "-12", "--y", "26"}, stdout: "create_node pool x=-10 y=30\nwrote model.hcl\nwrote views/main.layout.yaml\n"},
		{args: []string{"op", "set_parent", "--id", "api", "--parent", "pool"}, stdout: "set_parent api\nwrote model.hcl\n"},
		{args: []string{"op", "set_tags", "--id", "api", "--tags", "hot", "--tags", "new"}, stdout: "set_tags api\nwrote model.hcl\n"},
		{args: []string{"op", "add_port", "--id", "api", "--name", "grpc", "--side", "right"}, stdout: "add_port api\nwrote model.hcl\n"},
		{args: []string{"op", "create_node", "--kind", "database", "--label", "Orders"}, stdout: "create_node orders\nwrote model.hcl\n"},
		{args: []string{"op", "connect", "--from", "api", "--from-port", "grpc", "--to", "orders", "--start-label", "1", "--end-label", "*", "--json"}, stdout: `"id": "api-orders"`},
		{args: []string{"op", "set_edge_hint", "--view", "main", "--id", "api-orders", "--via", "100,40", "--via", "100, 80", "--label-pos", "0.5"}, stdout: "set_edge_hint api-orders\nwrote views/main.layout.yaml\n"},
		{args: []string{"op", "set_props", "--id", "orders", "--props", "{bad"}, code: 1, stderr: `--props: "{bad" is not valid JSON`},
		{args: []string{"op", "set_kind", "--id", "pool", "--kind", "service", "--json"}, code: 1, stdout: `"summary": "Parent is not a container"`},
		{args: []string{"op", "batch"}, stdin: `[{"op": "create_node", "kind": "service", "label": "Search"}, {"op": "connect", "from": {"node": "search"}, "to": {"node": "api"}}]`, stdout: "create_node search\nconnect search-api\nwrote model.hcl\n"},
		{args: []string{"op", "batch"}, stdin: `{"op": "unpin", "view": "main", "id": "pool"}`, stdout: "unpin pool\n"},
		{args: []string{"op", "update_view", "--id", "main", "--collapse", "node.pool", "--direction", "right"}, stdout: "update_view main\nwrote views.hcl\n"},
		{args: []string{"op", "update_view", "--id", "main", "--unset", "collapse", "--overlays", `[{"id": "hot", "members": ["tag.hot"], "label": "Hot"}]`}, stdout: "update_view main\nwrote views.hcl\n"},
		{args: []string{"op", "add_decoration", "--view", "main", "--decoration", `{"type": "text", "x": 3, "y": -61, "text": "Title"}`}, stdout: "add_decoration text\nwrote views/main.layout.yaml\n"},
		{args: []string{"op", "update_decoration", "--view", "main", "--id", "text", "--decoration", `{"font_size": 20}`}, stdout: "update_decoration text\nwrote views/main.layout.yaml\n"},
		{args: []string{"op", "promote_decoration", "--view", "main", "--id", "text", "--kind", "note"}, stdout: "promote_decoration title x=0 y=-60\nwrote model.hcl\nwrote views/main.layout.yaml\n"},
		{args: []string{"op", "batch"}, stdin: `[{"op": "create_node", "kind": "lane-set", "id": "lanes"}, {"op": "create_node", "kind": "lane", "id": "b", "parent": "lanes"}, {"op": "create_node", "kind": "lane", "id": "a", "parent": "lanes"}]`, stdout: "wrote model.hcl\n"},
		{args: []string{"op", "reorder", "--view", "main", "--container", "lanes", "--order", "b", "--order", "a"}, stdout: "reorder lanes\nwrote views/main.layout.yaml\n"},
		{args: []string{"op", "move", "--view", "main", "--id", "a", "--x", "0", "--y", "0"}, code: 1, stderr: "reorder it instead"},
		{args: []string{"op", "create_view", "--id", "old"}, stdout: "create_view old\nwrote views.hcl\n"},
		{args: []string{"op", "delete_view", "--id", "old"}, stdout: "delete_view old\nwrote views.hcl\n"},
		{args: []string{"op", "move", "-C", filepath.Join(dir, "missing"), "--view", "main", "--id", "a", "--x", "0", "--y", "0"}, code: 2},
	}
	for _, st := range steps {
		args := st.args
		if len(args) > 1 && !strings.HasPrefix(args[1], "-") && !strings.Contains(strings.Join(args, " "), " -C ") {
			args = append([]string{args[0], args[1], "-C", dir}, args[2:]...)
		}
		var stdout, stderr bytes.Buffer
		code := withStdin(t, st.stdin, func() int { return run(args, &stdout, &stderr) })
		if code != st.code {
			t.Fatalf("%v: exit code %d, want %d; stdout:\n%s\nstderr:\n%s", st.args, code, st.code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), st.stdout) {
			t.Errorf("%v: stdout %q does not contain %q", st.args, stdout.String(), st.stdout)
		}
		if !strings.Contains(stderr.String(), st.stderr) {
			t.Errorf("%v: stderr %q does not contain %q", st.args, stderr.String(), st.stderr)
		}
	}
	if code := run([]string{"check", "--strict", "-C", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Errorf("dg check --strict exits %d after the operations", code)
	}
}

func withStdin(t *testing.T, input string, f func() int) int {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		w.WriteString(input)
		w.Close()
	}()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old; r.Close() }()
	return f()
}

// rebuild runs testdata/rebuild/<example>.sh into a new directory with this test binary as dg,
// applying the operations or, with record set, recording them there.
func rebuild(t *testing.T, example, record string) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("bash", "../../testdata/rebuild/"+example+".sh", dir)
	cmd.Env = append(os.Environ(), "DG="+os.Args[0], "DG_TEST_AS_DG=1", "DG_TEST_RECORD="+record)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the script failed: %v\n%s", err, out)
	}
	return dir
}

var c4Goldens = map[string]string{
	"main":    "../../testdata/export/c4-container.svg",
	"pinned":  "../../testdata/export/c4-container-pinned.svg",
	"sign-in": "../../testdata/export/c4-container-sign-in.svg",
}

// TestRebuild rebuilds examples with dg op alone and exports every view, which must match the
// goldens of the example.
func TestRebuild(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	for example, goldens := range map[string]map[string]string{
		"c4-container": c4Goldens,
		"c4": {
			"main":       "../../testdata/export/c4.svg",
			"containers": "../../testdata/export/c4-containers.svg",
			"components": "../../testdata/export/c4-components.svg",
		},
		"incident": {
			"main":     "../../testdata/export/incident.svg",
			"incident": "../../testdata/export/incident-incident.svg",
		},
		"swimlane": {
			"main":  "../../testdata/export/swimlane.svg",
			"lanes": "../../testdata/export/swimlane-lanes.svg",
		},
		"flowchart":     {"main": "../../testdata/export/flowchart.svg"},
		"state-machine": {"main": "../../testdata/export/state-machine.svg"},
	} {
		t.Run(example, func(t *testing.T) {
			dir := rebuild(t, example, "")
			var stdout, stderr bytes.Buffer
			if code := run([]string{"check", "--strict", "-C", dir}, &stdout, &stderr); code != 0 {
				t.Fatalf("dg check: %s%s", stdout.String(), stderr.String())
			}
			out := t.TempDir()
			if code := run([]string{"export", "-C", dir, "--all", "--out", out}, &stdout, &stderr); code != 0 {
				t.Fatalf("dg export: %s", stderr.String())
			}
			for v, golden := range goldens {
				got, _ := os.ReadFile(filepath.Join(out, v+".svg"))
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("view %s differs from %s", v, golden)
				}
			}
		})
	}
}

// TestRebuildMCP builds the same diagram through an in-process MCP client: the operations the
// script makes go to the op tool one by one, and the export tool draws every view.
func TestRebuildMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	record := filepath.Join(t.TempDir(), "ops.jsonl")
	dir := rebuild(t, "c4-container", record)
	lines, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	if _, err := mcp.New(dir).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, line := range strings.Split(strings.TrimSpace(string(lines)), "\n") {
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "op", Arguments: json.RawMessage(line)})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("op %s: %s", line, res.Content[0].(*sdk.TextContent).Text)
		}
	}
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "check", Arguments: map[string]any{}})
	if err != nil || res.Content[0].(*sdk.TextContent).Text != "[]\n" {
		t.Fatalf("check: %v %+v", err, res)
	}
	for v, golden := range c4Goldens {
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "export", Arguments: map[string]any{"view": v, "format": "svg"}})
		if err != nil || res.IsError {
			t.Fatalf("export %s: %v %+v", v, err, res)
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if got := res.Content[0].(*sdk.TextContent).Text; got != string(want) {
			t.Errorf("view %s differs from %s", v, golden)
		}
	}
}

// FuzzOpArgs parses dg op command lines (arguments separated by NUL) into operations.
func FuzzOpArgs(f *testing.F) {
	for _, seed := range []string{
		"connect\x00--from\x00a\x00--from-port\x00p\x00--to\x00b\x00--label\x00x\x00--end-label\x00y",
		"set_edge_hint\x00--view\x00main\x00--id\x00e\x00--via\x0010,20\x00--label-pos\x000.5\x00--from-side\x00top",
		"reorder\x00--view\x00main\x00--container\x00lanes\x00--order\x00b\x00--order\x00a",
		"set_rows\x00--id\x00t\x00--compartments\x00[{\"rows\":[{\"name\":\"id\"}]}]",
		"set_props\x00--id\x00n\x00--props\x00{\"a\":1}",
		"create_view\x00--id\x00v\x00--include\x00node.a\x00--edges\x00edge_kind.k",
		"move\x00--view\x00main\x00--id\x00a\x00--x\x001e3\x00--y\x00-0",
		"set_tags\x00--id\x00a",
		"update_view\x00--id\x00main\x00--collapse\x00node.g\x00--unset\x00edges\x00--overlays\x00[{\"id\":\"o\",\"members\":[\"all\"]}]\x00--direction\x00right",
		"add_decoration\x00--view\x00main\x00--decoration\x00{\"type\":\"arrow\",\"points\":[[0,0],[1,1]]}",
		"update_decoration\x00--view\x00main\x00--id\x00t\x00--unset\x00attach",
		"promote_decoration\x00--view\x00main\x00--id\x00t\x00--kind\x00note\x00--new\x00n\x00--label\x00L",
		"create_node\x00--kind\x00service\x00--view\x00main\x00--parent\x00lanes\x00--before\x00b",
		"show_node\x00--view\x00main\x00--id\x00a",
		"hide_node\x00--view\x00main\x00--id\x00a",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		args := strings.Split(line, "\x00")
		def, ok := ops.LookupDef(args[0])
		if !ok || def.Name == "batch" {
			return
		}
		fl := newOpFlagSet(def)
		values := map[string]any{}
		defineOpFlags(fl, def, values)
		if fl.Parse(args[1:]) != nil {
			return
		}
		if op, err := opFromFlags(fl, def, values); err == nil && op.Op != def.Name {
			t.Errorf("parsed %q as %q", def.Name, op.Op)
		}
	})
}

// TestConcurrentProcesses runs dg op in many processes at once on one project: each that
// reports success is in the files.
func TestConcurrentProcesses(t *testing.T) {
	dir := t.TempDir()
	const n = 10
	cmds := make([]*exec.Cmd, n)
	for i := range n {
		cmds[i] = exec.Command(os.Args[0], "op", "create_node", "-C", dir, "--kind", "service", "--id", fmt.Sprintf("n%d", i))
		cmds[i].Env = append(os.Environ(), "DG_TEST_AS_DG=1")
		if err := cmds[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Errorf("process %d: %v", i, err)
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

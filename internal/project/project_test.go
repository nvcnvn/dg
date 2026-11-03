package project

import (
	"os"
	"testing/fstest"

	"github.com/nvcnvn/dg/internal/style"

	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadDiagnostics(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "valid",
			files: map[string]string{"model.hcl": `
node "g" {
  kind = "group"
}
node "a" {
  parent = node.g
}
node "kube/dns" {}
edge "e" {
  from = node.a
  to   = node["kube/dns"]
}
`},
		},
		{
			name:  "syntax error",
			files: map[string]string{"model.hcl": "node \"a\" {\n  label = \n}\n"},
			want:  []string{"model.hcl:2:11: error: Invalid expression: Expected the start of an expression, but found an invalid expression token."},
		},
		{
			name:  "duplicate node",
			files: map[string]string{"a.hcl": `node "x" {}`, "b.hcl": `node "x" {}`},
			want:  []string{`b.hcl:1:1: error: Duplicate ID: The node ID "x" is already defined at a.hcl:1:1.`},
		},
		{
			name:  "reserved and control-character IDs",
			files: map[string]string{"model.hcl": "node \"all\" {}\nnode \"a\\tb\" {}\n"},
			want: []string{
				`model.hcl:1:1: error: Invalid ID: The node ID "all" is reserved.`,
				`model.hcl:2:1: error: Invalid ID: The node ID "a\tb" contains a control character.`,
			},
		},
		{
			name:  "undefined kind and edge kind",
			files: map[string]string{"model.hcl": "node \"a\" {\n  kind = \"nope\"\n}\nedge \"e\" {\n  from = node.a\n  to   = node.b\n  kind = \"nope\"\n}\nnode \"b\" {}\n"},
			want: []string{
				`model.hcl:1:1: error: Undefined kind: Node "a" has undefined kind "nope".`,
				`model.hcl:4:1: error: Undefined edge kind: Edge "e" has undefined edge kind "nope".`,
			},
		},
		{
			name:  "parent not a container",
			files: map[string]string{"model.hcl": "node \"a\" {}\nnode \"b\" {\n  parent = node.a\n}\n"},
			want:  []string{`model.hcl:3:12: error: Parent is not a container: Parent "a" of node "b" has kind "node", which is not a container.`},
		},
		{
			name:  "containment cycle",
			files: map[string]string{"model.hcl": "node \"a\" {\n  kind   = \"group\"\n  parent = node.b\n}\nnode \"b\" {\n  kind   = \"group\"\n  parent = node.a\n}\n"},
			want: []string{
				`model.hcl:3:12: error: Containment cycle: Node "a" contains itself through its parents.`,
				`model.hcl:7:12: error: Containment cycle: Node "b" contains itself through its parents.`,
			},
		},
		{
			name:  "bad references",
			files: map[string]string{"model.hcl": "edge \"e\" {\n  from = node.a.port.p\n  to   = node.missing\n}\nedge \"f\" {\n  from = node.missing\n  to   = node.missing\n}\n"},
			want: []string{
				`model.hcl:2:10: error: Invalid node reference: Expected node.<id> or node["<id>"]. Port and row references are not supported yet.`,
				`model.hcl:6:10: error: Undefined node: Edge "f" references undefined node "missing".`,
				`model.hcl:7:10: error: Undefined node: Edge "f" references undefined node "missing".`,
			},
		},
		{
			name: "kinds and styles",
			files: map[string]string{
				"spec.hcl":  "kind \"a\" {\n  extends = \"b\"\n}\nkind \"b\" {\n  extends = \"a\"\n}\nkind \"c\" {\n  extends = \"zzz\"\n}\n",
				"model.hcl": "",
				"style.hcl": "style \"service\" {\n  shape = \"blob\"\n  fill  = \"red\"\n}\nstyle \"tag.x\" {}\nstyle \"ghost\" {}\nedge_style \"default\" {\n  fill = \"#FFF\"\n}\n",
			},
			want: []string{
				`spec.hcl:1:1: error: Extends cycle: Kind "a" extends itself through its ancestors.`,
				`spec.hcl:4:1: error: Extends cycle: Kind "b" extends itself through its ancestors.`,
				`spec.hcl:7:1: error: Undefined kind: Kind "c" extends undefined kind "zzz".`,
				`style.hcl:2:11: error: Invalid style value: "blob" is not a valid value for shape.`,
				`style.hcl:3:11: error: Invalid style value: "red" is not a valid value for fill.`,
				`style.hcl:5:1: error: Unsupported style label: Style label "tag.x": only kind labels are supported.`,
				`style.hcl:6:1: warning: Undefined kind: Style label "ghost" names no defined kind.`,
				`style.hcl:8:3: error: Unsupported argument: An argument named "fill" is not expected here.`,
			},
		},
		{
			name: "container inherited through extends",
			files: map[string]string{
				"spec.hcl":  "kind \"zone\" {\n  extends = \"group\"\n}\n",
				"model.hcl": "node \"z\" {\n  kind = \"zone\"\n}\nnode \"a\" {\n  parent = node.z\n}\n",
			},
		},
		{
			name:  "missing edge ends",
			files: map[string]string{"model.hcl": "node \"a\" {}\nedge \"e\" {\n  from = node.a\n}\n"},
			want:  []string{`model.hcl:2:1: error: Missing required argument: The argument "to" is required.`},
		},
		{
			name: "unsupported edges",
			files: map[string]string{"model.hcl": "node \"g\" {\n  kind = \"group\"\n}\nnode \"h\" {\n  kind   = \"group\"\n  parent = node.g\n}\nnode \"a\" {\n  parent = node.h\n}\n" +
				"edge \"loop\" {\n  from = node.a\n  to   = node.a\n}\nedge \"down\" {\n  from = node.g\n  to   = node.a\n}\nedge \"up\" {\n  from = node.a\n  to   = node.h\n}\n"},
			want: []string{
				`model.hcl:11:1: error: Unsupported edge: Edge "loop" starts and ends at "a"; self-loops are not supported yet.`,
				`model.hcl:15:1: error: Unsupported edge: Edge "down" connects a container to its own descendant; such edges are not supported yet.`,
				`model.hcl:19:1: error: Unsupported edge: Edge "up" connects a container to its own descendant; such edges are not supported yet.`,
			},
		},
		{
			name:  "duplicate kinds in one file",
			files: map[string]string{"spec.hcl": "kind \"x\" {}\nkind \"x\" {}\nedge_kind \"y\" {}\nedge_kind \"y\" {}\n", "model.hcl": ""},
			want: []string{
				`spec.hcl:2:1: error: Duplicate definition: The kind "x" is already defined at line 1 of this file.`,
				`spec.hcl:4:1: error: Duplicate definition: The edge kind "y" is already defined at line 3 of this file.`,
			},
		},
		{
			name:  "no model files",
			files: map[string]string{"spec.hcl": ""},
			want:  []string{"warning: No model files: DIR has no model files (*.hcl other than dg.hcl, spec.hcl, style.hcl and views.hcl)."},
		},
		{
			name:  "unsupported files",
			files: map[string]string{"dg.hcl": `project "x" {}`, "views.hcl": ``, "model.hcl": ``},
			want: []string{
				"dg.hcl:1:1: error: Unsupported file: dg.hcl is not supported yet.",
				"views.hcl:1:1: error: Unsupported file: views.hcl is not supported yet.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, src := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, diags, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, d := range diags {
				f := strings.ReplaceAll(FormatDiagnostic(d), dir+string(filepath.Separator), "")
				got = append(got, strings.ReplaceAll(f, dir, "DIR"))
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestLoadNotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "model.hcl")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(t.TempDir(), "missing"), file} {
		if _, _, err := Load(dir); err == nil {
			t.Errorf("Load(%s) succeeded", dir)
		}
	}
}

func TestStyleCascade(t *testing.T) {
	fsys := fstest.MapFS{
		"spec.hcl":  {Data: []byte("kind \"zone\" {\n  extends = \"group\"\n}\nedge_kind \"async\" {}\n")},
		"style.hcl": {Data: []byte("style \"zone\" {\n  shape = \"rounded\"\n}\n")},
		"model.hcl": {},
	}
	p, diags, err := load(fsys, ".")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	if got, want := p.Styles.ForNode(p.Spec, "zone"), (style.Node{Shape: "rounded", Fill: "#F8F9FA", Stroke: "#9AA0A6"}); got != want {
		t.Errorf("zone style %+v, want %+v", got, want)
	}
	if got, want := p.Styles.ForEdge(p.Spec, "async"), (style.Edge{Stroke: "#3C4043", ArrowEnd: "arrow"}); got != want {
		t.Errorf("async style %+v, want %+v", got, want)
	}
	for _, k := range []string{"node", "service", "group"} {
		if st := p.Styles.ForNode(p.Spec, k); st.Shape == "" || st.Fill == "" || st.Stroke == "" {
			t.Errorf("generic pack leaves a property of %q unset: %+v", k, st)
		}
	}
}

package ops

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/spec"
)

const tsPath = "../../web/src/protocol/ops.ts"

// TestTypeScript checks that web/src/protocol/ops.ts, the operations' types for the web app,
// is generated from Defs; -update writes it.
func TestTypeScript(t *testing.T) {
	var b strings.Builder
	b.WriteString("// Code generated from the operation definitions of internal/ops by\n")
	b.WriteString("// `go test ./internal/ops -run TestTypeScript -update`. DO NOT EDIT.\n\n")
	b.WriteString("// Op is an operation (spec FR-12) as the editor sends it.\nexport type Op =")
	for _, d := range Defs {
		fields := []string{fmt.Sprintf("op: %q", d.Name)}
		for _, f := range d.Fields {
			typ := tsType(t, mustJSON(f.Schema))
			if f.Name == "ops" {
				typ = "Op[]"
			}
			opt := "?"
			if f.Required {
				opt = ""
			}
			fields = append(fields, fmt.Sprintf("%s%s: %s", f.Name, opt, typ))
		}
		fmt.Fprintf(&b, "\n  | { %s }", strings.Join(fields, "; "))
	}
	b.WriteString(";\n\n// OpName names an operation.\nexport type OpName = Op[\"op\"];\n\n")
	quoted := make([]string, len(spec.Reserved))
	for i, r := range spec.Reserved {
		quoted[i] = fmt.Sprintf("%q", r)
	}
	fmt.Fprintf(&b, "// RESERVED are the IDs that cannot be simple IDs (spec 6.1).\nexport const RESERVED: readonly string[] = [%s];\n", strings.Join(quoted, ", "))
	got := b.String()
	if *update {
		if err := os.WriteFile(tsPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(tsPath)
	if err != nil || string(want) != got {
		t.Errorf("%s is out of date: go test ./internal/ops -run TestTypeScript -update", tsPath)
	}
}

// tsType is the TypeScript type of the values a JSON schema accepts.
func tsType(t *testing.T, s map[string]any) string {
	if enum, ok := s["enum"].([]any); ok {
		var alts []string
		for _, v := range enum {
			data, _ := json.Marshal(v)
			alts = append(alts, string(data))
		}
		return strings.Join(alts, " | ")
	}
	if alts, ok := s["anyOf"].([]any); ok {
		var out []string
		for _, a := range alts {
			out = append(out, tsType(t, a.(map[string]any)))
		}
		return strings.Join(out, " | ")
	}
	switch typ := s["type"].(type) {
	case []any:
		var out []string
		for _, v := range typ {
			out = append(out, tsType(t, map[string]any{"type": v}))
		}
		return strings.Join(out, " | ")
	case string:
		switch typ {
		case "string":
			return "string"
		case "integer", "number":
			return "number"
		case "boolean":
			return "boolean"
		case "array":
			item := tsType(t, s["items"].(map[string]any))
			if n, ok := s["minItems"].(float64); ok && s["maxItems"] == n {
				return "[" + strings.Join(slices.Repeat([]string{item}, int(n)), ", ") + "]"
			}
			if strings.Contains(item, " ") {
				item = "(" + item + ")"
			}
			return item + "[]"
		case "object":
			props, _ := s["properties"].(map[string]any)
			if len(props) == 0 {
				if extra, ok := s["additionalProperties"].(map[string]any); ok {
					return "Record<string, " + tsType(t, extra) + ">"
				}
				return "Record<string, unknown>"
			}
			required := map[string]bool{}
			list, _ := s["required"].([]any)
			for _, r := range list {
				required[r.(string)] = true
			}
			var fields []string
			for _, k := range slices.Sorted(maps.Keys(props)) {
				opt := "?"
				if required[k] {
					opt = ""
				}
				fields = append(fields, fmt.Sprintf("%s%s: %s", k, opt, tsType(t, props[k].(map[string]any))))
			}
			return "{ " + strings.Join(fields, "; ") + " }"
		}
	}
	t.Fatalf("no TypeScript type for the schema %v", s)
	return ""
}

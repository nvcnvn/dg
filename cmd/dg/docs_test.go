package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/style"
	"github.com/nvcnvn/dg/packs/generic"
)

// repoRoot is the repository root, which GitHub Pages publishes with Jekyll.
const repoRoot = "../.."

// docFiles returns the Markdown files of the documentation, relative to the repository root.
func docFiles(t *testing.T) []string {
	files := []string{"README.md"}
	err := filepath.WalkDir(filepath.Join(repoRoot, "docs"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
			rel, _ := filepath.Rel(repoRoot, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

var sectionRE = regexp.MustCompile(`(?s)(<!-- begin (generated|checked) ([a-z-]+)[^>]*-->\n)(.*?)(<!-- end ([a-z-]+) -->)`)

// TestDocs checks the sections of the documentation that follow the code: a generated section
// is rewritten from the code by -update; a checked section is hand-written and must name what
// the code defines.
func TestDocs(t *testing.T) {
	generators := map[string]func(*testing.T) string{
		"operations":             operationsDoc,
		"operations-toc":         operationsTOC,
		"generic-pack":           genericPackDoc,
		"getting-started-layout": firstDragLayout,
		"dg":                     func(t *testing.T) string { return "```text\n" + help(t, "--help") + "```\n" },
	}
	for _, cmd := range []string{"check", "fmt", "export", "serve", "vocabulary", "mcp", "lsp"} {
		generators["dg-"+cmd] = func(t *testing.T) string { return flagTable(help(t, cmd, "--help")) }
	}
	checks := map[string]func(*testing.T, string){
		"style-properties":         propsCheck(style.NodeProps),
		"edge-style-properties":    propsCheck(style.EdgeProps),
		"overlay-style-properties": propsCheck(style.OverlayProps),
	}
	seen := map[string]bool{}
	for _, file := range docFiles(t) {
		data, err := os.ReadFile(filepath.Join(repoRoot, file))
		if err != nil {
			t.Fatal(err)
		}
		got := sectionRE.ReplaceAllStringFunc(string(data), func(s string) string {
			m := sectionRE.FindStringSubmatch(s)
			head, kind, name, body, end := m[1], m[2], m[3], m[4], m[6]
			if end != name {
				t.Errorf("%s: section %s ends with <!-- end %s -->", file, name, end)
				return s
			}
			seen[name] = true
			if kind == "checked" {
				if check, ok := checks[name]; ok {
					check(t, body)
				} else {
					t.Errorf("%s: no check named %s", file, name)
				}
				return s
			}
			if name == "toc" {
				return head + "\n" + toc(string(data)) + "\n" + m[5]
			}
			gen, ok := generators[name]
			if !ok {
				t.Errorf("%s: no generator named %s", file, name)
				return s
			}
			return head + "\n" + gen(t) + "\n" + m[5]
		})
		if got == string(data) {
			continue
		}
		if *update {
			if err := os.WriteFile(filepath.Join(repoRoot, file), []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("%s is out of date: go test ./cmd/dg -run TestDocs -update", file)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(generators)) {
		if !seen[name] {
			t.Errorf("no documentation section %s", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(checks)) {
		if !seen[name] {
			t.Errorf("no documentation section %s", name)
		}
	}
}

// help returns what dg prints for args.
func help(t *testing.T, args ...string) string {
	var out bytes.Buffer
	if code := run(args, &out, &out); code != 0 {
		t.Fatalf("dg %s: exit code %d: %s", strings.Join(args, " "), code, out.String())
	}
	return out.String()
}

// helpFlag is a flag as a flag set's help text describes it.
type helpFlag struct{ name, typ, usage string }

// helpFlags returns the flags a flag set's help text describes.
func helpFlags(help string) []helpFlag {
	var flags []helpFlag
	lines := strings.Split(help, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "  -") {
			continue
		}
		name, typ, _ := strings.Cut(strings.TrimPrefix(line, "  -"), " ")
		var usage []string
		for _, next := range lines[i+1:] {
			if !strings.HasPrefix(next, "    \t") {
				break
			}
			usage = append(usage, strings.TrimSpace(next))
		}
		flags = append(flags, helpFlag{name, typ, strings.Join(usage, " ")})
	}
	return flags
}

// flagTable is a Markdown table of the flags in a flag set's help text, leaving out skip.
func flagTable(help string, skip ...string) string {
	var b strings.Builder
	b.WriteString("| Flag | Value | Meaning |\n|---|---|---|\n")
	for _, f := range helpFlags(help) {
		if slices.Contains(skip, f.name) {
			continue
		}
		typ := f.typ
		if typ == "value" {
			typ = "string"
		}
		dash := "--"
		if len(f.name) == 1 {
			dash = "-"
		}
		fmt.Fprintf(&b, "| `%s%s` | %s | %s |\n", dash, f.name, typ, cell(f.usage))
	}
	return b.String()
}

// cell escapes text for a Markdown table cell, which GitHub and Jekyll's kramdown both render.
func cell(s string) string {
	return strings.NewReplacer("|", `\|`, "<", "&lt;", ">", "&gt;", "*", `\*`).Replace(s)
}

func operationsDoc(t *testing.T) string {
	var b strings.Builder
	for _, d := range ops.Defs {
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", d.Name, cell(d.Summary))
		b.WriteString(flagTable(help(t, "op", d.Name, "--help"), "C", "json"))
		fmt.Fprintf(&b, "\nAs JSON, with the required fields: `%s`\n", d.Example())
		if d.Name != "batch" {
			fmt.Fprintf(&b, "\nFields (`?` marks the optional ones): `%s`\n", d.Signature())
		}
		b.WriteString("\n")
	}
	return b.String()
}

var tocHeadingRE = regexp.MustCompile(`(?m)^(##|###) (.+)$`)

// toc is a list of links to the second- and third-level headings of a page, which the generated
// operations are not.
func toc(page string) string {
	page, _, _ = strings.Cut(page, "<!-- begin generated operations:")
	var b strings.Builder
	for _, m := range tocHeadingRE.FindAllStringSubmatch(fenceRE.ReplaceAllString(page, ""), -1) {
		indent := ""
		if m[1] == "###" {
			indent = "  "
		}
		fmt.Fprintf(&b, "%s- [%s](#%s)\n", indent, m[2], slug(m[2]))
	}
	return b.String()
}

func operationsTOC(*testing.T) string {
	links := make([]string, len(ops.Defs))
	for i, d := range ops.Defs {
		links[i] = fmt.Sprintf("[%s](#%s)", d.Name, d.Name)
	}
	return strings.Join(links, ", ") + "\n"
}

// firstDragLayout is the layout file the editor writes when Worker is dragged 100 pixels right
// in the hello example: the first drag pins every node where its export draws it, plus the drag.
func firstDragLayout(t *testing.T) string {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(repoRoot, "examples", "hello"))); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "export", "hello.json"))
	if err != nil {
		t.Fatal(err)
	}
	var drawn struct {
		Nodes []struct {
			ID  string `json:"id"`
			Rel [2]int `json:"rel"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &drawn); err != nil {
		t.Fatal(err)
	}
	var batch []ops.Op
	for _, n := range drawn.Nodes {
		x, y := n.Rel[0], n.Rel[1]
		batch = append(batch, ops.Op{Op: "move", View: "main", ID: n.ID, X: &x, Y: &y})
	}
	x, y := *batch[len(batch)-1].X, *batch[len(batch)-1].Y
	if batch[len(batch)-1].ID != "worker" {
		t.Fatal("hello.json no longer lists worker last")
	}
	x += 100
	batch = append(batch, ops.Op{Op: "move", View: "main", ID: "worker", X: &x, Y: &y})
	if _, err := ops.Apply(dir, ops.Op{Op: "batch", Ops: batch}, ops.NewSession()); err != nil {
		t.Fatal(err)
	}
	layout, err := os.ReadFile(filepath.Join(dir, "views", "main.layout.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return "```yaml\n" + string(layout) + "```\n"
}

// hclBlock is a block of a generic pack file, with its attributes as written, in order.
type hclBlock struct {
	typ, label, text string
	attrs            []string
}

func genericBlocks(t *testing.T, name string) []hclBlock {
	src, err := fs.ReadFile(generic.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	f, diags := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	var blocks []hclBlock
	for _, blk := range f.Body.(*hclsyntax.Body).Blocks {
		attrs := slices.SortedFunc(maps.Values(blk.Body.Attributes), func(a, b *hclsyntax.Attribute) int {
			return a.SrcRange.Start.Byte - b.SrcRange.Start.Byte
		})
		var texts []string
		for _, a := range attrs {
			texts = append(texts, fmt.Sprintf("`%s = %s`", a.Name, a.Expr.Range().SliceBytes(src)))
		}
		blocks = append(blocks, hclBlock{blk.Type, blk.Labels[0], string(blk.Range().SliceBytes(src)), texts})
	}
	return blocks
}

func genericPackDoc(t *testing.T) string {
	styles := map[string]hclBlock{}
	var tagStyles, defaults []hclBlock
	for _, blk := range genericBlocks(t, "style.hcl") {
		switch {
		case blk.label == "node" && blk.typ == "style", blk.label == "default", blk.typ == "overlay_style":
			defaults = append(defaults, blk)
		case strings.HasPrefix(blk.label, "tag."):
			tagStyles = append(tagStyles, blk)
		default:
			styles[blk.typ+" "+blk.label] = blk
		}
	}
	var b strings.Builder
	b.WriteString("| Kind | Spec | Style |\n|---|---|---|\n")
	var edgeKinds []string
	for _, blk := range genericBlocks(t, "spec.hcl") {
		if blk.typ == "edge_kind" {
			edgeKinds = append(edgeKinds, blk.label)
			continue
		}
		s := strings.Join(styles["style "+blk.label].attrs, ", ")
		if blk.label == "node" {
			s = "the defaults below"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", blk.label, strings.Join(blk.attrs, ", "), s)
	}
	b.WriteString("\n| Edge kind | Style |\n|---|---|\n")
	for _, k := range edgeKinds {
		s := strings.Join(styles["edge_style "+k].attrs, ", ")
		if k == "default" {
			s = "the defaults below"
		}
		fmt.Fprintf(&b, "| `%s` | %s |\n", k, s)
	}
	b.WriteString("\n| Edge tag | Style |\n|---|---|\n")
	for _, blk := range tagStyles {
		fmt.Fprintf(&b, "| `%s` | %s |\n", strings.TrimPrefix(blk.label, "tag."), strings.Join(blk.attrs, ", "))
	}
	b.WriteString("\nThe defaults, which every other style overrides:\n\n```hcl\n")
	for i, blk := range defaults {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(blk.text + "\n")
	}
	b.WriteString("```\n")
	return b.String()
}

var (
	propRowRE  = regexp.MustCompile("(?m)^\\| (`[^|]*) \\|.*$")
	propNameRE = regexp.MustCompile("`([a-z_]+)`")
)

// propsCheck checks that a table lists every property of props, each with every quoted value
// the property takes, and no other property.
func propsCheck(props map[string]style.Prop) func(*testing.T, string) {
	return func(t *testing.T, body string) {
		rows := map[string]string{}
		for _, m := range propRowRE.FindAllStringSubmatch(body, -1) {
			for _, name := range propNameRE.FindAllStringSubmatch(m[1], -1) {
				rows[name[1]] = m[0]
			}
		}
		for _, name := range slices.Sorted(maps.Keys(props)) {
			row, ok := rows[name]
			if !ok {
				t.Errorf("the documentation does not list the style property %s", name)
				continue
			}
			for _, v := range props[name].Values {
				if strings.HasPrefix(v, `"`) && !strings.Contains(row, strings.Trim(v, `"`)) {
					t.Errorf("the documentation of the style property %s does not name the value %s", name, v)
				}
			}
		}
		for name := range rows {
			if _, ok := props[name]; !ok {
				t.Errorf("the documentation lists %s, which is not a style property", name)
			}
		}
	}
}

var hclFenceRE = regexp.MustCompile("(?s)```hcl\n(.*?)```")

// TestDocExamples checks that the HCL the README, the getting-started guide and the examples
// page show is copied from a file of examples/.
func TestDocExamples(t *testing.T) {
	var sources []string
	err := filepath.WalkDir(filepath.Join(repoRoot, "examples"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".hcl") {
			data, err := os.ReadFile(p)
			sources = append(sources, string(data))
			return err
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"README.md", "docs/getting-started.md", "docs/examples.md"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, file))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range hclFenceRE.FindAllStringSubmatch(string(data), -1) {
			if !slices.ContainsFunc(sources, func(s string) bool { return strings.Contains(s, m[1]) }) {
				t.Errorf("%s shows HCL that no file of examples/ holds:\n%s", file, m[1])
			}
		}
	}
}

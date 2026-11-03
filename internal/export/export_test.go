package export

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	for name, v := range exampleViews(t) {
		t.Run(name, func(t *testing.T) {
			first, err := Render(context.Background(), v)
			if err != nil {
				t.Fatal(err)
			}
			second, err := Render(context.Background(), v)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first.SVG, second.SVG) || !bytes.Equal(first.JSON, second.JSON) {
				t.Fatal("two exports of the same view differ")
			}
			golden(t, filepath.Join("..", "..", "testdata", "export", name+".svg"), first.SVG)
			golden(t, filepath.Join("..", "..", "testdata", "export", name+".json"), first.JSON)
		})
	}
}

// exampleViews resolves every view of every example project, keyed by golden file name: the
// example's name, followed by "-<view>" for views other than main.
func exampleViews(t *testing.T) map[string]*view.Resolved {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "examples", "*"))
	if err != nil {
		t.Fatal(err)
	}
	views := map[string]*view.Resolved{}
	for _, dir := range dirs {
		p, diags, err := project.Load(dir)
		if err != nil || diags.HasErrors() {
			t.Fatalf("load %s: %v %v", dir, err, diags)
		}
		for id := range p.Views {
			v, diags := p.Resolve(id)
			if notYet(t, dir, id, diags) {
				continue
			}
			name := filepath.Base(dir)
			if id != view.DefaultView {
				name += "-" + id
			}
			views[name] = v
		}
	}
	return views
}

// notYet reports whether a view is refused only for features later milestones draw, and fails
// the test for any other error.
func notYet(t *testing.T, dir, id string, diags diag.Diagnostics) bool {
	t.Helper()
	for _, d := range diags {
		if d.Severity == diag.Error && d.Summary != "Not supported yet" {
			t.Fatalf("resolve %s view %s: %v", dir, id, diags)
		}
	}
	return diags.HasErrors()
}

func TestAppIsOffline(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	ctx, stop, err := openApp(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	for _, url := range []string{"http://example.com/", "http://1.1.1.1/", "https://fonts.gstatic.com/"} {
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](
			`fetch("`+url+`").then(() => true, () => false)`, chromedp.EvalAwaitPromise))
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Errorf("the app fetched %s", url)
		}
	}
	if err := chromedp.Do(ctx, chromedp.Navigate("http://example.com/")); err == nil {
		t.Error("navigating to example.com succeeded; host names must not resolve")
	}
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the output:\n%s", path, got)
	}
}

// TestProjectFont exports a project whose font is a copy of the bundled one under another name:
// the drawing has the same geometry and names the project font.
func TestProjectFont(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	font, err := os.ReadFile(filepath.Join("..", "..", "web", "node_modules", "@fontsource", "inter", "files", "inter-latin-400-normal.woff2"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"model.hcl":         "node \"a\" {\n  label = \"Measured with the project font\"\n}\n",
		"dg.hcl":            "project \"p\" {\n  font = \"fonts/Brand.woff2\"\n}\n",
		"fonts/Brand.woff2": string(font),
	}
	for name, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	render := func() *Result {
		p, diags, err := project.Load(dir)
		if err != nil || diags.HasErrors() {
			t.Fatal(err, diags)
		}
		v, diags := p.Resolve(view.DefaultView)
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		res, err := Render(context.Background(), v)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	withFont := render()
	if err := os.Remove(filepath.Join(dir, "dg.hcl")); err != nil {
		t.Fatal(err)
	}
	bundled := render()
	if !bytes.Contains(withFont.SVG, []byte(`font-family="Brand, sans-serif"`)) {
		t.Errorf("the SVG does not name the project font:\n%s", withFont.SVG)
	}
	if !bytes.Equal(withFont.JSON, bundled.JSON) {
		t.Errorf("geometry with the project font differs from the bundled font's:\n%s\n%s", withFont.JSON, bundled.JSON)
	}
}

// TestLayoutRules exports a layout with a container pinned too small for its child (spec 7.2
// rule 5) and a node stored relative to the canvas whose frame is now a container, which is drawn
// where it was stored (spec 7.1).
func TestLayoutRules(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir := t.TempDir()
	files := map[string]string{
		"model.hcl": `node "g" {
  kind = "group"
}

node "a" {
  parent = node.g
}

node "moved" {
  parent = node.g
}
`,
		"views/main.layout.yaml": `version: 1
place:
  a: {in: g, x: 0, y: 0}
  g: {x: 100, y: 100, w: 120, h: 60}
  moved: {x: 300, y: 400}
`,
	}
	for name, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, diags, err := project.Load(dir)
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v, diags := p.Resolve(view.DefaultView)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	res, err := Render(context.Background(), v)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TooSmall) != 1 || res.TooSmall[0] != "g" {
		t.Errorf("too small containers %v, want [g]", res.TooSmall)
	}
	var geo struct {
		Nodes []struct {
			ID   string
			X, Y int
		}
	}
	if err := json.Unmarshal(res.JSON, &geo); err != nil {
		t.Fatal(err)
	}
	for _, n := range geo.Nodes {
		if n.ID == "moved" && (n.X != 300 || n.Y != 400) {
			t.Errorf("node in a stale frame drawn at %d,%d, want 300,400 where it was stored", n.X, n.Y)
		}
	}
}

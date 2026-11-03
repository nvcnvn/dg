package export

import (
	"bytes"
	"context"
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

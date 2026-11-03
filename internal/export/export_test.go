package export

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	for _, name := range []string{"hello"} {
		t.Run(name, func(t *testing.T) {
			p, diags, err := project.Load(filepath.Join("..", "..", "examples", name))
			if err != nil || diags.HasErrors() {
				t.Fatalf("load: %v %v", err, diags)
			}
			v, err := view.Resolve(p, view.DefaultView)
			if err != nil {
				t.Fatal(err)
			}
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

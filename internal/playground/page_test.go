package playground

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

const site = "../../dist/playground"

// tab opens a page of the built site in a new browser, with downloads going to the directory it
// returns. make playground builds the site; without it the test has nothing to open.
func tab(t *testing.T, url func(string) string) (context.Context, string, func(string) string) {
	t.Helper()
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	if _, err := os.Stat(filepath.Join(site, "dg.wasm")); err != nil {
		t.Skip("make playground builds the site")
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(site)))
	t.Cleanup(srv.Close)
	alloc, stopAlloc := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.WindowSize(1800, 1000),
		chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"),
	)...)
	ctx, cancel := chromedp.NewContext(alloc)
	ctx, cancelTime := context.WithTimeout(ctx, 150*time.Second)
	t.Cleanup(func() {
		cancelTime()
		cancel()
		stopAlloc()
	})
	downloads := t.TempDir()
	address := func(path string) string { return srv.URL + "/" + path }
	err := chromedp.Do(ctx,
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := chromedp.CallBrowser(ctx, browser.SetDownloadBehavior, browser.SetDownloadBehaviorParams{Behavior: browser.SetDownloadBehaviorBehaviorAllow, DownloadPath: downloads})
			return err
		}),
		chromedp.Navigate(url(srv.URL+"/")),
		drawn(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, downloads, address
}

// drawn waits for the editor to show a drawing it can edit.
func drawn() chromedp.Action[chromedp.Void] {
	return chromedp.Poll[chromedp.Void](`!document.body.classList.contains('loading') && document.body.classList.contains('editable') && document.querySelector('#paper [data-node]') !== null`, chromedp.WithPollingInterval(10*time.Millisecond))
}

func same(url string) string { return url }

func until(expr string) chromedp.Action[chromedp.Void] {
	return chromedp.Poll[chromedp.Void](expr, chromedp.WithPollingInterval(10*time.Millisecond))
}

// choose picks an item of the toolbar's menu of the button called tool.
func choose(tool, item string) chromedp.Action[chromedp.Void] {
	find := func(sel, text string) string {
		return fmt.Sprintf(`[...document.querySelectorAll(%q)].find((e) => (e.getAttribute('aria-label') || e.textContent).trim() === %q)`, sel, text)
	}
	return chromedp.Steps(
		until(find(`#bar button`, tool)+` !== undefined`),
		chromedp.Evaluate[chromedp.Void](find(`#bar button`, tool)+`.click()`),
		until(find(`#tool-menu [role="menuitem"]`, item)+` !== undefined`),
		chromedp.Evaluate[chromedp.Void](find(`#tool-menu [role="menuitem"]`, item)+`.click()`),
	)
}

func openExample(name string) chromedp.Action[chromedp.Void] {
	return chromedp.Steps(
		chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`{ window.confirm = () => true; const s = document.getElementById('examples'); s.value = %q; s.dispatchEvent(new Event('change')); }`, name)),
		chromedp.Poll[chromedp.Void](`document.querySelector('#model-text .cm-content').textContent.length > 0`),
	)
}

// modelText is what Go last showed the Model editor or the page last saved: the editor itself
// draws only the lines in its window.
const modelText = `(JSON.parse(localStorage.getItem('dg-playground'))['model.hcl'] || '')`

func download(t *testing.T, dir, name string) []byte {
	t.Helper()
	for range 500 {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil && len(data) > 0 {
			return data
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no download %s", name)
	return nil
}

// TestExamplesDraw checks FR-19's acceptance criterion: for every example in the menu, the SVG the
// playground downloads is byte-identical to the dg export golden.
func TestExamplesDraw(t *testing.T) {
	ctx, downloads, _ := tab(t, same)
	for _, name := range []string{"hello", "class", "er"} {
		if err := chromedp.Do(ctx, openExample(name), drawn()); err != nil {
			t.Fatal(err)
		}
		// A snapshot of the new example replaces the last drawing once it is laid out.
		want, err := os.ReadFile(filepath.Join("../../testdata/export", name+".svg"))
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			path := filepath.Join(downloads, "main.svg")
			_ = os.Remove(path)
			if err := chromedp.Do(ctx, choose("Export", "Export SVG")); err != nil {
				t.Fatal(err)
			}
			got := download(t, downloads, "main.svg")
			if bytes.Equal(got, want) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: the SVG differs from the golden:\n%s", name, got)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// center returns the middle of the node's shape on the screen.
func center(ctx context.Context, t *chromedp.Target, node string) (x, y float64, err error) {
	at, err := chromedp.Evaluate[[]float64](fmt.Sprintf(`(() => { const r = document.querySelector('#paper [data-node=%q]').getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; })()`, node))(ctx, t)
	if err != nil {
		return 0, 0, err
	}
	return at[0], at[1], nil
}

// TestBothWays types in the Model editor and sees the canvas change, then relabels a node on the
// canvas, finds the label in the Model editor, and undoes it there.
func TestBothWays(t *testing.T) {
	ctx, _, _ := tab(t, same)
	err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#model-text .cm-content').focus()`),
		chromedp.KeyEvent("node \"typed\" {\n"),
		chromedp.KeyEvent("kind = \"service\"\n}\n"),
		until(`document.querySelector('#paper [data-node="typed"]') !== null`),
		until(`document.querySelector('#model-text .cm-lintRange-error') === null`),
		chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
			x, y, err := center(ctx, tt, "user")
			if err != nil {
				return err
			}
			return chromedp.Do(ctx, chromedp.MouseClickXY(x, y, chromedp.ClickCount(2)))
		}),
		until(`document.activeElement?.classList.contains('dg-inline')`),
		chromedp.KeyEvent("Relabeled"),
		chromedp.KeyEvent("\r"),
		until(modelText+`.includes('Relabeled')`),
		until(`document.body.classList.contains('editable')`),
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#bar button[aria-label="Undo"]').click()`),
		until(`!`+modelText+`.includes('Relabeled')`),
		until(modelText+`.includes('typed')`),
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestDiagnostics types an error in each text editor and sees it there and above the canvas.
func TestDiagnostics(t *testing.T) {
	ctx, _, _ := tab(t, same)
	for _, editor := range []string{"model-text", "style-text"} {
		err := chromedp.Do(ctx,
			chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`document.querySelector('#%s .cm-content').focus()`, editor)),
			chromedp.KeyEvent("nope {\n"),
			until(fmt.Sprintf(`document.querySelector('#%s .cm-lintRange-error') !== null`, editor)),
			until(`/^\d+ error/.test(document.getElementById('diagnostics').textContent)`),
		)
		if err != nil {
			t.Fatal(editor, err)
		}
		for range 7 {
			if err := chromedp.Do(ctx, chromedp.KeyEvent(kb.Backspace)); err != nil {
				t.Fatal(err)
			}
		}
		if err := chromedp.Do(ctx, until(fmt.Sprintf(`document.querySelector('#%s .cm-lintRange-error') === null`, editor))); err != nil {
			t.Fatal(editor, err)
		}
	}
}

// TestShare drags a node, shares, and loads the link: the very files come back and the node is
// where it was dropped. The last project then comes back without a link.
func TestShare(t *testing.T) {
	ctx, _, _ := tab(t, same)
	const stored = `localStorage.getItem('dg-playground')`
	const place = `(() => { const r = document.querySelector('#paper [data-node="user"]').getBoundingClientRect(); const p = document.getElementById('paper').getBoundingClientRect(); return [r.x - p.x, r.y - p.y]; })()`
	var files, link string
	var dropped []float64
	err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#model-text .cm-content').focus()`),
		chromedp.KeyEvent("# Ålpha ✓\n"),
		until(`[...document.querySelectorAll('#model-text .cm-line')].some((l) => l.textContent.includes('Ålpha'))`),
		until(modelText+`.includes('Ålpha')`),
		chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
			x, y, err := center(ctx, tt, "user")
			if err != nil {
				return err
			}
			moves := []chromedp.Action[chromedp.Void]{chromedp.MouseEvent(chromedp.MouseMoved, x, y), chromedp.MouseEvent(chromedp.MousePressed, x, y, chromedp.ButtonLeft, chromedp.ClickCount(1))}
			for i := 1; i <= 10; i++ {
				moves = append(moves, chromedp.MouseEvent(chromedp.MouseMoved, x+float64(i)*12, y+float64(i)*6, chromedp.ButtonLeft))
			}
			return chromedp.Do(ctx, append(moves, chromedp.MouseEvent(chromedp.MouseReleased, x+120, y+60, chromedp.ButtonLeft, chromedp.ClickCount(1)))...)
		}),
		until(`(JSON.parse(`+stored+`)['views/main.layout.yaml'] || '').includes('user')`),
		until(`document.body.classList.contains('editable')`),
		chromedp.Evaluate[chromedp.Void](`document.getElementById('share').click()`),
		until(`document.querySelector('#dialog textarea') !== null`),
		chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
			var err error
			if files, err = chromedp.Evaluate[string](stored)(ctx, tt); err != nil {
				return err
			}
			if dropped, err = chromedp.Evaluate[[]float64](place)(ctx, tt); err != nil {
				return err
			}
			link, err = chromedp.Evaluate[string](`document.querySelector('#dialog textarea').value`)(ctx, tt)
			return err
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(link, "#p=") {
		t.Fatalf("link %q", link)
	}
	var got string
	var loaded []float64
	err = chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`localStorage.clear()`),
		chromedp.Navigate("about:blank"),
		chromedp.Navigate(link),
		drawn(),
		until(modelText+`.includes('Ålpha')`),
		until(`location.hash === ''`),
		chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
			var err error
			if got, err = chromedp.Evaluate[string](stored)(ctx, tt); err != nil {
				return err
			}
			loaded, err = chromedp.Evaluate[[]float64](place)(ctx, tt)
			return err
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != files {
		t.Errorf("the link loaded %s, want %s", got, files)
	}
	if !slices.Equal(loaded, dropped) {
		t.Errorf("the node is at %v, dropped at %v", loaded, dropped)
	}
	if err := chromedp.Do(ctx, chromedp.Reload(), drawn(), until(modelText+`.includes('Ålpha')`)); err != nil {
		t.Fatal(err)
	}
}

func TestDamagedLink(t *testing.T) {
	ctx, _, _ := tab(t, func(url string) string { return url + "#p=AAAA" })
	if err := chromedp.Do(ctx, until(`document.getElementById('message').textContent.includes('damaged')`)); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadProject(t *testing.T) {
	ctx, downloads, _ := tab(t, same)
	if err := chromedp.Do(ctx, choose("Export", "Download project (.zip)")); err != nil {
		t.Fatal(err)
	}
	data := download(t, downloads, "diagram.zip")
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	if want := []string{ModelFile, StyleFile}; !slices.Equal(names, want) {
		t.Errorf("the archive holds %v, want %v", names, want)
	}
}

// TestNoNetwork checks that the page loads everything from the site, and that nothing it does
// meets its content security policy, a PNG export included.
func TestNoNetwork(t *testing.T) {
	ctx, _, _ := tab(t, same)
	err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`window.blocked = []; document.addEventListener('securitypolicyviolation', (e) => window.blocked.push(e.blockedURI + ' ' + e.violatedDirective));`),
		choose("Export", "Export PNG"),
	)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	resources, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](`performance.getEntriesByType('resource').map((r) => r.name).filter((n) => !n.startsWith(location.origin) && !n.startsWith('data:') && !n.startsWith('blob:'))`))
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Errorf("loaded from elsewhere: %v", resources)
	}
	blocked, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](`window.blocked`))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 0 {
		t.Errorf("blocked: %v", blocked)
	}
}

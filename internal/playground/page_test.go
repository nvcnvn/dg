package playground

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

const site = "../../dist/playground"

// wasmDelay holds dg.wasm back for a test of the page before Go starts.
var wasmDelay atomic.Int64

// tab opens a page of the built site in a new browser, with downloads going to the directory it
// returns. make playground builds the site; without it the test has nothing to open. It records
// what the content security policy blocks, from the first script on, in window.blocked.
func tab(t *testing.T, url func(string) string) (context.Context, string, func(string) string) {
	t.Helper()
	ctx, downloads, address := browse(t)
	err := chromedp.Do(ctx, chromedp.Navigate(url(address(""))), drawn())
	if err != nil {
		t.Fatal(err)
	}
	return ctx, downloads, address
}

// browse starts a browser and the site, on no page yet.
func browse(t *testing.T) (context.Context, string, func(string) string) {
	t.Helper()
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	if _, err := os.Stat(filepath.Join(site, "dg.wasm")); err != nil {
		t.Skip("make playground builds the site")
	}
	files := http.FileServer(http.Dir(site))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dg.wasm" {
			time.Sleep(time.Duration(wasmDelay.Load()))
		}
		files.ServeHTTP(w, r)
	}))
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
	err := chromedp.Do(ctx,
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := chromedp.CallBrowser(ctx, browser.SetDownloadBehavior, browser.SetDownloadBehaviorParams{Behavior: browser.SetDownloadBehaviorBehaviorAllow, DownloadPath: downloads})
			return err
		}),
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := cdp.Call(ctx, t, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: `window.blocked = []; document.addEventListener('securitypolicyviolation', (e) => window.blocked.push(e.blockedURI + ' ' + e.violatedDirective));`})
			return err
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, downloads, func(path string) string { return srv.URL + "/" + path }
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
	item := fmt.Sprintf(`[...document.querySelectorAll('#tool-menu [role="menuitem"]')].find((e) => e.textContent.trim() === %q)`, name)
	return chromedp.Steps(
		chromedp.Evaluate[chromedp.Void](`window.confirm = () => true; document.getElementById('examples').click()`),
		until(item+` !== undefined`),
		chromedp.Evaluate[chromedp.Void](item+`.click()`),
		until(`!document.body.classList.contains('stale')`),
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

// userAt is where the node user is on the canvas.
const userAt = `(() => { const r = document.querySelector('#paper [data-node="user"]').getBoundingClientRect(); const p = document.getElementById('paper').getBoundingClientRect(); return [r.x - p.x, r.y - p.y]; })()`

// drag drops node 120 to the right of and 60 below where it was.
func drag(node string) chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
		x, y, err := center(ctx, tt, node)
		if err != nil {
			return err
		}
		moves := []chromedp.Action[chromedp.Void]{chromedp.MouseEvent(chromedp.MouseMoved, x, y), chromedp.MouseEvent(chromedp.MousePressed, x, y, chromedp.ButtonLeft, chromedp.ClickCount(1))}
		for i := 1; i <= 10; i++ {
			moves = append(moves, chromedp.MouseEvent(chromedp.MouseMoved, x+float64(i)*12, y+float64(i)*6, chromedp.ButtonLeft))
		}
		return chromedp.Do(ctx, append(moves, chromedp.MouseEvent(chromedp.MouseReleased, x+120, y+60, chromedp.ButtonLeft, chromedp.ClickCount(1)))...)
	})
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
	var files, link string
	var dropped []float64
	err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#model-text .cm-content').focus()`),
		chromedp.KeyEvent("# Ålpha ✓\n"),
		until(`[...document.querySelectorAll('#model-text .cm-line')].some((l) => l.textContent.includes('Ålpha'))`),
		until(modelText+`.includes('Ålpha')`),
		drag("user"),
		until(`(JSON.parse(`+stored+`)['views/main.layout.yaml'] || '').includes('user')`),
		until(`document.body.classList.contains('editable')`),
		chromedp.Evaluate[chromedp.Void](`document.getElementById('share').click()`),
		until(`document.querySelector('#dialog textarea') !== null`),
		chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
			var err error
			if files, err = chromedp.Evaluate[string](stored)(ctx, tt); err != nil {
				return err
			}
			if dropped, err = chromedp.Evaluate[[]float64](userAt)(ctx, tt); err != nil {
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
			loaded, err = chromedp.Evaluate[[]float64](userAt)(ctx, tt)
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

// TestOpenProject drags a node, downloads the project, opens an example, and gets the drag back
// from the archive. The picker is the browser's own, so the test hands the input its file.
func TestOpenProject(t *testing.T) {
	ctx, downloads, _ := tab(t, same)
	const layout = `(JSON.parse(localStorage.getItem('dg-playground'))['views/main.layout.yaml'] || '')`
	before, err := chromedp.Run(ctx, chromedp.Evaluate[string](layout))
	if err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, drag("user"), until(layout+fmt.Sprintf(` !== %q`, before)), until(`document.body.classList.contains('editable')`), choose("Export", "Download project (.zip)")); err != nil {
		t.Fatal(err)
	}
	archive := base64.StdEncoding.EncodeToString(download(t, downloads, "diagram.zip"))
	dropped, err := chromedp.Run(ctx, chromedp.Evaluate[[]float64](userAt))
	if err != nil {
		t.Fatal(err)
	}
	pick := fmt.Sprintf(`HTMLInputElement.prototype.click = function () {
		const files = new DataTransfer();
		files.items.add(new File([Uint8Array.from(atob(%q), (c) => c.charCodeAt(0))], 'diagram.zip'));
		this.files = files.files;
		this.dispatchEvent(new Event('change'));
	};
	document.getElementById('open').click();`, archive)
	err = chromedp.Do(ctx,
		openExample("class"),
		until(`document.querySelector('#paper [data-node="user"]') === null`),
		chromedp.Evaluate[chromedp.Void](pick),
		until(`document.querySelector('#paper [data-node="user"]') !== null`),
		drawn(),
	)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := chromedp.Run(ctx, chromedp.Evaluate[[]float64](userAt))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(loaded, dropped) {
		t.Errorf("the node is at %v, dropped at %v", loaded, dropped)
	}
}

// TestOverflowOnPhone keeps the toolbar's overflow menu on a short screen, where its last items
// would otherwise be out of reach.
func TestOverflowOnPhone(t *testing.T) {
	ctx, _, _ := tab(t, same)
	err := chromedp.Do(ctx,
		chromedp.EmulateViewport(393, 659),
		until(`!document.getElementById('more').hidden`),
		chromedp.Evaluate[chromedp.Void](`document.getElementById('more').click()`),
		until(`!document.getElementById('overflow-menu').hidden`),
	)
	if err != nil {
		t.Fatal(err)
	}
	bottom, err := chromedp.Run(ctx, chromedp.Evaluate[float64](`innerHeight - document.getElementById('overflow-menu').getBoundingClientRect().bottom`))
	if err != nil {
		t.Fatal(err)
	}
	if bottom < 0 {
		t.Errorf("the menu ends %.0f pixels below the screen", -bottom)
	}
}

// TestNoNetwork checks that the page loads everything from the site, and that nothing it does,
// from its first script to a PNG export, meets its content security policy.
func TestNoNetwork(t *testing.T) {
	ctx, _, _ := tab(t, same)
	if err := chromedp.Do(ctx, choose("Export", "Export PNG")); err != nil {
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

// TestSize checks FR-19's limit on dg.wasm.
func TestSize(t *testing.T) {
	if _, err := os.Stat(filepath.Join(site, "dg.wasm")); err != nil {
		t.Skip("make playground builds the site")
	}
	data, err := os.ReadFile(filepath.Join(site, "dg.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if b.Len() > 3_000_000 {
		t.Errorf("dg.wasm is %d bytes compressed", b.Len())
	}
}

// TestRedraw checks FR-19's limit: for every example, an edit redraws within 300 milliseconds of
// being sent. It takes the fastest of three edits, since a shared CI machine can pause any one.
func TestRedraw(t *testing.T) {
	ctx, _, _ := tab(t, same)
	for _, name := range []string{"hello", "class", "er"} {
		if err := chromedp.Do(ctx, openExample(name), drawn()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
		fastest := math.Inf(1)
		for i := range 3 {
			took, err := chromedp.Run(ctx, chromedp.Evaluate[float64](fmt.Sprintf(`new Promise((resolve) => {
				const text = JSON.parse(localStorage.getItem('dg-playground'))['model.hcl'];
				const start = performance.now();
				new MutationObserver((_, o) => { o.disconnect(); resolve(performance.now() - start); }).observe(document.getElementById('paper'), { subtree: true, childList: true, attributes: true, characterData: true });
				globalThis.dgToGo(JSON.stringify({ type: 'write-file', path: 'model.hcl', text: text + '\nnode "added-%d" {}\n' }));
			})`, i), chromedp.EvalAwaitPromise))
			if err != nil {
				t.Fatal(err)
			}
			fastest = min(fastest, took)
			time.Sleep(200 * time.Millisecond)
		}
		t.Logf("%s: %.0f ms", name, fastest)
		if fastest > 300 {
			t.Errorf("%s: redrew after %.0f ms", name, fastest)
		}
	}
}

// TestUndoInText checks that Ctrl+Z in a text editor undoes the typing, not the canvas, and that a
// new project starts without an undo history.
func TestUndoInText(t *testing.T) {
	ctx, _, _ := tab(t, same)
	first := `document.querySelector('#model-text .cm-line').textContent`
	err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`globalThis.dgToGo(JSON.stringify({ type: 'op', seq: 900, op: { op: 'set_label', id: 'user', label: 'Canvas' } }))`),
		until(modelText+`.includes('Canvas')`),
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#model-text .cm-content').focus()`),
		chromedp.KeyEvent("# typed"),
		until(first+` === '# typednode "user" {'`),
		chromedp.KeyEvent("z", chromedp.KeyModifiers(chromedp.ModifierCtrl)),
		until(first+` === 'node "user" {'`),
		until(modelText+`.includes('Canvas')`),
		openExample("er"),
		drawn(),
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#model-text .cm-content').focus()`),
		chromedp.KeyEvent("z", chromedp.KeyModifiers(chromedp.ModifierCtrl)),
	)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if errors, err := chromedp.Run(ctx, chromedp.Evaluate[int](`document.querySelectorAll('#model-text .cm-lintRange-error').length`)); err != nil || errors != 0 {
		t.Errorf("after undo in a new project: %d errors, %v", errors, err)
	}
}

// TestViewGesturesInPage checks that hiding a node survives a reload and a share link.
func TestViewGesturesInPage(t *testing.T) {
	ctx, _, _ := tab(t, same)
	const stored = `localStorage.getItem('dg-playground')`
	err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#model-text .cm-content').focus()`),
		chromedp.KeyEvent("# mine\n"),
		until(modelText+`.includes('# mine')`),
		chromedp.Evaluate[chromedp.Void](`globalThis.dgToGo(JSON.stringify({ type: 'op', seq: 900, op: { op: 'hide_node', view: 'main', id: 'user' } }))`),
		until(stored+`.includes('views.hcl')`),
		chromedp.Reload(),
		drawn(),
		until(modelText+`.includes('# mine')`),
		until(stored+`.includes('views.hcl')`),
		until(`document.querySelector('#paper [data-node="user"]') === null`),
		chromedp.Evaluate[chromedp.Void](`document.getElementById('share').click()`),
		until(`document.querySelector('#dialog textarea') !== null`),
	)
	if err != nil {
		t.Fatal(err)
	}
	link, err := chromedp.Run(ctx, chromedp.Evaluate[string](`document.querySelector('#dialog textarea').value`))
	if err != nil {
		t.Fatal(err)
	}
	err = chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`localStorage.clear()`),
		chromedp.Navigate("about:blank"),
		chromedp.Navigate(link),
		drawn(),
		until(modelText+`.includes('# mine')`),
		until(`document.getElementById('message').textContent === ''`),
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestLinks checks what a link does to the last project: an empty one is damaged and leaves it, a
// good one asks first, and one that arrives in an open page opens there.
func TestLinks(t *testing.T) {
	ctx, _, address := tab(t, same)
	link := func(text string) string {
		var b bytes.Buffer
		w, err := flate.NewWriter(&b, flate.BestCompression)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(map[string]string{ModelFile: text})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		return "#p=" + base64.RawURLEncoding.EncodeToString(b.Bytes())
	}
	err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#model-text .cm-content').focus()`),
		chromedp.KeyEvent("# mine\n"),
		until(modelText+`.includes('# mine')`),
		chromedp.Navigate("about:blank"),
		chromedp.Navigate(address("#p=qq4FDAA")),
		drawn(),
		until(`document.getElementById('message').textContent.includes('damaged')`),
		until(modelText+`.includes('# mine')`),
		chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
			return chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`window.confirm = () => false; location.hash = %q`, link(testModel))))
		}),
		until(`location.hash === ''`),
		until(modelText+`.includes('# mine')`),
		chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
			return chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`window.confirm = () => true; location.hash = %q`, link(testModel))))
		}),
		until(`!`+modelText+`.includes('# mine') && `+modelText+`.includes('Alpha')`),
		chromedp.Func(func(ctx context.Context, tt *chromedp.Target) error {
			deep := link("node \"a\" {\n  label = " + strings.Repeat("[", 200000) + "\n}\n")
			return chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`location.hash = %q`, deep)))
		}),
		until(`document.querySelector('#model-text .cm-lintRange-error') !== null`),
		openExample("er"),
		drawn(),
		until(`document.getElementById('message').textContent === ''`),
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestLeaveWhileLoading checks that leaving before Go has started keeps the last project.
func TestLeaveWhileLoading(t *testing.T) {
	ctx, _, _ := tab(t, same)
	const stored = `localStorage.getItem('dg-playground')`
	wasmDelay.Store(int64(2 * time.Second))
	t.Cleanup(func() { wasmDelay.Store(0) })
	before, err := chromedp.Run(ctx, chromedp.Evaluate[string](stored))
	if err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, chromedp.Reload(), chromedp.Evaluate[chromedp.Void](`window.dispatchEvent(new Event('pagehide'))`)); err != nil {
		t.Fatal(err)
	}
	after, err := chromedp.Run(ctx, chromedp.Evaluate[string](stored))
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("the last project became %s", after)
	}
	wasmDelay.Store(0)
	if err := chromedp.Do(ctx, drawn()); err != nil {
		t.Fatal(err)
	}
}

// TestDeep checks that Go survives text at the nesting limit in the browser, and says so past it.
func TestDeep(t *testing.T) {
	ctx, _, _ := tab(t, same)
	save := func(text string) chromedp.Action[chromedp.Void] {
		data, err := json.Marshal(map[string]string{"type": "write-file", "path": ModelFile, "text": text})
		if err != nil {
			t.Fatal(err)
		}
		return chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`globalThis.dgToGo(%q)`, data))
	}
	node := func(label string) string { return "node \"a\" {\n  label = " + label + "\n}\n" }
	n := 95
	for name, label := range map[string]string{
		"lists":       strings.Repeat("[", n) + strings.Repeat("]", n),
		"parentheses": strings.Repeat("(", n) + "1" + strings.Repeat(")", n),
		"not":         strings.Repeat("!", n) + "true",
		"sum":         "1" + strings.Repeat("+1", n),
		"conditions":  "true" + strings.Repeat(" ? 1 : true", n/3),
	} {
		err := chromedp.Do(ctx,
			save(node(label)),
			chromedp.Evaluate[chromedp.Void](`globalThis.dgToGo('{"type":"select-view"}')`),
			save(testModel),
			until(`document.querySelector('#paper [data-node="a"]') !== null`),
			until(`document.getElementById('message').textContent === ''`),
		)
		if err != nil {
			t.Fatal(name, err)
		}
	}
	err := chromedp.Do(ctx,
		save(node(strings.Repeat("[", 200000))),
		until(`[...document.querySelectorAll('#diagnostics, #broken')].some((e) => e.textContent.includes('Nested too deeply')) || document.querySelector('#model-text .cm-lintRange-error') !== null`),
		save(testModel),
		until(`document.querySelector('#paper [data-node="a"]') !== null`),
		until(`document.getElementById('message').textContent === ''`),
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestProjectThatStopsGo checks that a project that stopped Go while opening is not opened again.
func TestProjectThatStopsGo(t *testing.T) {
	ctx, _, _ := tab(t, same)
	err := chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`window.addEventListener('pagehide', () => localStorage.setItem('dg-playground-opening', '1'))`),
		chromedp.Reload(),
		drawn(),
		until(`document.getElementById('message').textContent.includes('stopped the playground')`),
		until(`localStorage.getItem('dg-playground-opening') === null`),
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestAccessible checks the page with axe-core.
func TestAccessible(t *testing.T) {
	ctx, _, _ := tab(t, same)
	src, err := os.ReadFile("../../web/node_modules/axe-core/axe.min.js")
	if err != nil {
		t.Fatal(err)
	}
	found, err := chromedp.Run(ctx, chromedp.Evaluate[string](string(src)+`;
		axe.run(document, { resultTypes: ['violations'] }).then((r) => JSON.stringify(r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => v.id)))`, chromedp.EvalAwaitPromise))
	if err != nil {
		t.Fatal(err)
	}
	if found != "[]" {
		t.Errorf("axe-core reports %s", found)
	}
}

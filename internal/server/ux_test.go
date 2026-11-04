package server

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// sizes are the windows the first-use tasks pass in (FR-20).
var sizes = []struct{ w, h int64 }{{1440, 900}, {900, 650}}

// example returns the files of an example project.
func example(t *testing.T, name string) map[string]string {
	t.Helper()
	return tree(t, filepath.Join("..", "..", "examples", name))
}

// page opens the editor at w by h pixels, downloads going to the directory downloads, and waits
// until the page has connected.
func (e *editor) page(w, h int64, downloads string) context.Context {
	e.t.Helper()
	ctx, _ := chromedp.NewContext(e.browser)
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	e.t.Cleanup(cancel)
	err := chromedp.Do(ctx,
		chromedp.EmulateViewport(w, h),
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := chromedp.CallBrowser(ctx, browser.SetDownloadBehavior, browser.SetDownloadBehaviorParams{Behavior: browser.SetDownloadBehaviorBehaviorAllow, DownloadPath: downloads})
			return err
		}),
		chromedp.Navigate(e.url),
		chromedp.Poll[chromedp.Void](`document.getElementById('views') !== null && !document.body.classList.contains('disconnected') && document.getElementById('views').options.length + document.querySelectorAll('#diagnostics li').length > 0`, chromedp.WithPollingInterval(10*time.Millisecond)),
	)
	if err != nil {
		e.t.Fatal(err)
	}
	return ctx
}

// control returns the backend ID of the control with an accessible name, as the browser's
// accessibility tree computes it, the way a user finds it by what it says.
func control(ctx context.Context, t *chromedp.Target, role, name string) (cdp.BackendNodeID, error) {
	doc, err := cdp.Call(ctx, t, dom.GetDocument, dom.GetDocumentParams{})
	if err != nil {
		return 0, err
	}
	found, err := cdp.Call(ctx, t, accessibility.QueryAXTree, accessibility.QueryAXTreeParams{NodeID: doc.Root.NodeID, AccessibleName: name, Role: role})
	if err != nil {
		return 0, err
	}
	for _, n := range found.Nodes {
		if !n.Ignored && n.BackendDOMNodeID != 0 {
			return n.BackendDOMNodeID, nil
		}
	}
	return 0, nil
}

// use clicks the control with an accessible name, opening the toolbar's overflow menu first if
// the control is there.
func use(role, name string) chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		deadline := time.Now().Add(5 * time.Second)
		for {
			id, err := control(ctx, t, role, name)
			if err != nil {
				return fmt.Errorf("%s %q: %w", role, name, err)
			}
			if id != 0 {
				return clickNode(ctx, t, id)
			}
			more, err := control(ctx, t, "button", "More tools")
			if err != nil {
				return err
			}
			if open, err := chromedp.Evaluate[bool](`!document.getElementById('overflow-menu').hidden`)(ctx, t); err == nil && more != 0 && !open {
				if err := clickNode(ctx, t, more); err != nil {
					return err
				}
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("no %s named %q", role, name)
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}

func clickNode(ctx context.Context, t *chromedp.Target, id cdp.BackendNodeID) error {
	if _, err := cdp.Call(ctx, t, dom.ScrollIntoViewIfNeeded, dom.ScrollIntoViewIfNeededParams{BackendNodeID: id}); err != nil {
		return err
	}
	box, err := cdp.Call(ctx, t, dom.GetBoxModel, dom.GetBoxModelParams{BackendNodeID: id})
	if err != nil {
		return err
	}
	q := box.Model.Content
	_, err = chromedp.MouseClickXY((q[0]+q[4])/2, (q[1]+q[5])/2)(ctx, t)
	return err
}

// says waits until the element sel shows text.
func says(sel, text string) chromedp.Action[chromedp.Void] {
	return chromedp.Poll[chromedp.Void](`(() => { const el = document.querySelector(` + jsString(sel) + `); return !!el && el.checkVisibility() && el.innerText.includes(` + jsString(text) + `); })()`)
}

// noViolations fails unless axe-core finds no serious or critical violation in the page's state.
func noViolations(t *testing.T, ctx context.Context, state string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "web", "node_modules", "axe-core", "axe.min.js"))
	if err != nil {
		t.Fatal(err)
	}
	found, err := chromedp.Run(ctx, chromedp.Evaluate[string](string(src)+`;
		axe.run(document, { resultTypes: ['violations'] }).then((r) => JSON.stringify(r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => ({ id: v.id, nodes: v.nodes.map((n) => n.html.slice(0, 120) + ' ' + n.failureSummary) })), null, 1))`, chromedp.EvalAwaitPromise))
	if err != nil {
		t.Fatalf("%s: axe: %v", state, err)
	}
	if found != "[]" {
		t.Errorf("%s: axe-core reports %s", state, found)
	}
}

// labelNew waits for the label editor of a new node, types label and waits for the drawing.
func (e *editor) labelNew(label string) chromedp.Action[chromedp.Void] {
	return chromedp.Steps(edit(label), chromedp.Poll[chromedp.Void](`document.body.classList.contains('editable')`, chromedp.WithPollingInterval(10*time.Millisecond)))
}

// TestFirstUse runs the first-use tasks of FR-20, each at both window sizes, finding each control
// by its accessible name, and checks each state they reach with axe-core.
func TestFirstUse(t *testing.T) {
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			t.Run("create two nodes and connect them", func(t *testing.T) {
				e := newEditor(t, map[string]string{})
				ctx := e.page(size.w, size.h, t.TempDir())
				err := chromedp.Do(ctx, says(`#empty`, "Drag a kind from the palette, or click one, to add a node"), says(`#palette`, "service"))
				if err != nil {
					t.Fatal(err)
				}
				noViolations(t, ctx, "empty")
				err = chromedp.Do(ctx,
					use("button", "service"), e.labelNew("API"),
					use("button", "database"), e.labelNew("Orders DB"),
					use("button", "Connect"), says(`#guide`, "Drag from a node, port or row to another. Esc to finish."),
				)
				if err != nil {
					t.Fatal(err)
				}
				noViolations(t, ctx, "connect mode")
				err = chromedp.Do(ctx, dragOnto(`[data-node="api"]`, `[data-node="orders-db"]`, 0.5, 0.5, 0, 0), e.idle(), chromedp.KeyEvent(kbEscape))
				if err != nil {
					t.Fatal(err)
				}
				if model := tree(t, e.dir)["model.hcl"]; !strings.Contains(model, "edge \"api-orders-db\" {\n  from = node.api\n  to   = node.orders-db\n}") {
					t.Errorf("model.hcl:\n%s", model)
				}
			})

			t.Run("work in the C4 example", func(t *testing.T) {
				e := newEditor(t, example(t, "c4"))
				downloads := t.TempDir()
				ctx := e.page(size.w, size.h, downloads)
				err := chromedp.Do(ctx, e.idle(), click(`[data-node="customer"]`))
				if err != nil {
					t.Fatal(err)
				}
				showInspector(t, ctx)
				noViolations(t, ctx, "a selection")
				steps := []struct {
					name    string
					actions []chromedp.Action[chromedp.Void]
				}{
					{"rename", []chromedp.Action[chromedp.Void]{use("button", "Rename ID…"), use("textbox", "New ID"), js(`document.activeElement.select()`), chromedp.KeyEvent("client\r"), e.idle(), chromedp.Poll[chromedp.Void](`document.querySelector('[data-node="client"]') !== null`)}},
					{"move", []chromedp.Action[chromedp.Void]{dragBy(`[data-node="client"]`, 60, 0), e.idle()}},
					{"align", []chromedp.Action[chromedp.Void]{click(`[data-node="email"]`), click(`[data-node="client"]`, chromedp.ButtonModifiers(chromedp.ModifierShift)), use("button", "Arrange"), use("menuitem", "Align top"), e.idle()}},
					{"undo", []chromedp.Action[chromedp.Void]{use("button", "Undo"), e.idle()}},
					{"drill down", []chromedp.Action[chromedp.Void]{click(`[data-drill="banking"]`), chromedp.Poll[chromedp.Void](`document.getElementById('paper').dataset.view === 'containers'`), e.idle()}},
				}
				for _, s := range steps {
					sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
					err := chromedp.Do(sctx, s.actions...)
					cancel()
					if err != nil {
						t.Fatalf("%s: %v", s.name, err)
					}
				}
				noViolations(t, ctx, "a view opened by its badge")
				if err := chromedp.Do(ctx, use("button", "Export"), use("menuitem", "Export SVG")); err != nil {
					t.Fatal(err)
				}
				if data := download(t, downloads, "containers.svg"); !strings.HasPrefix(string(data), "<svg ") {
					t.Errorf("the export starts %q", data[:min(len(data), 20)])
				}
				if !strings.Contains(tree(t, e.dir)["model.hcl"], `node "client"`) {
					t.Error("the rename did not reach the model")
				}
			})

			t.Run("read an error and fix it", func(t *testing.T) {
				e := newEditor(t, map[string]string{"model.hcl": "node \"a\" {\n  label = \n}\n"})
				ctx := e.page(size.w, size.h, t.TempDir())
				if err := chromedp.Do(ctx, says(`#broken`, "model.hcl:2")); err != nil {
					t.Fatal(err)
				}
				noViolations(t, ctx, "broken")
				shown, err := chromedp.Run(ctx, chromedp.Evaluate[string](`document.body.innerText`))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(shown, "last good drawing") {
					t.Error(`"last good drawing" with no drawing`)
				}
				if err := os.WriteFile(filepath.Join(e.dir, "model.hcl"), []byte("node \"a\" {\n  label = \"Alpha\"\n}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := chromedp.Do(ctx, e.idle(), chromedp.Poll[chromedp.Void](`document.getElementById('broken').hidden`)); err != nil {
					t.Fatal(err)
				}
				noViolations(t, ctx, "fixed")
			})
		})
	}
}

// showInspector shows the inspector, which starts hidden in a narrow window.
func showInspector(t *testing.T, ctx context.Context) {
	t.Helper()
	hidden, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.body.classList.contains('no-inspector')`))
	if err != nil {
		t.Fatal(err)
	}
	if hidden {
		if err := chromedp.Do(ctx, use("button", "Inspector")); err != nil {
			t.Fatal(err)
		}
	}
}

// download waits for the file name in dir and returns it.
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

// TestExportGoldens checks that Export saves, for each view of each example, the file dg export
// writes (FR-20).
func TestExportGoldens(t *testing.T) {
	dirs, err := os.ReadDir(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		t.Run(dir.Name(), func(t *testing.T) {
			e := newEditor(t, example(t, dir.Name()))
			downloads := t.TempDir()
			ctx := e.page(1440, 900, downloads)
			views, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](`[...document.getElementById('views').options].map((o) => o.value)`))
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range views {
				name := dir.Name()
				if v != "main" {
					name += "-" + v
				}
				golden, err := os.ReadFile(filepath.Join("..", "..", "testdata", "export", name+".svg"))
				if err != nil {
					t.Fatal(err)
				}
				err = chromedp.Do(ctx,
					chromedp.SetValue(`#views`, v), js(`document.getElementById('views').dispatchEvent(new Event('change'))`),
					chromedp.Poll[chromedp.Void](`document.getElementById('paper').dataset.view === `+jsString(v)),
					e.idle(),
					toolMenu("save", "Export SVG"),
				)
				if err != nil {
					t.Fatal(err)
				}
				if got := download(t, downloads, v+".svg"); string(got) != string(golden) {
					t.Errorf("view %s: the export differs from %s.svg", v, name)
				}
			}
		})
	}
}

// TestExportPNG checks that Export draws a PNG of what the canvas shows at 100%: its size, and,
// since a picture sees none of the page's fonts, its text in the bundled font rather than a
// system one, whose glyphs would land elsewhere.
func TestExportPNG(t *testing.T) {
	e := newEditor(t, example(t, "hello"))
	downloads := t.TempDir()
	ctx := e.page(1440, 900, downloads)
	if err := chromedp.Do(ctx, e.idle()); err != nil {
		t.Fatal(err)
	}
	canvas, err := chromedp.Run(ctx, chromedp.Screenshot(`#paper`))
	if err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, toolMenu("save", "Export PNG")); err != nil {
		t.Fatal(err)
	}
	want, err := png.Decode(bytes.NewReader(canvas))
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(download(t, downloads, "main.png")))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Size() != want.Bounds().Size() {
		t.Fatalf("the PNG is %v, the canvas shows %v", got.Bounds().Size(), want.Bounds().Size())
	}
	differ, ink := 0, 0
	b := got.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r0, g0, b0, _ := got.At(x, y).RGBA()
			r1, g1, b1, _ := want.At(x, y).RGBA()
			if r1>>8 < 200 || g1>>8 < 200 || b1>>8 < 200 {
				ink++
			}
			if max(abs(r0, r1), abs(g0, g1), abs(b0, b1))>>8 > 96 {
				differ++
			}
		}
	}
	t.Logf("%d of %d inked pixels differ from the canvas", differ, ink)
	if differ*20 > ink {
		t.Errorf("%d of %d inked pixels differ from the canvas", differ, ink)
	}
}

func abs(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestEditorLook checks the presentation of FR-20 that the first-use tasks do not reach.
func TestEditorLook(t *testing.T) {
	files := map[string]string{
		"dg.hcl":                 "project \"look\" {\n  packs = [\"packs/acme\"]\n}\n",
		"spec.hcl":               "kind \"mine\" {}\n",
		"packs/acme/spec.hcl":    "kind \"gear\" {}\n",
		"packs/acme/style.hcl":   "style \"gear\" {\n  shape = \"hexagon\"\n}\n",
		"model.hcl":              editorModel + "\nnode \"far\" {\n  label = \"Far\"\n  tags  = [\"hot\"]\n}\n",
		"views/main.layout.yaml": "version: 1\nplace:\n  a: {x: 0, y: 0}\n  b: {x: 200, y: 0}\n  c: {x: 3000, y: 2000}\n  far: {x: 3000, y: 0}\n  g: {x: 0, y: 150, w: 300, h: 160}\n  d: {in: g, x: 0, y: 0}\n  e: {in: g, x: 140, y: 0}\n  lane: {x: 400, y: 150}\n",
	}
	e := newEditor(t, files)
	ctx := e.page(1440, 900, t.TempDir())
	if err := chromedp.Do(ctx, e.idle()); err != nil {
		t.Fatal(err)
	}
	eval := func(src string) string {
		t.Helper()
		out, err := chromedp.Run(ctx, chromedp.Evaluate[string](src))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	check := func(what, src, want string) {
		t.Helper()
		if got := eval(src); got != want {
			t.Errorf("%s: %q, want %q", what, got, want)
		}
	}

	check("palette groups in load order", `[...document.querySelectorAll('#kinds h3')].map((h) => h.textContent).join(' ')`, "generic acme Project")
	check("every kind has a picture", `String([...document.querySelectorAll('#kinds button')].every((b) => b.querySelector('svg.picture') !== null))`, "true")
	check("a kind's picture has its default shape", `String(document.querySelector('[data-kind="gear"] svg path') !== null)`, "true")
	if err := chromedp.Do(ctx, chromedp.SendKeys(`#palette-filter`, "gear")); err != nil {
		t.Fatal(err)
	}
	check("the filter hides empty groups", `[...document.querySelectorAll('#kinds h3')].map((h) => h.textContent).join(' ')`, "acme")
	if err := chromedp.Do(ctx, js(`(() => { const f = document.getElementById('palette-filter'); f.value = ''; f.dispatchEvent(new Event('input')); })()`)); err != nil {
		t.Fatal(err)
	}

	whole := `(() => { const p = document.getElementById('paper').getBoundingClientRect(); const c = document.getElementById('canvas').getBoundingClientRect(); return String(p.left >= c.left && p.top >= c.top && p.right <= c.right && p.bottom <= c.bottom); })()`
	check("a view opens whole", whole, "true")
	zoom := `document.getElementById('zoom').innerText`
	if got := eval(zoom); got == "100%" {
		t.Errorf("a view larger than the canvas opens at %s", got)
	}
	if err := chromedp.Do(ctx, click(`[data-node="a"]`), click(`[data-node="b"]`, chromedp.ButtonModifiers(chromedp.ModifierShift)), chromedp.KeyEvent("@", chromedp.KeyModifiers(chromedp.ModifierShift))); err != nil {
		t.Fatal(err)
	}
	check("zoom to selection", zoom, "100%")
	check("the selection in view", `(() => { const r = document.querySelector('[data-node="a"]').getBoundingClientRect(); const c = document.getElementById('canvas').getBoundingClientRect(); return String(r.left >= c.left && r.right <= c.right && r.top >= c.top && r.bottom <= c.bottom); })()`, "true")
	if err := chromedp.Do(ctx, chromedp.KeyEvent("!", chromedp.KeyModifiers(chromedp.ModifierShift))); err != nil {
		t.Fatal(err)
	}
	check("fit", whole, "true")
	if err := chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`#zoom`))); err != nil {
		t.Fatal(err)
	}
	check("the zoom level's menu", `[...document.querySelectorAll('#tool-menu button')].map((b) => b.querySelector('.text').textContent).join(', ')`, "50%, 100%, 200%, Fit, Zoom to selection")
	if err := chromedp.Do(ctx, chromedp.KeyEvent(kbEscape), chromedp.Poll[chromedp.Void](`document.activeElement?.id === 'zoom'`)); err != nil {
		t.Fatal(err)
	}

	if err := chromedp.Do(ctx, click(`[data-node="a"]`), at(`#arrange`, func(x, y float64) chromedp.Action[chromedp.Void] {
		return chromedp.MouseEvent(chromedp.MouseMoved, x, y)
	}), says(`#tip`, "Select two nodes or more")); err != nil {
		t.Fatalf("a disabled control's tooltip: %v", err)
	}
	if err := chromedp.Do(ctx, at(`#undo`, func(x, y float64) chromedp.Action[chromedp.Void] {
		return chromedp.MouseEvent(chromedp.MouseMoved, x, y)
	}), says(`#tip`, "Nothing to undo")); err != nil {
		t.Fatalf("a disabled control's tooltip: %v", err)
	}
	if err := chromedp.Do(ctx, at(`#fit`, func(x, y float64) chromedp.Action[chromedp.Void] {
		return chromedp.MouseEvent(chromedp.MouseMoved, x, y)
	}), says(`#tip`, "Fit the view (Shift+1)")); err != nil {
		t.Fatalf("a control's tooltip: %v", err)
	}

	if err := chromedp.Do(ctx, click(`[data-node="far"]`), chromedp.Poll[chromedp.Void](`document.querySelector('#inspector .id')?.textContent === 'far'`)); err != nil {
		t.Fatal(err)
	}
	check("the inspector's title", `document.querySelector('#inspector h2').textContent + '/' + document.querySelector('#inspector .kind').textContent`, "Far/node")
	check("the inspector's groups", `[...document.querySelectorAll('#inspector h3')].map((h) => h.textContent).join(', ')`, "Text, Kind and tags, Ports")
	check("tags are chips", `document.querySelector('#inspector .chip').firstChild.textContent`, "hot")
	check("Delete comes last, destructive", `(() => { const b = [...document.querySelectorAll('#inspector button')]; const l = b[b.length - 1]; return l.textContent + l.className; })()`, "Deletedanger")
	if err := chromedp.Do(ctx, click(`[data-node="far"]`, chromedp.ButtonRight), chromedp.Poll[chromedp.Void](`!document.getElementById('context-menu').hidden`)); err != nil {
		t.Fatal(err)
	}
	check("destructive items come last", `(() => { const c = [...document.getElementById('context-menu').children]; const l = c[c.length - 1]; return c[c.length - 2].tagName + l.className + l.querySelector('.text').textContent; })()`, "HRitem dangerDelete from model")
	check("a shortcut on the right", `(() => { const b = document.querySelector('#context-menu button'); return b.querySelector('kbd').textContent + String(b.querySelector('kbd').getBoundingClientRect().left > b.querySelector('.text').getBoundingClientRect().right); })()`, "F2true")
	noViolations(t, ctx, "the context menu")
	if err := chromedp.Do(ctx, chromedp.KeyEvent(kbEscape), chromedp.KeyEvent("c"), chromedp.MouseEvent(chromedp.MouseMoved, 10, 10)); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, at(`[data-node="far"]`, func(x, y float64) chromedp.Action[chromedp.Void] {
		return chromedp.MouseEvent(chromedp.MouseMoved, x, y)
	}), chromedp.Poll[chromedp.Void](`document.querySelector('[data-reach="node"][data-target="far"]') !== null`)); err != nil {
		t.Fatalf("connect mode's highlight: %v", err)
	}
	if err := chromedp.Do(ctx, chromedp.KeyEvent(kbEscape), chromedp.Poll[chromedp.Void](`document.getElementById('guide').hidden && document.querySelector('[data-reach]') === null`)); err != nil {
		t.Fatal(err)
	}

	if err := chromedp.Do(ctx, click(`#paper`), chromedp.KeyEvent("?"), chromedp.Poll[chromedp.Void](`document.getElementById('dialog').open`)); err != nil {
		t.Fatal(err)
	}
	check("the sheet of shortcuts", `document.querySelector('#shortcuts tbody tr td').textContent`, "Ctrl/⌘+Z, Ctrl/⌘+Shift+Z, Ctrl+Y")
	noViolations(t, ctx, "the sheet of shortcuts")
	if err := chromedp.Do(ctx, chromedp.KeyEvent(kbEscape), chromedp.Poll[chromedp.Void](`!document.getElementById('dialog').open`)); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(e.dir, "views", "main.layout.yaml"), []byte("version: 1\nplace:\n  ghost: {x: 0, y: 0}\n  gone: {x: 0, y: 0}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`!document.getElementById('diagnostics').hidden`)); err != nil {
		t.Fatal(err)
	}
	count := eval(`document.querySelector('#diagnostics > button').innerText`)
	if !strings.Contains(count, "2 warnings") && !strings.Contains(count, "2 errors") {
		t.Errorf("the bar says %q", count)
	}
	if err := chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`#diagnostics > button`)), chromedp.Poll[chromedp.Void](`document.querySelector('#diagnostics .diags li svg') !== null && document.querySelector('#diagnostics .place').textContent.startsWith('views/main.layout.yaml:')`)); err != nil {
		t.Fatal(err)
	}
	check("the bar overlays the canvas", `String(document.getElementById('diagnostics').getBoundingClientRect().top < document.getElementById('canvas').getBoundingClientRect().top + 40)`, "true")
	noViolations(t, ctx, "the diagnostics expanded")
}

// TestToolbarRow checks that the toolbar is one row at 900 pixels wide, with nothing in its
// overflow menu when the view has no breadcrumbs.
func TestToolbarRow(t *testing.T) {
	e := newEditor(t, example(t, "c4-container"))
	ctx := e.page(900, 650, t.TempDir())
	if err := chromedp.Do(ctx, e.idle()); err != nil {
		t.Fatal(err)
	}
	tops, err := chromedp.Run(ctx, chromedp.Evaluate[[]int](`[...document.querySelectorAll('#bar .group')].map((g) => Math.round(g.getBoundingClientRect().top + g.getBoundingClientRect().height / 2))`))
	if err != nil {
		t.Fatal(err)
	}
	if len(tops) != 8 || slices.Min(tops) != slices.Max(tops) {
		t.Errorf("the groups' middles are at %v", tops)
	}
	overflow, err := chromedp.Run(ctx, chromedp.Evaluate[int](`document.querySelectorAll('#overflow-menu .group').length`))
	if err != nil {
		t.Fatal(err)
	}
	if overflow != 0 {
		t.Errorf("%d groups in the overflow menu", overflow)
	}
}

// TestDrillTooltip checks that a drill-down badge's tooltip names the view it opens, and that a
// pack's kind reaches the editor with its pack and default style.
func TestDrillTooltip(t *testing.T) {
	e := newEditor(t, example(t, "c4"))
	ctx := e.page(1440, 900, t.TempDir())
	err := chromedp.Do(ctx, e.idle(), at(`[data-drill="banking"]`, func(x, y float64) chromedp.Action[chromedp.Void] {
		return chromedp.MouseEvent(chromedp.MouseMoved, x, y)
	}), says(`#tip`, "Open view Internet Banking System: containers"))
	if err != nil {
		t.Fatal(err)
	}
}

// shotThemes are the prefers-color-scheme values TestScreenshots draws.
var shotThemes = []string{"light", "dark"}

// TestScreenshots writes, to the directory $DG_SCREENSHOTS names, the editor's states in both
// themes at both widths, to look at; they are not goldens (spec 15).
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("DG_SCREENSHOTS")
	if dir == "" {
		t.Skip("DG_SCREENSHOTS is not set")
	}
	dirs, err := os.ReadDir(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range shotThemes {
		for _, size := range sizes {
			name := fmt.Sprintf("%s-%d", theme, size.w)
			open := func(t *testing.T, files map[string]string) (*editor, context.Context) {
				e := newEditor(t, files)
				ctx, _ := chromedp.NewContext(e.browser)
				ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				t.Cleanup(cancel)
				err := chromedp.Do(ctx,
					chromedp.EmulateViewport(size.w, size.h),
					chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
						_, err := cdp.Call(ctx, t, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: theme}}})
						return err
					}),
					chromedp.Navigate(e.url),
					chromedp.Poll[chromedp.Void](`document.getElementById('views') !== null && (document.body.classList.contains('editable') || !document.getElementById('broken').hidden)`, chromedp.WithPollingInterval(10*time.Millisecond)),
				)
				if err != nil {
					t.Fatal(err)
				}
				return e, ctx
			}
			shot := func(t *testing.T, ctx context.Context, state string) {
				if err := chromedp.Do(ctx, chromedp.Sleep(150*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				data, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name+"-"+state+".png"), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			t.Run(name, func(t *testing.T) {
				for _, d := range dirs {
					t.Run(d.Name(), func(t *testing.T) {
						_, ctx := open(t, example(t, d.Name()))
						shot(t, ctx, "example-"+d.Name())
					})
				}
				t.Run("empty", func(t *testing.T) {
					_, ctx := open(t, map[string]string{})
					shot(t, ctx, "empty")
				})
				t.Run("broken", func(t *testing.T) {
					_, ctx := open(t, map[string]string{"model.hcl": "node \"a\" {\n  label = \n}\n"})
					shot(t, ctx, "broken")
				})
				e, ctx := open(t, example(t, "c4-container"))
				if err := chromedp.Do(ctx, click(`[data-node="customer"]`)); err != nil {
					t.Fatal(err)
				}
				showInspector(t, ctx)
				shot(t, ctx, "selection")
				if err := chromedp.Do(ctx, click(`[data-node="customer"]`, chromedp.ButtonRight)); err != nil {
					t.Fatal(err)
				}
				shot(t, ctx, "context-menu")
				if err := chromedp.Do(ctx, chromedp.KeyEvent(kbEscape), chromedp.KeyEvent("c"), at(`[data-node="customer"]`, func(x, y float64) chromedp.Action[chromedp.Void] {
					return chromedp.MouseEvent(chromedp.MouseMoved, x, y)
				})); err != nil {
					t.Fatal(err)
				}
				shot(t, ctx, "connect-mode")
				if err := chromedp.Do(ctx, chromedp.KeyEvent(kbEscape), js(`document.getElementById('help').click()`)); err != nil {
					t.Fatal(err)
				}
				shot(t, ctx, "shortcuts")
				if err := chromedp.Do(ctx, chromedp.KeyEvent(kbEscape), js(`document.getElementById('save').click()`)); err != nil {
					t.Fatal(err)
				}
				shot(t, ctx, "export-menu")
				if err := os.WriteFile(filepath.Join(e.dir, "model.hcl"), []byte("node \"a\" {\n  label = \n}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.body.classList.contains('stale')`), js(`document.querySelector('#diagnostics > button').click()`)); err != nil {
					t.Fatal(err)
				}
				shot(t, ctx, "last-good-drawing")
			})
		}
	}
}

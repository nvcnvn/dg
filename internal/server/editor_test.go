package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// editorModel is a project for the editor tests: three nodes on the canvas, a container holding
// two, a grid holding three, and edges between them.
const editorModel = `node "a" {
  label = "Alpha"
}

node "b" {
  label = "Beta"
}

node "c" {
  label = "Gamma"

  port "in" {
    side = "top"
  }
}

node "g" {
  kind  = "group"
  label = "Group"
}

node "d" {
  label  = "Delta"
  parent = node.g
}

node "e" {
  label  = "Epsilon"
  parent = node.g
}

node "lane" {
  kind  = "grid-group"
  label = "Lane"
}

node "s1" {
  label  = "One"
  parent = node.lane
}

node "s2" {
  label  = "Two"
  parent = node.lane
}

node "s3" {
  label  = "Three"
  parent = node.lane
}

edge "ab" {
  from  = node.a
  to    = node.b
  label = "calls"
}

edge "bc" {
  from = node.b
  to   = node.c
}

edge "ad" {
  from = node.a
  to   = node.d
}
`

// editor serves a project in a Chromium window, where tab opens the editor.
type editor struct {
	t       *testing.T
	dir     string
	url     string
	s       *Server
	browser context.Context
}

// newEditor serves a project of files, by slash path, and starts Chromium.
func newEditor(t *testing.T, files map[string]string) *editor {
	t.Helper()
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := New(dir)
	srv := httptest.NewServer(s.Handler())
	watch, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Watch(watch, func(err error) { t.Error(err) }) }()
	alloc, stopAlloc := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.WindowSize(1600, 1000))...)
	browser, cancelBrowser := chromedp.NewContext(alloc)
	cancel := func() {
		cancelBrowser()
		stopAlloc()
	}
	if err := chromedp.Do(browser); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
		srv.Close()
	})
	return &editor{t: t, dir: dir, url: srv.URL, s: s, browser: browser}
}

// tab opens the editor in a new tab of a 1600 by 1000 window and waits for a drawing.
// It records every request the page sends in window.sent.
func (e *editor) tab() context.Context {
	e.t.Helper()
	ctx, _ := chromedp.NewContext(e.browser)
	err := chromedp.Do(ctx,
		chromedp.Navigate(e.url),
		chromedp.Evaluate[chromedp.Void](`window.sent = []; { const send = WebSocket.prototype.send; WebSocket.prototype.send = function (d) { window.sent.push(JSON.parse(d)); return send.call(this, d); }; }`),
		e.idle(),
	)
	if err != nil {
		e.t.Fatal(err)
	}
	return ctx
}

// idle waits until the page shows a drawing and can edit it.
func (e *editor) idle() chromedp.Action[chromedp.Void] {
	return chromedp.Poll[chromedp.Void](`document.body.classList.contains('editable') && document.querySelector('[data-node]') !== null`, chromedp.WithPollingInterval(10*time.Millisecond))
}

// drag presses the left button at (x, y), moves the pointer by (dx, dy) in steps of a frame
// and releases it.
func drag(x, y, dx, dy float64, steps int) chromedp.Action[chromedp.Void] {
	actions := []chromedp.Action[chromedp.Void]{chromedp.MouseEvent(chromedp.MouseMoved, x, y), chromedp.MouseEvent(chromedp.MousePressed, x, y, chromedp.ButtonLeft, chromedp.ClickCount(1))}
	for i := 1; i <= steps; i++ {
		f := float64(i) / float64(steps)
		actions = append(actions, chromedp.MouseEvent(chromedp.MouseMoved, x+dx*f, y+dy*f, chromedp.ButtonLeft), chromedp.Sleep(16*time.Millisecond))
	}
	return chromedp.Steps(append(actions, chromedp.MouseEvent(chromedp.MouseReleased, x+dx, y+dy, chromedp.ButtonLeft, chromedp.ClickCount(1)))...)
}

// sent returns the requests the page sent other than select-view, and forgets them.
func sent(ctx context.Context, t *testing.T) []map[string]any {
	t.Helper()
	out, err := chromedp.Run(ctx, chromedp.Evaluate[[]map[string]any](`(() => { const s = window.sent.filter((m) => m.type !== 'select-view'); window.sent = []; return s; })()`))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// shot saves a screenshot of the tab ctx to the directory $DG_SCREENSHOTS names, if any, to look
// at.
func (e *editor) shot(ctx context.Context, name string) {
	dir := os.Getenv("DG_SCREENSHOTS")
	if dir == "" {
		return
	}
	png, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
	if err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".png"), png, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// at returns an action that runs f with the page coordinates of the center of the drawn
// element sel, or of the point halfway along it for an edge's path.
func at(sel string, f func(x, y float64) chromedp.Action[chromedp.Void]) chromedp.Action[chromedp.Void] {
	return func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
		r, err := chromedp.Evaluate[[]float64](`(() => {
			const el = document.querySelector(`+jsString(sel)+`);
			if (el instanceof SVGPathElement) {
				const p = el.getPointAtLength(el.getTotalLength() / 2);
				const q = new DOMPoint(p.x, p.y).matrixTransform(el.getScreenCTM());
				return [q.x, q.y];
			}
			const r = el.getBoundingClientRect();
			return [r.x + r.width / 2, r.y + r.height / 2];
		})()`)(ctx, t)
		if err != nil {
			return chromedp.Void{}, fmt.Errorf("%s: %w", sel, err)
		}
		return f(r[0], r[1])(ctx, t)
	}
}

func click(sel string, opts ...chromedp.MouseOption) chromedp.Action[chromedp.Void] {
	return at(sel, func(x, y float64) chromedp.Action[chromedp.Void] { return chromedp.MouseClickXY(x, y, opts...) })
}

func dragBy(sel string, dx, dy float64) chromedp.Action[chromedp.Void] {
	return at(sel, func(x, y float64) chromedp.Action[chromedp.Void] { return drag(x, y, dx, dy, 8) })
}

// arrange chooses an action of the toolbar's arrange menu.
func arrange(value string) chromedp.Action[chromedp.Void] {
	return chromedp.Evaluate[chromedp.Void](`(() => { const s = document.getElementById('arrange'); s.value = ` + jsString(value) + `; s.dispatchEvent(new Event('change')); })()`)
}

// TestEditorGestures drives the layout gestures in Chromium (FR-11): each sends one operation or
// batch on drop, which is one undo step: undo restores the files byte for byte, and redo the
// files the gesture wrote.
func TestEditorGestures(t *testing.T) {
	e := newEditor(t, map[string]string{"model.hcl": editorModel})
	ctx := e.tab()
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := chromedp.KeyModifiers(chromedp.ModifierCtrl)
	for i, g := range []struct {
		name    string
		actions []chromedp.Action[chromedp.Void]
		op      string   // the operation sent
		want    []string // lines the layout file holds after
		lacks   []string // and does not
	}{
		{"first drag pins every node", []chromedp.Action[chromedp.Void]{dragBy(`[data-node="a"]`, 100, 60)}, "batch",
			[]string{"\n  a: {x: ", "\n  b: {", "\n  c: {", "\n  d: {in: g,", "\n  e: {in: g,", "\n  g: {", "\n  lane: {"}, []string{"s1", "s2", "s3"}},
		{"move", []chromedp.Action[chromedp.Void]{dragBy(`[data-node="c"]`, 0, 40)}, "move", nil, nil},
		{"multi-select move", []chromedp.Action[chromedp.Void]{click(`[data-node="a"]`), click(`[data-node="b"]`, chromedp.ButtonModifiers(chromedp.ModifierShift)), dragBy(`[data-node="b"]`, -40, 20)}, "batch", nil, nil},
		{"align from the context menu", []chromedp.Action[chromedp.Void]{click(`[data-node="a"]`, chromedp.ButtonRight), chromedp.Poll[chromedp.Void](`!document.getElementById('context-menu').hidden`), chromedp.Evaluate[chromedp.Void](`[...document.querySelectorAll('#context-menu button')].find((b) => b.textContent === 'Align left').click()`)}, "move", nil, nil},
		{"distribute", []chromedp.Action[chromedp.Void]{click(`[data-node="c"]`, chromedp.ButtonModifiers(chromedp.ModifierShift)), arrange("y")}, "move", nil, nil},
		{"resize", []chromedp.Action[chromedp.Void]{click(`[data-node="c"]`), dragBy(`[data-handle="resize:se"]`, 40, 20)}, "resize", []string{"w: 140, h: 60"}, nil},
		{"resize from the top left", []chromedp.Action[chromedp.Void]{dragBy(`[data-handle="resize:nw"]`, -20, -20)}, "batch", []string{"w: 160, h: 80"}, nil},
		{"move inside a container stays inside", []chromedp.Action[chromedp.Void]{dragBy(`[data-node="d"]`, -400, -400)}, "move", []string{"d: {in: g, x: 0, y: 0}"}, nil},
		{"reorder in a grid", []chromedp.Action[chromedp.Void]{dragBy(`[data-node="s1"]`, 230, 0)}, "reorder", []string{"order: [s2, s3, s1]"}, nil},
		{"edge end to a side", []chromedp.Action[chromedp.Void]{click(`[data-edge="bc"] path`), dragBy(`[data-handle="end:1"]`, 170, 0)}, "set_edge_hint", []string{"bc: {to_side: right}"}, nil},
		{"middle label", []chromedp.Action[chromedp.Void]{click(`[data-edge="ab"] path`), dragBy(`[data-handle="label"]`, 0, 10)}, "set_edge_hint", []string{"ab: {label_pos: "}, nil},
		{"add a via point", []chromedp.Action[chromedp.Void]{click(`[data-edge="ad"] path`), at(`[data-edge="ad"] path`, func(x, y float64) chromedp.Action[chromedp.Void] {
			return chromedp.MouseClickXY(x, y, chromedp.ClickCount(2))
		})}, "set_edge_hint", []string{"ad: {via: [["}, nil},
		{"remove the via point", []chromedp.Action[chromedp.Void]{click(`[data-edge="ad"] path`), at(`[data-handle="via:0"]`, func(x, y float64) chromedp.Action[chromedp.Void] {
			return chromedp.MouseClickXY(x, y, chromedp.ClickCount(2))
		})}, "set_edge_hint", nil, []string{"ad:"}},
		{"pin a port", []chromedp.Action[chromedp.Void]{click(`[data-node="c"]`), dragBy(`[data-handle="port:in"]`, 100, 30)}, "pin_port", []string{"in: {side: right, offset: "}, nil},
		{"unpin", []chromedp.Action[chromedp.Void]{click(`[data-node="c"]`), chromedp.KeyEvent("u")}, "unpin", nil, []string{"\n  c:"}},
		{"box select and move", []chromedp.Action[chromedp.Void]{chromedp.KeyEvent(kbEscape), at(`#paper`, func(x, y float64) chromedp.Action[chromedp.Void] {
			return chromedp.MouseClickXY(x, y)
		}), boxSelect(`[data-node="a"]`, `[data-node="b"]`), dragBy(`[data-node="b"]`, 0, 40)}, "batch", nil, nil},
	} {
		before := tree(t, e.dir)
		if err := chromedp.Do(ctx, append(g.actions, e.idle())...); err != nil {
			t.Fatalf("%s: %v", g.name, err)
		}
		e.shot(ctx, fmt.Sprintf("%02d-%s", i, strings.ReplaceAll(g.name, " ", "-")))
		reqs := sent(ctx, t)
		if len(reqs) != 1 || reqs[0]["type"] != "op" || reqs[0]["op"].(map[string]any)["op"] != g.op {
			t.Fatalf("%s sent %v, want one %s", g.name, reqs, g.op)
		}
		after := tree(t, e.dir)
		layout := after["views/main.layout.yaml"]
		for _, w := range g.want {
			if !strings.Contains(layout, w) {
				t.Errorf("%s: the layout file lacks %q:\n%s", g.name, w, layout)
			}
		}
		for _, w := range g.lacks {
			if strings.Contains(layout, w) {
				t.Errorf("%s: the layout file holds %q:\n%s", g.name, w, layout)
			}
		}
		if err := chromedp.Do(ctx, chromedp.KeyEvent("z", cmd), e.idle()); err != nil {
			t.Fatal(err)
		}
		if got := tree(t, e.dir); !reflect.DeepEqual(got, before) {
			t.Fatalf("%s: one undo left %v, want %v", g.name, got, before)
		}
		if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`!document.getElementById('redo').disabled`), chromedp.Click(chromedp.CSS(`#redo`)), e.idle()); err != nil {
			t.Fatal(err)
		}
		if got := tree(t, e.dir); !reflect.DeepEqual(got, after) {
			t.Fatalf("%s: redo left %v, want %v", g.name, got, after)
		}
		if reqs := sent(ctx, t); len(reqs) != 2 || reqs[0]["type"] != "undo" || reqs[1]["type"] != "redo" {
			t.Fatalf("%s: undo and redo sent %v", g.name, reqs)
		}
	}
}

const kbEscape = "\u001b"

// boxSelect drags a selection box from above the left of the first element to below the right
// of the second.
func boxSelect(first, second string) chromedp.Action[chromedp.Void] {
	return at(first, func(x0, y0 float64) chromedp.Action[chromedp.Void] {
		return at(second, func(x1, y1 float64) chromedp.Action[chromedp.Void] {
			return drag(min(x0, x1)-80, min(y0, y1)-40, max(x0, x1)-min(x0, x1)+160, max(y0, y1)-min(y0, y1)+80, 8)
		})
	})
}

// waitFiles waits until the project's files are want.
func waitFiles(t *testing.T, e *editor, want map[string]string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := tree(t, e.dir)
		if reflect.DeepEqual(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("files are %v, want %v", got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestEditorClients checks that two clients of one dg serve session share the undo history and
// see each other's edits, and that both go read-only while the project does not parse.
func TestEditorClients(t *testing.T) {
	e := newEditor(t, map[string]string{"model.hcl": editorModel})
	a, b := e.tab(), e.tab()
	ctx, cancel := context.WithTimeout(a, 60*time.Second)
	defer cancel()
	before := tree(t, e.dir)
	position := func(ctx context.Context) string {
		v, err := chromedp.Run(ctx, chromedp.Evaluate[string](`document.querySelector('[data-node="b"]').closest('[transform]').getAttribute('transform')`))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	was := position(b)
	if err := chromedp.Do(ctx, dragBy(`[data-node="b"]`, 120, 40), e.idle()); err != nil {
		t.Fatal(err)
	}
	if len(sent(ctx, t)) != 1 {
		t.Fatal("the drag sent no operation, or more than one")
	}
	err := chromedp.Do(b,
		chromedp.Poll[chromedp.Void](`document.querySelector('[data-node="b"]').closest('[transform]').getAttribute('transform') !== `+jsString(was)),
		chromedp.Poll[chromedp.Void](`!document.getElementById('undo').disabled`),
		chromedp.Click(chromedp.CSS(`#undo`)),
		e.idle(),
	)
	if err != nil {
		t.Fatalf("the other client: %v", err)
	}
	waitFiles(t, e, before)
	if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.getElementById('undo').disabled && !document.getElementById('redo').disabled`)); err != nil {
		t.Fatalf("the history of the first client after the other undid: %v", err)
	}

	if err := os.WriteFile(filepath.Join(e.dir, "model.hcl"), []byte(editorModel+"node {"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []context.Context{a, b} {
		if err := chromedp.Do(c, chromedp.Poll[chromedp.Void](`document.body.classList.contains('stale') && !document.body.classList.contains('editable') && document.querySelector('#diagnostics li.error') !== null`)); err != nil {
			t.Fatal(err)
		}
	}
	broken := tree(t, e.dir)
	if err := chromedp.Do(ctx, dragBy(`[data-node="b"]`, 120, 40), chromedp.Sleep(200*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if reqs := sent(ctx, t); len(reqs) != 0 {
		t.Errorf("a drag while the project does not parse sent %v", reqs)
	}
	if got := tree(t, e.dir); !reflect.DeepEqual(got, broken) {
		t.Error("a drag while the project does not parse changed files")
	}
	if p := position(a); p != was {
		t.Errorf("node b is drawn at %s after a refused drag, want %s", p, was)
	}
}

// TestEditorStaleFrame checks that moving a node whose layout entry names another frame than its
// parent (spec 7.1) writes coordinates in its current frame, so it lands where it was dropped.
func TestEditorStaleFrame(t *testing.T) {
	model := editorModel + "node \"h\" {\n  kind  = \"group\"\n  label = \"Other\"\n}\n"
	layout := "version: 1\nplace:\n  a: {x: 0, y: 0}\n  d: {in: h, x: 40, y: 20}\n  g: {x: 0, y: 200}\n  h: {x: 400, y: 200}\n"
	e := newEditor(t, map[string]string{"model.hcl": model, "views/main.layout.yaml": layout})
	ctx := e.tab()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	box := func() []float64 {
		r, err := chromedp.Run(ctx, chromedp.Evaluate[[]float64](`(() => { const r = document.querySelector('[data-node="d"]').getBoundingClientRect(); const p = document.getElementById('paper').getBoundingClientRect(); return [r.x - p.x, r.y - p.y]; })()`))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	was := box()
	if err := chromedp.Do(ctx, dragBy(`[data-node="d"]`, 20, 0), e.idle()); err != nil {
		t.Fatal(err)
	}
	if got := tree(t, e.dir)["views/main.layout.yaml"]; !strings.Contains(got, "\n  d: {in: g, x: ") {
		t.Fatalf("the move did not write d in its current frame:\n%s", got)
	}
	if got := box(); got[0] != was[0]+20 || got[1] != was[1] {
		t.Errorf("d moved from %v to %v in the paper, want 20 to the right", was, got)
	}
}

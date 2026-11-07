package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/coder/websocket"

	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/project"
)

func TestLoopbackOnly(t *testing.T) {
	_, url := start(t)
	for host, want := range map[string]int{"evil.example": 403, "evil.example:80": 403, "10.0.0.1": 403, strings.TrimPrefix(url, "http://"): 200} {
		req, err := http.NewRequest("GET", url+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("Host %s: status %d, want %d", host, resp.StatusCode, want)
		}
	}
	for addr, want := range map[string]bool{"127.0.0.1:0": true, "localhost:8080": true, "[::1]:0": true, ":8080": false, "0.0.0.0:0": false, "192.168.1.2:80": false} {
		if got := IsLoopback(addr); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

const testModel = `node "a" {
  label = "Alpha"
}

node "b" {
  label = "Beta"
}

edge "ab" {
  from = node.a
  to   = node.b
}
`

// start serves a new project holding model.hcl and watches it until the test ends.
func start(t *testing.T) (dir, url string) {
	t.Helper()
	dir = t.TempDir()
	write(t, dir, testModel)
	s := New(dir)
	srv := httptest.NewServer(s.Handler())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Watch(ctx, func(err error) { t.Error(err) }) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
		srv.Close()
	})
	return dir, srv.URL
}

func write(t *testing.T, dir, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "model.hcl"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

type received struct {
	Type        string `json:"type"`
	Diagnostics []struct {
		Severity string `json:"severity"`
		Summary  string `json:"summary"`
	} `json:"diagnostics"`
	Resolved *struct {
		Nodes []struct {
			Label string `json:"label"`
		} `json:"nodes"`
	} `json:"resolved"`
}

// TestReloadOnPackFiles checks that a running server sees dg.hcl, dg.lock and a pack's files
// change, in a directory created after it started (FR-13).
func TestReloadOnPackFiles(t *testing.T) {
	dir, url := start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(url, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"select-view","view":""}`)); err != nil {
		t.Fatal(err)
	}
	put := func(name, src string) {
		t.Helper()
		file := filepath.Join(dir, name)
		if err := errors.Join(os.MkdirAll(filepath.Dir(file), 0o755), os.WriteFile(file, []byte(src), 0o644)); err != nil {
			t.Fatal(err)
		}
	}
	summaries := func(want ...string) {
		t.Helper()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				t.Fatal(err, want)
			}
			var m received
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, d := range m.Diagnostics {
				got = append(got, d.Summary)
			}
			if m.Type == "diagnostics" && slices.Equal(got, want) {
				return
			}
		}
	}
	summaries()
	put("dg.hcl", "project \"p\" {\n  pack \"aws\" {\n    source  = \"example.com/aws\"\n    version = \"v1\"\n  }\n}\n")
	summaries("Pack not installed", "Pack not locked")
	put("packs/aws/spec.hcl", "kind \"vpc\" {}\n")
	summaries("Pack not locked")
	pack, err := project.ReadPack(os.DirFS(dir), "packs/aws")
	if err != nil {
		t.Fatal(err)
	}
	lock := project.Lock{Packs: map[string]project.LockEntry{"aws": {
		Source: "example.com/aws", Version: "v1", Commit: strings.Repeat("a", 40), Hash: project.PackHash(pack),
	}}}
	put(project.LockFile, string(lock.Encode()))
	summaries()
	put("packs/aws/icons/new.svg", "<svg/>")
	summaries("Pack changed")
}

func TestReload(t *testing.T) {
	dir, url := start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(url, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	next := func() received {
		t.Helper()
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m received
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"select-view","view":""}`)); err != nil {
		t.Fatal(err)
	}
	if m := next(); m.Type != "pictures" {
		t.Fatalf("first message %+v, want the pictures", m)
	}
	if m := next(); m.Type != "model" {
		t.Fatalf("second message %+v, want the model", m)
	}
	if m := next(); m.Type != "snapshot" || m.Resolved.Nodes[0].Label != "Alpha" {
		t.Fatalf("message %+v, want the snapshot", m)
	}
	if m := next(); m.Type != "diagnostics" || len(m.Diagnostics) != 0 {
		t.Fatalf("second message %+v, want no diagnostics", m)
	}
	if m := next(); m.Type != "history" {
		t.Fatalf("third message %+v, want the history", m)
	}

	write(t, dir, strings.Replace(testModel, `"Alpha"`, `"Alpha 2"`, 1))
	if m := next(); m.Type != "model" {
		t.Fatalf("after a save: %+v, want the changed model, without the pictures", m)
	}
	if m := next(); m.Type != "snapshot" || m.Resolved.Nodes[0].Label != "Alpha 2" {
		t.Fatalf("after a save: %+v, want a new snapshot", m)
	}
	next()

	if err := os.WriteFile(filepath.Join(dir, "style.hcl"), []byte("style \"service\" {\n  icon = \"<svg xmlns='http://www.w3.org/2000/svg'/>\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := next(); m.Type != "pictures" {
		t.Fatalf("after a new icon: %+v, want the pictures", m)
	}
	if m := next(); m.Type != "diagnostics" {
		t.Fatalf("after a new icon: %+v, want the diagnostics, the model and the drawing being the same", m)
	}

	write(t, dir, testModel+"node {")
	if m := next(); m.Type != "diagnostics" || len(m.Diagnostics) == 0 || m.Diagnostics[0].Severity != "error" {
		t.Fatalf("after a broken save: %+v, want only diagnostics with an error", m)
	}

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"select-view","view":"nope"}`)); err != nil {
		t.Fatal(err)
	}
	if m := next(); m.Type != "error" {
		t.Fatalf("after selecting an undefined view: %+v, want an error", m)
	}

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"op"}`)); err != nil {
		t.Fatal(err)
	}
	if m := next(); m.Type != "error" {
		t.Fatalf("after an op without an operation: %+v, want an error", m)
	}
}

// TestEditor checks in Chromium that a saved model file is redrawn within 300 milliseconds and
// that a broken one keeps the last drawing (FR-10).
func TestEditor(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir, url := start(t)
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()
	label := func(text string) chromedp.Action[chromedp.Void] {
		return chromedp.Poll[chromedp.Void](`[...document.querySelectorAll('[data-node="a"] text')].some((t) => t.textContent === `+jsString(text)+`)`,
			chromedp.WithPollingInterval(5*time.Millisecond))
	}
	if err := chromedp.Do(ctx, chromedp.Navigate(url), label("Alpha")); err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{"First", "Second", "Third"} {
		start := time.Now()
		write(t, dir, strings.Replace(testModel, `"Alpha"`, jsString(text), 1))
		if err := chromedp.Do(ctx, label(text)); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 300*time.Millisecond {
			t.Errorf("label %q was drawn %v after the save, want at most 300ms", text, d)
		}
	}

	write(t, dir, testModel+"node {")
	err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.querySelector('#diagnostics li.error') !== null && document.body.classList.contains('stale')`))
	if err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, label("Third")); err != nil {
		t.Errorf("the last drawing is gone after a broken save: %v", err)
	}

	write(t, dir, testModel)
	err = chromedp.Do(ctx, label("Alpha"), chromedp.Poll[chromedp.Void](`!document.body.classList.contains('stale') && document.querySelector('#diagnostics').hidden`))
	if err != nil {
		t.Fatal(err)
	}
}

// TestEditorAnimation checks in Chromium that the editor animates an edge with animate = "flow"
// in real time.
func TestEditorAnimation(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir, url := start(t)
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()
	if err := chromedp.Do(ctx, chromedp.Navigate(url), chromedp.Poll[chromedp.Void](`document.querySelector('[data-edge="ab"]') !== null`)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "style.hcl"), []byte("edge_style \"default\" {\n  animate = \"flow\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	running := `document.querySelector('[data-edge="ab"] path').getAnimations().some((a) => a.animationName === 'dg-flow-0' && a.playState === 'running' && a.currentTime > 0)`
	if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](running)); err != nil {
		t.Fatal(err)
	}
}

func jsString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// TestDrillDown checks in Chromium that examples/c4 is navigable by drill-down: a badge opens
// the view of its node, breadcrumbs lead back, and a badge of a node several views show the
// inside of opens a menu of them (spec 8.2).
func TestDrillDown(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "examples", "c4"))); err != nil {
		t.Fatal(err)
	}
	s := New(dir)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	watch, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Watch(watch, func(err error) { t.Error(err) }) }()
	defer func() {
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()
	shows := func(view string) chromedp.Action[chromedp.Void] {
		return chromedp.Poll[chromedp.Void](`document.getElementById('paper').dataset.view === ` + jsString(view))
	}
	crumbs := func(want string) chromedp.Action[chromedp.Void] {
		return chromedp.Poll[chromedp.Void](`(() => { const c = document.getElementById('crumbs'); return (c.hidden ? '' : c.textContent) === ` + jsString(want) + ` })()`)
	}
	steps := []struct {
		name    string
		actions []chromedp.Action[chromedp.Void]
	}{
		{"the context view", []chromedp.Action[chromedp.Void]{chromedp.Navigate(srv.URL), shows("main"), crumbs(""), chromedp.Poll[chromedp.Void](`document.querySelector('[data-node="banking"] [data-collapsed]') !== null`)}},
		{"drill into the system", []chromedp.Action[chromedp.Void]{chromedp.Click(chromedp.CSS(`[data-drill="banking"]`)), shows("containers"), crumbs("Internet Banking System: system context › Internet Banking System: containers")}},
		{"drill into the API", []chromedp.Action[chromedp.Void]{chromedp.Click(chromedp.CSS(`[data-drill="api"]`)), shows("components"), crumbs("Internet Banking System: system context › Internet Banking System: containers › API Application: components")}},
		{"back by the breadcrumb", []chromedp.Action[chromedp.Void]{chromedp.Click(chromedp.CSS(`#crumbs button[data-views="containers"]`)), shows("containers")}},
	}
	for _, st := range steps {
		if err := chromedp.Do(ctx, st.actions...); err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
	}

	views, err := os.ReadFile(filepath.Join(dir, "views.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	more := "\nview \"security\" {\n  title = \"API security\"\n  of    = node.api\n}\n\nview \"deployment\" {\n  title = \"Deployment\"\n  of    = node.banking\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "views.hcl"), append(views, more...), 0o644); err != nil {
		t.Fatal(err)
	}
	err = chromedp.Do(ctx,
		chromedp.Poll[chromedp.Void](`[...document.querySelectorAll('#views option')].some((o) => o.value === 'security')`),
		// The redraw of this reload can replace the badge at any moment, and a press and a release
		// on different elements click neither; a dispatched click is one step.
		js(`(() => { const b = document.querySelector('[data-drill="api"]'); const r = b.getBoundingClientRect(); b.dispatchEvent(new MouseEvent('click', { bubbles: true, clientX: r.x, clientY: r.y })); })()`),
		chromedp.Poll[chromedp.Void](`!document.getElementById('drill-menu').hidden && document.querySelectorAll('#drill-menu button').length === 2`),
		chromedp.Click(chromedp.CSS(`#drill-menu button[data-view="security"]`)),
		shows("security"),
		chromedp.Poll[chromedp.Void](`document.getElementById('drill-menu').hidden`),
		crumbs("Internet Banking System: system context › Internet Banking System: containers (+1) ▾ › API security"),
		chromedp.Click(chromedp.CSS(`#crumbs button[data-views="containers deployment"]`)),
		chromedp.Poll[chromedp.Void](`!document.getElementById('drill-menu').hidden && document.querySelectorAll('#drill-menu button').length === 2`),
		chromedp.Click(chromedp.CSS(`#drill-menu button[data-view="containers"]`)),
		shows("containers"),
	)
	if err != nil {
		t.Fatalf("drill-down menu: %v", err)
	}
}

// TestEditing checks the editing protocol over the WebSocket (FR-10): an operation is answered
// with an ack holding its results, every client gets the new drawing and the history, a batch is
// one undo step, undo and redo from any client of the session restore the files byte for byte,
// and a refused operation is an error that changes nothing.
func TestEditing(t *testing.T) {
	dir, url := start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	type message struct {
		Type     string          `json:"type"`
		Seq      int64           `json:"seq"`
		Message  string          `json:"message"`
		Results  []ops.Result    `json:"results"`
		Files    []string        `json:"files"`
		Undo     bool            `json:"undo"`
		Redo     bool            `json:"redo"`
		Resolved json.RawMessage `json:"resolved"`
	}
	dial := func() func(want string) message {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(url, "http")+"/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"select-view"}`)); err != nil {
			t.Fatal(err)
		}
		return func(send string) message {
			t.Helper()
			if send != "" {
				if err := conn.Write(ctx, websocket.MessageText, []byte(send)); err != nil {
					t.Fatal(err)
				}
			}
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var m message
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				if m.Type == "ack" || m.Type == "error" && m.Seq != 0 || m.Type == "history" && send == "" {
					return m
				}
			}
		}
	}
	a, b := dial(), dial()
	a("")
	b("")
	before := tree(t, dir)

	m := a(`{"type":"op","seq":1,"op":{"op":"batch","ops":[{"op":"move","view":"main","id":"a","x":41,"y":0},{"op":"move","view":"main","id":"b","x":200,"y":0}]}}`)
	x := 40
	if m.Type != "ack" || m.Seq != 1 || len(m.Results) != 2 || *m.Results[0].X != x || !slices.Equal(m.Files, []string{"views/main.layout.yaml"}) {
		t.Fatalf("after a batch: %+v", m)
	}
	if h := b(""); !h.Undo || h.Redo {
		t.Fatalf("the other client's history after the batch: %+v", h)
	}
	after := tree(t, dir)
	if !strings.Contains(after["views/main.layout.yaml"], "a: {x: 40, y: 0}") {
		t.Fatalf("layout file:\n%s", after["views/main.layout.yaml"])
	}

	if m := b(`{"type":"undo","seq":2}`); m.Type != "ack" || m.Seq != 2 {
		t.Fatalf("undo: %+v", m)
	}
	if h := a(""); h.Undo || !h.Redo {
		t.Fatalf("history after the undo: %+v", h)
	}
	if got := tree(t, dir); !reflect.DeepEqual(got, before) {
		t.Fatalf("after undo the files are %v, want %v", got, before)
	}
	if m := a(`{"type":"redo","seq":3}`); m.Type != "ack" {
		t.Fatalf("redo: %+v", m)
	}
	if got := tree(t, dir); !reflect.DeepEqual(got, after) {
		t.Fatalf("after redo the files are %v, want %v", got, after)
	}

	if m := a(`{"type":"op","seq":4,"op":{"op":"move","view":"main","id":"nope","x":0,"y":0}}`); m.Type != "error" || m.Seq != 4 || !strings.Contains(m.Message, `"nope" does not exist`) {
		t.Fatalf("a refused op: %+v", m)
	}
	if got := tree(t, dir); !reflect.DeepEqual(got, after) {
		t.Fatalf("a refused op changed the files")
	}
	if m := a(`{"type":"redo","seq":5}`); m.Type != "error" || !strings.Contains(m.Message, "nothing to redo") {
		t.Fatalf("redo with nothing to redo: %+v", m)
	}
	if m := a(`{"type":"op","seq":6,"op":{"op":"move","view":"main","id":"a","x":10.5,"y":0}}`); m.Type != "error" || m.Seq != 6 {
		t.Fatalf("an invalid op: %+v, want an error with its seq", m)
	}
}

// tree returns every file under dir by its slash path.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/coder/websocket"
)

func TestDecodeMessage(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Message
		err  string
	}{
		{`{"type":"select-view","view":"main"}`, Message{Type: "select-view", View: "main"}, ""},
		{`{"type":"select-view"}`, Message{Type: "select-view"}, ""},
		{`{"type":"op"}`, Message{}, "read-only"},
		{`{"type":"nope"}`, Message{}, "unknown message type"},
		{`{"type":"select-view","extra":1}`, Message{}, "unknown field"},
		{`{"type":"select-view","view":3}`, Message{}, "invalid message"},
		{`{"type":"select-view"} {}`, Message{}, "more than one"},
		{`[`, Message{}, "invalid message"},
	} {
		got, err := DecodeMessage([]byte(tc.in))
		if tc.err == "" && err != nil || tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
			t.Errorf("DecodeMessage(%s) error %v, want %q", tc.in, err, tc.err)
		}
		if got != tc.want {
			t.Errorf("DecodeMessage(%s) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

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

func FuzzDecodeMessage(f *testing.F) {
	for _, s := range []string{`{"type":"select-view","view":"main"}`, `{"type":"op"}`, `{}`, `[]`, `{"type":"select-view","view":"\u0000"}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := DecodeMessage(data)
		if err == nil && m.Type != "select-view" {
			t.Fatalf("accepted message of type %q", m.Type)
		}
	})
}

const model = `node "a" {
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
	write(t, dir, model)
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
	} `json:"diagnostics"`
	Resolved *struct {
		Nodes []struct {
			Label string `json:"label"`
		} `json:"nodes"`
	} `json:"resolved"`
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
	if m := next(); m.Type != "snapshot" || m.Resolved.Nodes[0].Label != "Alpha" {
		t.Fatalf("first message %+v, want the snapshot", m)
	}
	if m := next(); m.Type != "diagnostics" || len(m.Diagnostics) != 0 {
		t.Fatalf("second message %+v, want no diagnostics", m)
	}

	write(t, dir, strings.Replace(model, `"Alpha"`, `"Alpha 2"`, 1))
	if m := next(); m.Type != "snapshot" || m.Resolved.Nodes[0].Label != "Alpha 2" {
		t.Fatalf("after a save: %+v, want a new snapshot", m)
	}
	next()

	write(t, dir, model+"node {")
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
		t.Fatalf("after an op: %+v, want an error", m)
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
		write(t, dir, strings.Replace(model, `"Alpha"`, jsString(text), 1))
		if err := chromedp.Do(ctx, label(text)); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 300*time.Millisecond {
			t.Errorf("label %q was drawn %v after the save, want at most 300ms", text, d)
		}
	}

	write(t, dir, model+"node {")
	err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.querySelector('#diagnostics li.error') !== null && document.body.classList.contains('stale')`))
	if err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, label("Third")); err != nil {
		t.Errorf("the last drawing is gone after a broken save: %v", err)
	}

	write(t, dir, model)
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

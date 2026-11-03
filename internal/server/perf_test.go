package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/io"
	"github.com/chromedp/cdproto/tracing"
	"github.com/chromedp/chromedp"
)

// dense returns a project of 200 pinned nodes in 10 rows of 20 and 400 edges between
// neighbors: to the right, below and below right.
func dense() map[string]string {
	var model, layout strings.Builder
	layout.WriteString("version: 1\nplace:\n")
	id := func(i int) string { return fmt.Sprintf("n%03d", i) }
	for i := range 200 {
		fmt.Fprintf(&model, "node %q {\n  label = \"Node %d\"\n}\n\n", id(i), i)
	}
	for i := range 200 {
		fmt.Fprintf(&layout, "  %s: {x: %d, y: %d}\n", id(i), i%20*200, i/20*140)
	}
	edges := 0
	edge := func(a, b int) {
		if edges < 400 {
			fmt.Fprintf(&model, "edge \"%s-%s\" {\n  from = node.%s\n  to   = node.%s\n}\n\n", id(a), id(b), id(a), id(b))
			edges++
		}
	}
	for i := range 200 {
		if i%20 < 19 {
			edge(i, i+1)
		}
		if i < 180 {
			edge(i, i+20)
		}
	}
	for i := 0; edges < 400; i++ {
		if i%20 < 19 && i < 180 {
			edge(i, i+21)
		}
	}
	return map[string]string{"model.hcl": model.String(), "views/main.layout.yaml": layout.String()}
}

// traceEvent is an event of a Chromium trace.
type traceEvent struct {
	Name string          `json:"name"`
	Cat  string          `json:"cat"`
	Ph   string          `json:"ph"`
	Ts   float64         `json:"ts"`
	Dur  float64         `json:"dur"`
	Pid  int             `json:"pid"`
	Tid  int             `json:"tid"`
	Args json.RawMessage `json:"args"`
}

// record runs a while recording a Chromium performance trace and returns its events.
func record(ctx context.Context, a chromedp.Action[chromedp.Void]) ([]traceEvent, error) {
	complete := chromedp.Events(ctx, tracing.TracingComplete)
	err := chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		_, err := cdp.Call(ctx, t, tracing.Start, tracing.StartParams{
			TransferMode: tracing.StartTransferModeReturnAsStream,
			TraceConfig:  &tracing.TraceConfig{IncludedCategories: []string{"devtools.timeline", "toplevel", "disabled-by-default-devtools.timeline", "disabled-by-default-devtools.timeline.frame", "benchmark"}},
		})
		return err
	}), a, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		_, err := cdp.Call(ctx, t, tracing.End, cdp.Empty{})
		return err
	}))
	if err != nil {
		return nil, err
	}
	for ev, err := range complete {
		if err != nil {
			return nil, err
		}
		var data []byte
		err := chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			for {
				r, err := cdp.Call(ctx, t, io.Read, io.ReadParams{Handle: ev.Stream})
				if err != nil {
					return err
				}
				data = append(data, r.Data...)
				if r.EOF {
					_, err := cdp.Call(ctx, t, io.Close, io.CloseParams{Handle: ev.Stream})
					return err
				}
			}
		}))
		if err != nil {
			return nil, err
		}
		var trace struct {
			TraceEvents []traceEvent `json:"traceEvents"`
		}
		if err := json.Unmarshal(data, &trace); err != nil {
			return nil, err
		}
		return trace.TraceEvents, nil
	}
	return nil, ctx.Err()
}

// frameStats reads a trace of a drag: the window from the first to the last pointermove
// dispatched on the page's main thread, the longest task of that thread in it and after it (the
// drop), and the frames the page's compositor began in it by their state (presented, dropped, or
// no update desired).
func frameStats(events []traceEvent) (window, longest, after float64, states map[string]int, err error) {
	var pid, tid int
	start, end := -1.0, -1.0
	for _, ev := range events {
		if ev.Name == "EventDispatch" && strings.Contains(string(ev.Args), `"pointermove"`) {
			if start < 0 || ev.Ts < start {
				start = ev.Ts
			}
			end = max(end, ev.Ts+ev.Dur)
			pid, tid = ev.Pid, ev.Tid
		}
	}
	if start < 0 {
		return 0, 0, 0, nil, fmt.Errorf("the trace holds no pointermove")
	}
	states = map[string]int{}
	for _, ev := range events {
		if ev.Pid == pid && ev.Name == "PipelineReporter" && ev.Ph == "b" && strings.Contains(string(ev.Args), `"frame_reporter"`) {
			var a struct {
				R struct {
					State string `json:"state"`
				} `json:"frame_reporter"`
			}
			if err := json.Unmarshal(ev.Args, &a); err != nil {
				return 0, 0, 0, nil, err
			}
			if ev.Ts >= start && ev.Ts <= end {
				states[a.R.State]++
			}
		}
		if ev.Pid != pid || ev.Tid != tid || ev.Name != "ThreadControllerImpl::RunTask" || ev.Ph != "X" {
			continue
		}
		if ev.Ts+ev.Dur >= start && ev.Ts <= end {
			longest = max(longest, ev.Dur/1000)
		} else if ev.Ts > end {
			after = max(after, ev.Dur/1000)
		}
	}
	return (end - start) / 1000, longest, after, states, nil
}

// TestDragPerformance drags one node, eight nodes and every node of a view of 200 nodes and 400
// edges for a second and a half each, a pointer move per frame, and reads the performance trace:
// no task of the page's main thread may take longer than a frame at 60 frames per second, no
// frame may be dropped, and frames must be presented whole at that rate (FR-8, spec 11). After
// each drop, the editor must draw the change and be editable again within afterDrop. The window is
// as large as the viewport, since headless Chromium draws no frame for changes outside its window.
func TestDragPerformance(t *testing.T) {
	e := newEditor(t, dense())
	ctx, _ := chromedp.NewContext(e.browser)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	err := chromedp.Do(ctx,
		chromedp.Navigate(e.url),
		chromedp.Poll[chromedp.Void](`document.body.classList.contains('editable') && document.querySelectorAll('[data-edge]').length === 400`, chromedp.WithPollingInterval(100*time.Millisecond)),
	)
	if err != nil {
		t.Fatal(err)
	}
	const budget = 1000.0 / 60
	// Each step moves the pointer a grid unit or more, so every frame draws something new.
	for _, tc := range []struct {
		name   string
		start  chromedp.Action[chromedp.Void]
		node   string
		dx, dy float64
		live   string // how the edges with one end moving are drawn: routed, lines or none; "" for either of the first two
		// afterDrop bounds the milliseconds from the drop until the editor has drawn the change
		// and is editable again (open decision "Drawing a dense view after a drop"); 0 for none.
		afterDrop float64
	}{
		{"one node", nil, "n021", 900, 300, "routed", 500},
		{"eight nodes", boxSelect(`[data-node="n062"]`, `[data-node="n085"]`), "n062", 900, 200, "", 0},
		{"every node", chromedp.KeyEvent("a", chromedp.KeyModifiers(chromedp.ModifierCtrl)), "n000", 900, 200, "none", 0},
	} {
		if tc.start != nil {
			if err := chromedp.Do(ctx, tc.start); err != nil {
				t.Fatal(err)
			}
		}
		err := chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`(() => {
			const d = window.drop = { at: 0, off: false, ready: 0 };
			document.addEventListener('pointerup', () => (d.at = performance.now()), { capture: true, once: true });
			const editable = () => document.body.classList.contains('editable');
			const o = new MutationObserver(() => {
				if (d.at && !editable()) d.off = true;
				if (d.off && editable()) {
					d.ready = performance.now();
					o.disconnect();
				}
			});
			o.observe(document.body, { attributes: true, attributeFilter: ['class'] });
		})()`))
		if err != nil {
			t.Fatal(err)
		}
		var live string
		events, err := record(ctx, at(`[data-node="`+tc.node+`"]`, func(x, y float64) chromedp.Action[chromedp.Void] {
			steps := paced(x, y, tc.dx, tc.dy, 90)
			read := chromedp.Func(func(ctx context.Context, t *chromedp.Target) (err error) {
				live, err = chromedp.Evaluate[string](`document.querySelector('[data-preview]').dataset.live`)(ctx, t)
				return err
			})
			drawn := chromedp.Poll[chromedp.Void](`window.drop.ready > 0`, chromedp.WithPollingInterval(20*time.Millisecond))
			return chromedp.Steps(append(steps[:len(steps)-1:len(steps)-1], read, steps[len(steps)-1], drawn)...)
		}))
		if err != nil {
			t.Fatal(err)
		}
		window, longest, stall, states, err := frameStats(events)
		if err != nil {
			t.Fatal(err)
		}
		presented := states["STATE_PRESENTED_ALL"]
		fps := float64(presented) / window * 1000
		t.Logf("%s: %.0f ms of dragging, longest task %.1f ms, %d frames presented (%.1f per second), %v; edges %s", tc.name, window, longest, presented, fps, states, live)
		if live != tc.live && (tc.live != "" || live == "none") {
			t.Errorf("%s: moving edges drawn %s, want %s", tc.name, live, tc.live)
		}
		if longest > budget {
			t.Errorf("%s: a task took %.1f ms, longer than a frame (%.1f ms)", tc.name, longest, budget)
		}
		if states["STATE_DROPPED"] > 0 {
			t.Errorf("%s: %d frames dropped", tc.name, states["STATE_DROPPED"])
		}
		if fps < 55 {
			t.Errorf("%s: %.1f frames per second, want 60", tc.name, fps)
		}
		after, err := chromedp.Run(ctx, chromedp.Evaluate[float64](`window.drop.ready - window.drop.at`))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: editable again %.0f ms after the drop, longest task meanwhile %.0f ms", tc.name, after, stall)
		if tc.afterDrop > 0 && after > tc.afterDrop {
			t.Errorf("%s: the editor took %.0f ms after the drop to draw the change, want at most %.0f ms", tc.name, after, tc.afterDrop)
		}
	}
}

// paced returns the steps that press the left button at (x, y), move the pointer by (dx, dy) in
// steps, each sent when the page has handled the one before, and release it.
func paced(x, y, dx, dy float64, steps int) []chromedp.Action[chromedp.Void] {
	actions := []chromedp.Action[chromedp.Void]{chromedp.MouseEvent(chromedp.MouseMoved, x, y), chromedp.MouseEvent(chromedp.MousePressed, x, y, chromedp.ButtonLeft, chromedp.ClickCount(1))}
	for i := 1; i <= steps; i++ {
		f := float64(i) / float64(steps)
		actions = append(actions, chromedp.MouseEvent(chromedp.MouseMoved, x+dx*f, y+dy*f, chromedp.ButtonLeft))
	}
	return append(actions, chromedp.MouseEvent(chromedp.MouseReleased, x+dx, y+dy, chromedp.ButtonLeft, chromedp.ClickCount(1)))
}

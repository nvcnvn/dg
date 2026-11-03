// Package export draws views in headless Chromium with the embedded app.
package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	goruntime "runtime"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/nvcnvn/dg/internal/view"
	"github.com/nvcnvn/dg/web"
)

// Timeout bounds one export, from starting Chromium to reading the result.
const Timeout = 60 * time.Second

// Result is a drawn view.
type Result struct {
	SVG  []byte
	JSON []byte
}

// Render draws v and returns it as SVG and as JSON geometry.
func Render(ctx context.Context, v *view.Resolved) (*Result, error) {
	viewJSON, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	type result struct {
		SVG  string `json:"svg"`
		JSON string `json:"json"`
	}
	var out result
	ctx, stop, err := openApp(ctx)
	if err == nil {
		defer stop()
		out, err = chromedp.Run(ctx, chromedp.Evaluate[result]("window.dg.render("+string(viewJSON)+")", chromedp.EvalAwaitPromise))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("export timed out after %s", Timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	return &Result{SVG: []byte(out.SVG), JSON: []byte(out.JSON)}, nil
}

// openApp serves the embedded app on a loopback port and opens it in
// headless Chromium. Chromium resolves no host name, and the app's content
// security policy allows only the embedded files, so the app never reaches
// the network. stop closes both.
func openApp(ctx context.Context) (_ context.Context, stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	srv := &http.Server{Handler: web.Handler(), ReadHeaderTimeout: Timeout}
	go func() { _ = srv.Serve(ln) }()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"),
	)
	if goruntime.GOOS == "linux" && os.Geteuid() == 0 {
		// Chromium refuses to start its sandbox as root, as in most CI containers.
		opts = append(opts, chromedp.NoSandbox)
	}
	ctx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	ctx, cancelTab := chromedp.NewContext(ctx)
	stop = func() {
		cancelTab()
		cancelAlloc()
		_ = srv.Close()
	}
	err = chromedp.Do(ctx,
		chromedp.Navigate("http://"+ln.Addr().String()+"/"),
		chromedp.Poll[chromedp.Void]("window.dg !== undefined", chromedp.WithPollingTimeout(0)),
	)
	if err != nil {
		stop()
		return nil, nil, fmt.Errorf("open app: %w", err)
	}
	return ctx, stop, nil
}

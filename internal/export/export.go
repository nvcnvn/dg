// Package export draws views in headless Chromium with the embedded app.
package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
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

// GIF holds one animation period (spec 4.3) as GIFFrames frames of GIFDelay each.
const (
	GIFFrames = 10
	GIFDelay  = 50 * time.Millisecond
)

// Result is a drawn view, TooSmall the containers too small for their children and Overflowing
// the nodes too small for their labels (spec 7.2).
type Result struct {
	SVG         []byte
	JSON        []byte
	PNG         []byte // only for format "png": the drawing at animation time 0
	GIF         []byte // only for format "gif": one frame per GIFDelay of animation time
	TooSmall    []string
	Overflowing []string
}

// Render draws v as SVG, JSON geometry and, when format is "png" or "gif", that picture at one
// pixel per unit.
func Render(ctx context.Context, v *view.Resolved, format string) (*Result, error) {
	return RenderScaled(ctx, v, format, 1)
}

// RenderScaled is Render with PNG pictures at scale pixels per unit.
func RenderScaled(ctx context.Context, v *view.Resolved, format string, scale float64) (*Result, error) {
	viewJSON, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	type drawn struct {
		SVG         string   `json:"svg"`
		JSON        string   `json:"json"`
		TooSmall    []string `json:"too_small"`
		Overflowing []string `json:"overflowing"`
	}
	var out drawn
	res := &Result{}
	ctx, stop, err := openApp(ctx)
	if err == nil {
		defer stop()
		out, err = chromedp.Run(ctx, chromedp.Evaluate[drawn]("window.dg.render("+string(viewJSON)+")", chromedp.EvalAwaitPromise))
	}
	if err == nil && format == "png" {
		res.PNG, err = capture(ctx, 0, scale)
	}
	if err == nil && format == "gif" {
		res.GIF, err = captureGIF(ctx)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("export timed out after %s", Timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	res.SVG, res.JSON, res.TooSmall, res.Overflowing = []byte(out.SVG), []byte(out.JSON), out.TooSmall, out.Overflowing
	return res, nil
}

// capture returns a PNG screenshot of the drawn view with the animation clock at t, at scale
// pixels per unit.
func capture(ctx context.Context, t time.Duration, scale float64) ([]byte, error) {
	_, err := chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void](fmt.Sprintf("window.dg.setClock(%d)", t.Milliseconds()), chromedp.EvalAwaitPromise))
	if err != nil {
		return nil, err
	}
	return chromedp.Run(ctx, chromedp.ScreenshotScale(chromedp.CSS("#export > svg"), scale))
}

func captureGIF(ctx context.Context) ([]byte, error) {
	frames := make([]image.Image, GIFFrames)
	for i := range frames {
		data, err := capture(ctx, time.Duration(i)*GIFDelay, 1)
		if err != nil {
			return nil, err
		}
		if frames[i], err = png.Decode(bytes.NewReader(data)); err != nil {
			return nil, err
		}
	}
	return encodeGIF(frames, GIFDelay)
}

// openApp serves the embedded app on a loopback port and opens it in export
// mode in headless Chromium. Chromium resolves no host name, and the app's content
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
		// GPU rasterization antialiases some tiles differently from run to run when the machine
		// is busy, so PNG and GIF exports would differ; software rasterization does not.
		chromedp.DisableGPU,
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
		chromedp.Navigate("http://"+ln.Addr().String()+"/?export"),
		chromedp.Poll[chromedp.Void]("window.dg !== undefined", chromedp.WithPollingTimeout(0)),
	)
	if err != nil {
		stop()
		return nil, nil, fmt.Errorf("open app: %w", err)
	}
	return ctx, stop, nil
}

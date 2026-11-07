package export

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

// watchRequests returns a function that gives the URLs ctx's page has asked for from now on, other
// than data: URIs. Each document the page loads after this lists what the content security policy
// refuses in window.blocked.
func watchRequests(t *testing.T, ctx context.Context) func() []string {
	t.Helper()
	var mu sync.Mutex
	var urls []string
	events := chromedp.Events(ctx, network.RequestWillBeSent)
	go func() {
		for ev, err := range events {
			if err != nil {
				return
			}
			if !strings.HasPrefix(ev.Request.URL, "data:") {
				mu.Lock()
				urls = append(urls, ev.Request.URL)
				mu.Unlock()
			}
		}
	}()
	err := chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		_, err := cdp.Call(ctx, t, network.Enable, network.EnableParams{})
		return err
	}), chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		_, err := cdp.Call(ctx, t, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: `window.blocked = []; document.addEventListener('securitypolicyviolation', (e) => window.blocked.push(e.blockedURI + ' ' + e.violatedDirective));`})
		return err
	}))
	if err != nil {
		t.Fatal(err)
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(urls)
	}
}

func canaries(urls []string) []string {
	return slices.DeleteFunc(slices.Clone(urls), func(u string) bool { return !strings.Contains(u, "canary") })
}

var hrefRE = regexp.MustCompile(`(?s)<image[^>]*? href="([^"]*)"[^>]*/>`)

// inlined is svg with each picture as markup instead of an <image>: what spec 4.7 forbids.
func inlined(t *testing.T, svg string) string {
	t.Helper()
	return hrefRE.ReplaceAllStringFunc(svg, func(m string) string {
		text, err := url.PathUnescape(strings.TrimPrefix(hrefRE.FindStringSubmatch(m)[1], "data:image/svg+xml,"))
		if err != nil {
			t.Fatal(err)
		}
		return text
	})
}

// open navigates to svg in a file of its own, as someone opens an export, and gives its scripts
// and requests a moment; nothing else tells when a request that is not sent has not been sent.
func open(t *testing.T, ctx context.Context, svg string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "export.svg")
	if err := os.WriteFile(file, []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, chromedp.Navigate("file://"+file), chromedp.Sleep(300*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
}

func ran(t *testing.T, ctx context.Context) bool {
	t.Helper()
	got, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`window.dgRan === true`))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestPicturesAreInert exports a project with a hostile SVG in every place a picture goes, an icon,
// an image, each stretch of a custom shape and an image decoration: drawing it runs and loads
// nothing, and neither does the exported SVG opened on its own, with no content security policy
// to help. The same SVG with the pictures inlined runs and loads, so the test would see it.
func TestPicturesAreInert(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	p, diags, err := project.Load(filepath.Join("..", "..", "testdata", "inert"))
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v, diags := p.Resolve(view.DefaultView)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	viewJSON, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop, err := openApp(context.Background(), chromedp.WindowSize(800, 600))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	seen := watchRequests(t, ctx)
	if err := chromedp.Do(ctx, chromedp.Reload(), chromedp.Poll[chromedp.Void]("window.dg !== undefined")); err != nil {
		t.Fatal(err)
	}

	out, err := chromedp.Run(ctx, chromedp.Evaluate[struct {
		SVG string `json:"svg"`
	}]("window.dg.render("+string(viewJSON)+")", chromedp.EvalAwaitPromise))
	if err != nil {
		t.Fatal(err)
	}
	picture, err := capture(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(ctx, chromedp.Sleep(300*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	blocked, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](`window.blocked`))
	if err != nil {
		t.Fatal(err)
	}
	if got := canaries(seen()); len(got) > 0 || len(blocked) > 0 {
		t.Errorf("drawing the export: asked for %v, refused %v", got, blocked)
	}
	if !redPixel(t, picture) {
		t.Error("the export drew none of the picture")
	}

	if n := len(hrefRE.FindAllString(out.SVG, -1)); n < 5 {
		t.Errorf("the export has %d pictures, want one for each place", n)
	}
	for _, m := range hrefRE.FindAllStringSubmatch(out.SVG, -1) {
		if !strings.HasPrefix(m[1], "data:image/svg+xml,") {
			t.Errorf("a picture comes from %.60s", m[1])
		}
	}
	markup := hrefRE.ReplaceAllString(out.SVG, "")
	for _, s := range []string{"script", "canary", "onload"} {
		if strings.Contains(markup, s) {
			t.Errorf("the export outside its data: URIs holds %q", s)
		}
	}

	before := len(seen())
	open(t, ctx, out.SVG)
	if got := canaries(seen()[before:]); len(got) > 0 || ran(t, ctx) {
		t.Errorf("the exported file on its own: asked for %v, ran %v", got, ran(t, ctx))
	}

	before = len(seen())
	open(t, ctx, inlined(t, out.SVG))
	got := canaries(seen()[before:])
	if !ran(t, ctx) {
		t.Error("control: the inlined picture's script did not run, so the test cannot see one that does")
	}
	for _, want := range []string{"script", "href.png", "canary-relative.png"} {
		if !slices.ContainsFunc(got, func(u string) bool { return strings.Contains(u, want) }) {
			t.Errorf("control: the inlined picture asked for %v, none with %q, so the test cannot see one that does", got, want)
		}
	}
}

// redPixel reports whether data, a PNG, shows the fill of the hostile picture.
func redPixel(t *testing.T, data []byte) bool {
	t.Helper()
	pic, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := pic.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, g, bl, _ := pic.At(x, y).RGBA(); r>>8 == 0xD9 && g>>8 == 0x30 && bl>>8 == 0x25 {
				return true
			}
		}
	}
	return false
}

package server

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// inertEvidence is what the editor's page shows of pictures: everything that holds one, and
// anything a picture brought into the page.
const inertEvidence = `(() => {
	const images = [...document.querySelectorAll('#paper image, #kinds image')];
	return {
		canvas: document.querySelectorAll('#paper image').length,
		palette: document.querySelectorAll('#kinds image').length,
		notData: images.filter((im) => !im.getAttribute('href')?.startsWith('data:image/svg+xml,')).length,
		foreign: document.querySelectorAll('#paper script, #kinds script, #paper [onload], #kinds [onload]').length,
		blocked: window.blocked,
	};
})()`

type evidence struct {
	Canvas  int      `json:"canvas"`
	Palette int      `json:"palette"`
	NotData int      `json:"notData"`
	Foreign int      `json:"foreign"`
	Blocked []string `json:"blocked"`
}

// TestPicturesAreInert opens a project with a hostile SVG in every place a picture goes, an icon,
// an image, each stretch of a custom shape and an image decoration, in the canvas, the palette, a
// drag's preview and both exports: it runs and loads nothing. The picture's markup put in the page
// makes the requests and refusals seen here, so the test would see them.
func TestPicturesAreInert(t *testing.T) {
	e := newEditor(t, tree(t, filepath.Join("..", "..", "testdata", "inert")))
	downloads := t.TempDir()
	ctx := e.page(1440, 900, downloads)
	var mu sync.Mutex
	var urls []string
	events := chromedp.Events(ctx, network.RequestWillBeSent)
	go func() {
		for ev, err := range events {
			if err != nil {
				return
			}
			mu.Lock()
			urls = append(urls, ev.Request.URL)
			mu.Unlock()
		}
	}()
	canaries := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.DeleteFunc(slices.Clone(urls), func(u string) bool { return strings.HasPrefix(u, "data:") || !strings.Contains(u, "canary") })
	}
	err := chromedp.Do(ctx,
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := cdp.Call(ctx, t, network.Enable, network.EnableParams{})
			return err
		}),
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := cdp.Call(ctx, t, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: `window.blocked = []; document.addEventListener('securitypolicyviolation', (e) => window.blocked.push(e.blockedURI + ' ' + e.violatedDirective));`})
			return err
		}),
		chromedp.Navigate(e.url),
		e.idle(),
		dragBy(`[data-node="badge"]`, 60, 40),
		e.idle(),
		toolMenu("save", "Export SVG"),
		toolMenu("save", "Export PNG"),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.svg", "main.png"} {
		if len(download(t, downloads, name)) == 0 {
			t.Errorf("%s is empty", name)
		}
	}
	got, err := chromedp.Run(ctx, chromedp.Evaluate[evidence](inertEvidence))
	if err != nil {
		t.Fatal(err)
	}
	// One per picture on the canvas: an icon, an image, a decoration, a uniform and a natural
	// shape, and a nine-slice shape's picture in the paper's defs, which its slices <use>.
	if got.Canvas < 6 || got.Palette < 5 || got.NotData != 0 || got.Foreign != 0 || len(got.Blocked) != 0 {
		t.Errorf("the page shows %+v", got)
	}
	if asked := canaries(); len(asked) != 0 {
		t.Errorf("the page asked for %v", asked)
	}

	err = chromedp.Do(ctx,
		js(`(() => {
			const text = decodeURIComponent(document.querySelector('#paper image').getAttribute('href').replace('data:image/svg+xml,', ''));
			const paper = document.querySelector('#paper svg');
			paper.append(document.createRange().createContextualFragment(text));
		})()`),
		chromedp.Poll[chromedp.Void](`window.blocked.length >= 2`),
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err = chromedp.Run(ctx, chromedp.Evaluate[evidence](inertEvidence))
	if err != nil {
		t.Fatal(err)
	}
	if got.Foreign == 0 {
		t.Error("control: the inlined markup is not in the page")
	}
	for _, want := range []string{"script-src", "img-src"} {
		if !slices.ContainsFunc(got.Blocked, func(b string) bool { return strings.Contains(b, want) }) {
			t.Errorf("control: refused %v, none by %s, so the test cannot see one that is", got.Blocked, want)
		}
	}
	if asked := canaries(); !slices.ContainsFunc(asked, func(u string) bool { return strings.HasSuffix(u, "/canary-relative.png") }) {
		t.Errorf("control: the inlined markup asked for %v, so the test cannot see one that does", asked)
	}
}

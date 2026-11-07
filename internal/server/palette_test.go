package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"

	"github.com/nvcnvn/dg/internal/project"
)

const bigKinds = 900

// bigPack returns a project with an installed pack of bigKinds kinds with icons, in 30 categories
// and none for ten of them, and one node of the generic pack.
func bigPack(t *testing.T) map[string]string {
	t.Helper()
	var spec, style strings.Builder
	for i := range bigKinds {
		category := ""
		if i >= 10 {
			category = fmt.Sprintf("  category = %q\n", fmt.Sprintf("Group %02d", i%30))
		}
		fmt.Fprintf(&spec, "kind \"k%d\" {\n%s}\n", i, category)
		fmt.Fprintf(&style, "style \"k%d\" {\n  icon = file(\"i.svg\")\n}\n", i)
	}
	pack := map[string]string{
		"spec.hcl":  spec.String(),
		"style.hcl": style.String(),
		"i.svg":     `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="#1A73E8"/></svg>`,
	}
	dir := t.TempDir()
	files := map[string]string{
		"dg.hcl":    "project \"p\" {\n  pack \"big\" {\n    source  = \"example.com/big\"\n    version = \"v1\"\n  }\n}\n",
		"model.hcl": "node \"a\" {\n  label = \"Alpha\"\n}\n",
		"spec.hcl":  "kind \"mine\" {}\n",
		"views.hcl": "view \"main\" {\n}\n",
	}
	for name, src := range pack {
		file := filepath.Join(dir, "packs", "big", name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		files["packs/big/"+name] = src
	}
	read, err := project.ReadPack(os.DirFS(dir), "packs/big")
	if err != nil {
		t.Fatal(err)
	}
	lock := project.Lock{Packs: map[string]project.LockEntry{"big": {Source: "example.com/big", Version: "v1", Commit: strings.Repeat("a", 40), Hash: project.PackHash(read)}}}
	files[project.LockFile] = string(lock.Encode())
	return files
}

// TestPaletteCategories checks FR-20 with a pack of 900 kinds: it starts closed and draws nothing
// until a group opens, from the keyboard too, and the filter finds kinds by either name and by
// category, and leaves the groups as they were when it is cleared.
func TestPaletteCategories(t *testing.T) {
	e := newEditor(t, bigPack(t))
	ctx := e.page(1440, 900, t.TempDir())
	count := func(sel string) int {
		t.Helper()
		n, err := chromedp.Run(ctx, chromedp.Evaluate[int](`document.querySelectorAll(`+jsString(sel)+`).length`))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	bigKind := `[data-kind^="big-"]`
	if err := chromedp.Do(ctx, e.idle()); err != nil {
		t.Fatal(err)
	}
	if got := count(`#kinds details`); got != 31 {
		t.Errorf("%d groups, want 30 categories and the kinds without one", got)
	}
	if got := count(`#kinds details[open]`); got != 0 {
		t.Errorf("%d groups start open", got)
	}
	if got := count(bigKind); got != 0 {
		t.Errorf("%d kinds of the pack are in the page before a group opens", got)
	}
	others := count(`#kinds [data-kind]`)
	heads, err := chromedp.Run(ctx, chromedp.Evaluate[string](`[...document.querySelectorAll('#kinds h3')].map((h) => h.textContent).join(' ') + ' | ' + [...document.querySelectorAll('#kinds summary')].slice(0, 2).map((s) => s.innerText.replace(/\s+/g, ' ')).join(' | ')`))
	if err != nil {
		t.Fatal(err)
	}
	if want := "generic big Project | No category 10 | Group 00 29"; heads != want {
		t.Errorf("headings %q, want %q", heads, want)
	}

	// The Enter key on the focused summary opens it; its kinds are drawn then.
	err = chromedp.Do(ctx,
		js(`[...document.querySelectorAll('#kinds summary')].find((s) => s.textContent.includes('Group 13')).focus()`),
		chromedp.KeyEvent("\r"),
		chromedp.Poll[chromedp.Void](`document.querySelectorAll('#kinds details[open] [data-kind]').length > 0`, chromedp.WithPollingInterval(10*time.Millisecond)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := count(bigKind); got != 30 {
		t.Errorf("%d kinds drawn in the opened group, want 30", got)
	}
	if got := count(bigKind + ` image`); got != 30 {
		t.Errorf("%d pictures drawn in the opened group, want 30", got)
	}
	text, err := chromedp.Run(ctx, chromedp.Evaluate[string](`document.querySelector('[data-kind="big-k43"]').innerText.trim() + ' ' + document.querySelector('[data-kind="big-k43"]').dataset.tip`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "k43 ") || !strings.Contains(text, "big-k43") {
		t.Errorf("the kind shows %q: its short name, its whole name in the tooltip", text)
	}
	noViolations(t, ctx, "a group open")

	filter := func(q string, want int) {
		t.Helper()
		err := chromedp.Do(ctx, js(`(() => { const f = document.getElementById('palette-filter'); f.value = `+jsString(q)+`; f.dispatchEvent(new Event('input')); })()`))
		if err != nil {
			t.Fatal(err)
		}
		if got := count(`#kinds [data-kind]`); got != want {
			t.Errorf("filter %q shows %d kinds, want %d", q, got, want)
		}
		if got, open := count(`#kinds details`), count(`#kinds details[open]`); q != "" && got != open {
			t.Errorf("filter %q leaves %d of %d groups closed", q, got-open, got)
		}
	}
	filter("k899", 1)
	filter("big-k899", 1)
	filter("group 29", 30)
	filter("nothing", 0)
	if got := count(`#kinds h3`); got != 0 {
		t.Errorf("no match leaves %d headings", got)
	}
	start := time.Now()
	filter("big", bigKinds)
	t.Logf("filter matching every kind of the pack: %s", time.Since(start))

	filter("", others+30)
	if got := count(`#kinds details[open]`); got != 1 {
		t.Errorf("%d groups open after the filter, want the one opened before it", got)
	}
	if got := count(bigKind + ` image`); got != 30 {
		t.Errorf("%d pictures after the filter, want those of the open group", got)
	}

	for _, theme := range shotThemes {
		err := chromedp.Do(ctx, js(`document.querySelector('#kinds summary').scrollIntoView()`), chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			_, err := cdp.Call(ctx, t, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: theme}}})
			return err
		}), chromedp.Sleep(150*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		e.shot(ctx, "palette-"+theme)
	}
}

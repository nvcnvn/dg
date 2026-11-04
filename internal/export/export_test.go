package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/view"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	for name, v := range exampleViews(t) {
		t.Run(name, func(t *testing.T) {
			first, err := Render(context.Background(), v, "png")
			if err != nil {
				t.Fatal(err)
			}
			second, err := Render(context.Background(), v, "png")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first.SVG, second.SVG) || !bytes.Equal(first.JSON, second.JSON) || !bytes.Equal(first.PNG, second.PNG) {
				t.Fatal("two exports of the same view differ")
			}
			pic, err := png.DecodeConfig(bytes.NewReader(first.PNG))
			if err != nil {
				t.Fatal(err)
			}
			if size := fmt.Sprintf(`width="%d" height="%d"`, pic.Width, pic.Height); !bytes.Contains(first.SVG, []byte(size)) {
				t.Errorf("the PNG is %dx%d, the SVG is not %s", pic.Width, pic.Height, size)
			}
			golden(t, filepath.Join("..", "..", "testdata", "export", name+".svg"), first.SVG)
			golden(t, filepath.Join("..", "..", "testdata", "export", name+".json"), first.JSON)
		})
	}
}

// TestPinnedLabels exports a node pinned too small for its label, which is reported and stays in
// the picture, and a pinned empty container narrower than min_w, which is not too small for
// anything. The PNG holds nothing but the drawing: no pixel of the page around it.
func TestPinnedLabels(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir := t.TempDir()
	files := map[string]string{
		"model.hcl": `node "short" {
  label = "A label long enough to wrap over several lines in a box this narrow"
}

node "bar" {
  kind = "group"
}
`,
		"views/main.layout.yaml": `version: 1
place:
  bar: {x: 300, y: 0, w: 40, h: 300}
  short: {x: 0, y: 100, w: 120, h: 20}
`,
	}
	for name, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, diags, err := project.Load(dir)
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v, diags := p.Resolve(view.DefaultView)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	res, err := Render(context.Background(), v, "png")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TooSmall) != 0 {
		t.Errorf("too small containers %v, want none", res.TooSmall)
	}
	if len(res.Overflowing) != 1 || res.Overflowing[0] != "short" {
		t.Errorf("overflowing nodes %v, want [short]", res.Overflowing)
	}
	pic, err := png.Decode(bytes.NewReader(res.PNG))
	if err != nil {
		t.Fatal(err)
	}
	b := pic.Bounds()
	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Min.Y+10; y++ {
			if r, g, b, _ := pic.At(x, y).RGBA(); r != 0xffff || g != 0xffff || b != 0xffff {
				t.Fatalf("the top margin is not white at (%d, %d)", x, y)
			}
		}
	}
}

// TestScale checks that a scaled PNG is that many pixels per unit, the same on every run, and
// that geometry does not change with it.
func TestScale(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	v := exampleViews(t)["hello"]
	one, err := Render(context.Background(), v, "png")
	if err != nil {
		t.Fatal(err)
	}
	two, err := RenderScaled(context.Background(), v, "png", 2)
	if err != nil {
		t.Fatal(err)
	}
	again, err := RenderScaled(context.Background(), v, "png", 2)
	if err != nil {
		t.Fatal(err)
	}
	a, err := png.DecodeConfig(bytes.NewReader(one.PNG))
	if err != nil {
		t.Fatal(err)
	}
	b, err := png.DecodeConfig(bytes.NewReader(two.PNG))
	if err != nil {
		t.Fatal(err)
	}
	if b.Width != 2*a.Width || b.Height != 2*a.Height {
		t.Errorf("at scale 2 the PNG is %dx%d, at 1 it is %dx%d", b.Width, b.Height, a.Width, a.Height)
	}
	if !bytes.Equal(two.PNG, again.PNG) {
		t.Error("two exports at scale 2 differ")
	}
	if !bytes.Equal(one.JSON, two.JSON) {
		t.Error("the geometry changes with the scale")
	}
}

// exampleViews resolves every view of every example project, keyed by golden file name: the
// example's name, followed by "-<view>" for views other than main.
func exampleViews(t *testing.T) map[string]*view.Resolved {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "examples", "*"))
	if err != nil {
		t.Fatal(err)
	}
	views := map[string]*view.Resolved{}
	for _, dir := range dirs {
		p, diags, err := project.Load(dir)
		if err != nil || diags.HasErrors() {
			t.Fatalf("load %s: %v %v", dir, err, diags)
		}
		for id := range p.Views {
			v, diags := p.Resolve(id)
			if diags.HasErrors() {
				t.Fatalf("resolve %s view %s: %v", dir, id, diags)
			}
			name := filepath.Base(dir)
			if id != view.DefaultView {
				name += "-" + id
			}
			views[name] = v
		}
	}
	return views
}

// TestAnimation exports the animated view of examples/c4-container as a GIF: byte-identical
// across runs, one half-second period in 10 frames of 50 milliseconds looping forever, each frame
// differing from the one before, and the drawing at the end of the period, past the last frame,
// equal to the first frame, so the loop back to the first frame is one more equal step.
func TestAnimation(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	p, diags, err := project.Load(filepath.Join("..", "..", "examples", "c4-container"))
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v, diags := p.Resolve("sign-in")
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	first, err := Render(context.Background(), v, "gif")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(context.Background(), v, "gif")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.GIF, second.GIF) {
		t.Fatal("two GIF exports of the same view differ")
	}
	g, err := gif.DecodeAll(bytes.NewReader(first.GIF))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != GIFFrames || g.LoopCount != 0 {
		t.Fatalf("%d frames looping %d times, want %d looping forever (0)", len(g.Image), g.LoopCount, GIFFrames)
	}
	shown := make([][]byte, len(g.Image))
	canvas := image.NewRGBA(g.Image[0].Bounds())
	for i, img := range g.Image {
		if g.Delay[i] != 5 {
			t.Errorf("frame %d shown for %d hundredths of a second, want 5", i, g.Delay[i])
		}
		draw.Draw(canvas, img.Bounds(), img, img.Bounds().Min, draw.Over)
		shown[i] = slices.Clone(canvas.Pix)
	}
	for i := range shown {
		if bytes.Equal(shown[i], shown[(i+GIFFrames-1)%GIFFrames]) {
			t.Errorf("frame %d looks like the frame before it", i)
		}
	}

	period := fmt.Sprintf(" %dms linear infinite;", (GIFFrames * GIFDelay).Milliseconds())
	if !bytes.Contains(first.SVG, []byte(period)) {
		t.Errorf("the SVG's animation period is not the GIF's %d frames of %s:\n%s", GIFFrames, GIFDelay, first.SVG)
	}

	ctx, stop, err := openApp(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	viewJSON, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void]("window.dg.render("+string(viewJSON)+").then(() => {})", chromedp.EvalAwaitPromise)); err != nil {
		t.Fatal(err)
	}
	start, err := capture(ctx, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	// One iteration held at its end shows the keyframes' last offset rather than wrapping to the
	// next iteration's first.
	stopAtEnd := "document.getAnimations().forEach((a) => a.effect.updateTiming({ iterations: 1, fill: 'forwards' }))"
	if _, err := chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void](stopAtEnd)); err != nil {
		t.Fatal(err)
	}
	end, err := capture(ctx, GIFFrames*GIFDelay, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(start, end) {
		t.Error("the drawing at the end of the period differs from the drawing at time 0")
	}
	before, err := capture(ctx, (GIFFrames-1)*GIFDelay, 1)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(start, before) {
		t.Error("the drawing one frame before the end of the period equals the drawing at time 0")
	}
}

// TestBrokenPicture checks that a picture the browser cannot draw fails the export naming its
// node; dg check refuses such files, so the view is changed after resolving.
func TestBrokenPicture(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	p, diags, err := project.Load(filepath.Join("..", "..", "examples", "hello"))
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v, diags := p.Resolve(view.DefaultView)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	v.Nodes[0].Style.Icon = `<svg xmlns="http://www.w3.org/2000/svg"><rect`
	_, err = Render(context.Background(), v, "png")
	if want := fmt.Sprintf("the picture of node %q did not load", v.Nodes[0].ID); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("export error %v, want one containing %q", err, want)
	}
}

func TestNoBrowser(t *testing.T) {
	_, _, err := openApp(context.Background(), chromedp.ExecPath("dg-no-such-browser"))
	if !errors.Is(err, errNoBrowser) {
		t.Errorf("export error %v, want %v", err, errNoBrowser)
	}
}

func TestAppIsOffline(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	ctx, stop, err := openApp(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	for _, url := range []string{"http://example.com/", "http://1.1.1.1/", "https://fonts.gstatic.com/"} {
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](
			`fetch("`+url+`").then(() => true, () => false)`, chromedp.EvalAwaitPromise))
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Errorf("the app fetched %s", url)
		}
	}
	if err := chromedp.Do(ctx, chromedp.Navigate("http://example.com/")); err == nil {
		t.Error("navigating to example.com succeeded; host names must not resolve")
	}
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the output:\n%s", path, got)
	}
}

// TestProjectFont exports a project whose font is a copy of the bundled one under another name:
// the drawing has the same geometry and names the project font.
func TestProjectFont(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	font, err := os.ReadFile(filepath.Join("..", "..", "web", "node_modules", "@fontsource", "inter", "files", "inter-latin-400-normal.woff2"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"model.hcl":         "node \"a\" {\n  label = \"Measured with the project font\"\n}\n",
		"dg.hcl":            "project \"p\" {\n  font = \"fonts/Brand.woff2\"\n}\n",
		"fonts/Brand.woff2": string(font),
	}
	for name, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	render := func() *Result {
		p, diags, err := project.Load(dir)
		if err != nil || diags.HasErrors() {
			t.Fatal(err, diags)
		}
		v, diags := p.Resolve(view.DefaultView)
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		res, err := Render(context.Background(), v, "svg")
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	withFont := render()
	if err := os.Remove(filepath.Join(dir, "dg.hcl")); err != nil {
		t.Fatal(err)
	}
	bundled := render()
	if !bytes.Contains(withFont.SVG, []byte(`font-family="Brand, sans-serif"`)) {
		t.Errorf("the SVG does not name the project font:\n%s", withFont.SVG)
	}
	if !bytes.Equal(withFont.JSON, bundled.JSON) {
		t.Errorf("geometry with the project font differs from the bundled font's:\n%s\n%s", withFont.JSON, bundled.JSON)
	}
}

// TestLayoutRules exports a layout with a container pinned too small for its child (spec 7.2
// rule 5) and a node stored relative to the canvas whose frame is now a container, which is drawn
// where it was stored (spec 7.1).
func TestLayoutRules(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir := t.TempDir()
	files := map[string]string{
		"model.hcl": `node "g" {
  kind = "group"
}

node "a" {
  parent = node.g
}

node "moved" {
  parent = node.g
}
`,
		"views/main.layout.yaml": `version: 1
place:
  a: {in: g, x: 0, y: 0}
  g: {x: 100, y: 100, w: 120, h: 60}
  moved: {x: 300, y: 400}
`,
	}
	for name, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, diags, err := project.Load(dir)
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v, diags := p.Resolve(view.DefaultView)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	res, err := Render(context.Background(), v, "svg")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TooSmall) != 1 || res.TooSmall[0] != "g" {
		t.Errorf("too small containers %v, want [g]", res.TooSmall)
	}
	var geo struct {
		Nodes []struct {
			ID   string
			X, Y int
		}
	}
	if err := json.Unmarshal(res.JSON, &geo); err != nil {
		t.Fatal(err)
	}
	for _, n := range geo.Nodes {
		if n.ID == "moved" && (n.X != 300 || n.Y != 400) {
			t.Errorf("node in a stale frame drawn at %d,%d, want 300,400 where it was stored", n.X, n.Y)
		}
	}
}

// TestRowEnds checks that edges ending at rows attach at the end of the row's line that faces
// the other endpoint (spec 5.4): the side the other box lies on, or, when the boxes overlap
// horizontally, the side where their edges are closer.
func TestRowEnds(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir := t.TempDir()
	var model strings.Builder
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		fmt.Fprintf(&model, "node %q {\n  kind = \"table\"\n  row \"id\" {}\n  row \"fk\" {}\n}\n\n", id)
	}
	for _, pair := range [][2]string{{"a", "b"}, {"c", "d"}, {"e", "f"}} {
		fmt.Fprintf(&model, "edge %q {\n  from = node.%s.row.fk\n  to   = node.%s.row.id\n}\n\n", pair[0]+pair[1], pair[0], pair[1])
	}
	files := map[string]string{
		"model.hcl": model.String(),
		// a and b side by side; d overlaps c horizontally and reaches further right; f overlaps e and
		// reaches further right.
		"views/main.layout.yaml": `version: 1
place:
  a: {x: 0, y: 0, w: 100}
  b: {x: 300, y: 0, w: 100}
  c: {x: 0, y: 200, w: 100}
  d: {x: 40, y: 400, w: 160}
  e: {x: 600, y: 0, w: 200}
  f: {x: 750, y: 200, w: 100}
`,
	}
	for name, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ends := rowEnds(t, dir)
	want := map[string][2]int{"ab": {100, 300}, "cd": {0, 40}, "ef": {800, 850}}
	for id, w := range want {
		if got := [2]int{ends[id][0].x, ends[id][1].x}; got != w {
			t.Errorf("edge %s attaches at x %v, want %v", id, got, w)
		}
		for _, end := range ends[id] {
			if end.y != end.rowY {
				t.Errorf("edge %s attaches at y %d, want the row's line at %d", id, end.y, end.rowY)
			}
		}
	}

	er := rowEnds(t, filepath.Join("..", "..", "examples", "er"))
	wantER := map[string][2]string{
		"address-customer": {"right", "left"},
		"category-parent":  {"right", "right"},
		"item-order":       {"right", "left"},
		"item-product":     {"right", "left"},
		"order-bill-to":    {"right", "left"},
		"order-customer":   {"right", "left"},
		"order-ship-to":    {"right", "left"},
		"product-category": {"right", "left"},
	}
	if len(er) != len(wantER) {
		t.Errorf("examples/er has %d edges ending at rows, want %d", len(er), len(wantER))
	}
	for id, w := range wantER {
		if got := [2]string{er[id][0].side, er[id][1].side}; got != w {
			t.Errorf("examples/er edge %s attaches at %v, want %v", id, got, w)
		}
		for _, end := range er[id] {
			// Ends with different markers at one row end are spread 8 pixels each way.
			if abs(end.y-end.rowY) > 8 {
				t.Errorf("examples/er edge %s attaches at y %d, away from its row's line at %d", id, end.y, end.rowY)
			}
		}
	}
}

type rowEnd struct {
	x, y, rowY int
	side       string
}

// rowEnds exports the main view of the project in dir and returns the start and end of every
// edge whose two ends are rows.
func rowEnds(t *testing.T, dir string) map[string][2]rowEnd {
	t.Helper()
	p, diags, err := project.Load(dir)
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v, diags := p.Resolve(view.DefaultView)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	res, err := Render(context.Background(), v, "svg")
	if err != nil {
		t.Fatal(err)
	}
	var geo struct {
		Nodes []struct {
			ID   string
			X, W int
			Rows []struct {
				Name string
				Y    int
			}
		}
		Edges []struct {
			ID     string
			Points [][2]int
		}
	}
	if err := json.Unmarshal(res.JSON, &geo); err != nil {
		t.Fatal(err)
	}
	type node struct {
		x, w int
		rows map[string]int
	}
	nodes := map[string]node{}
	for _, n := range geo.Nodes {
		rows := map[string]int{}
		for _, r := range n.Rows {
			rows[r.Name] = r.Y
		}
		nodes[n.ID] = node{n.X, n.W, rows}
	}
	routes := map[string][][2]int{}
	for _, e := range geo.Edges {
		routes[e.ID] = e.Points
	}
	out := map[string][2]rowEnd{}
	for _, e := range v.Edges {
		if e.From.Row == "" || e.To.Row == "" {
			continue
		}
		pts := routes[e.ID]
		var pair [2]rowEnd
		for i, end := range []struct {
			at  [2]int
			ref view.End
		}{{pts[0], e.From}, {pts[len(pts)-1], e.To}} {
			n := nodes[end.ref.Node]
			side := map[int]string{n.x: "left", n.x + n.w: "right"}[end.at[0]]
			pair[i] = rowEnd{x: end.at[0], y: end.at[1], rowY: n.rows[end.ref.Row], side: side}
		}
		out[e.ID] = pair
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestOfPolicy exports a view whose of is a stack and which includes a node from elsewhere: the
// stack's children are stacked in one box, marked ordered_in it, and the other node is placed
// beside the box, free.
func TestOfPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chromium")
	}
	dir := t.TempDir()
	files := map[string]string{
		"model.hcl": "node \"vs\" {\n  kind = \"lane-set\"\n}\n\nnode \"v1\" {\n  kind   = \"lane\"\n  parent = node.vs\n}\n\nnode \"v2\" {\n  kind   = \"lane\"\n  parent = node.vs\n}\n\nnode \"h1\" {}\n\nedge \"v1-h1\" {\n  from = node.v1\n  to   = node.h1\n}\n",
		"views.hcl": "view \"extra\" {\n  of      = node.vs\n  include = [descendants.vs, node.h1]\n}\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, diags, err := project.Load(dir)
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	v, diags := p.Resolve("extra")
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	res, err := Render(context.Background(), v, "svg")
	if err != nil {
		t.Fatal(err)
	}
	var geo struct {
		Nodes []struct {
			ID         string
			X, Y, W, H int
			OrderedIn  string `json:"ordered_in"`
		}
	}
	if err := json.Unmarshal(res.JSON, &geo); err != nil {
		t.Fatal(err)
	}
	n := map[string]int{}
	for i, g := range geo.Nodes {
		n[g.ID] = i
	}
	v1, v2, h1 := geo.Nodes[n["v1"]], geo.Nodes[n["v2"]], geo.Nodes[n["h1"]]
	if v1.OrderedIn != "vs" || v2.OrderedIn != "vs" || h1.OrderedIn != "" {
		t.Errorf("ordered_in v1 %q, v2 %q, h1 %q; want vs, vs and none", v1.OrderedIn, v2.OrderedIn, h1.OrderedIn)
	}
	if v2.X != v1.X || v2.Y != v1.Y+v1.H || v2.W != v1.W {
		t.Errorf("v1 %+v and v2 %+v are not stacked", v1, v2)
	}
	for _, l := range []struct{ X, Y, W, H int }{{v1.X, v1.Y, v1.W, v1.H}, {v2.X, v2.Y, v2.W, v2.H}} {
		if h1.X < l.X+l.W && l.X < h1.X+h1.W && h1.Y < l.Y+l.H && l.Y < h1.Y+h1.H {
			t.Errorf("h1 %+v overlaps a lane %+v", h1, l)
		}
	}
}

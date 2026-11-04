package server

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// modelFiles is a project for the model editing tests: the editor tests' model with a table, a
// node of a project kind with properties and a node in a read-only file, all pinned.
var modelFiles = map[string]string{
	"dg.hcl": "project \"edit\" {\n  read_only = [\"generated.hcl\"]\n}\n",
	"spec.hcl": `kind "server" {
  extends = "service"
  property "replicas" {
    type = number
  }
  property "public" {
    type = bool
  }
}
`,
	"model.hcl": editorModel + `
node "t" {
  kind  = "table"
  label = "Orders"
  row "id" {
    type = "uuid"
    key  = "pk"
  }
  row "total" {}
}

node "srv" {
  kind  = "server"
  label = "Server"
}
`,
	"generated.hcl": "node \"ext\" {\n  label = \"External\"\n}\n",
	"views/main.layout.yaml": `version: 1
place:
  a: {x: 0, y: 0}
  b: {x: 200, y: 0}
  c: {x: 400, y: 0}
  d: {in: g, x: 0, y: 0}
  e: {in: g, x: 140, y: 0}
  ext: {x: 500, y: 400}
  g: {x: 0, y: 150, w: 300, h: 160}
  lane: {x: 400, y: 150}
  srv: {x: 250, y: 400}
  t: {x: 0, y: 400}
`,
}

// pointAt returns an action that runs f at the page point a fraction (fx, fy) across the box of
// the element sel, moved by (dx, dy).
func pointAt(sel string, fx, fy, dx, dy float64, f func(x, y float64) chromedp.Action[chromedp.Void]) chromedp.Action[chromedp.Void] {
	return func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
		r, err := chromedp.Evaluate[[]float64](`(() => { const r = document.querySelector(`+jsString(sel)+`).getBoundingClientRect(); return [r.x, r.y, r.width, r.height]; })()`)(ctx, t)
		if err != nil {
			return chromedp.Void{}, fmt.Errorf("%s: %w", sel, err)
		}
		return f(r[0]+r[2]*fx+dx, r[1]+r[3]*fy+dy)(ctx, t)
	}
}

// dragFrom presses at (x, y), moves to (x+dx, y+dy) in steps with the modifiers mods held, and
// releases there.
func dragFrom(x, y, dx, dy float64, mods ...chromedp.Modifier) chromedp.Action[chromedp.Void] {
	m := chromedp.ButtonModifiers(mods...)
	actions := []chromedp.Action[chromedp.Void]{chromedp.MouseEvent(chromedp.MouseMoved, x, y, m), chromedp.MouseEvent(chromedp.MousePressed, x, y, chromedp.ButtonLeft, chromedp.ClickCount(1), m)}
	for i := 1; i <= 8; i++ {
		f := float64(i) / 8
		actions = append(actions, chromedp.MouseEvent(chromedp.MouseMoved, x+dx*f, y+dy*f, chromedp.ButtonLeft, m), chromedp.Sleep(16*time.Millisecond))
	}
	return chromedp.Steps(append(actions, chromedp.MouseEvent(chromedp.MouseReleased, x+dx, y+dy, chromedp.ButtonLeft, chromedp.ClickCount(1), m))...)
}

// dragOnto drags from the center of from to the point a fraction (fx, fy) across to, moved by
// (dx, dy), with the modifiers mods held.
func dragOnto(from, to string, fx, fy, dx, dy float64, mods ...chromedp.Modifier) chromedp.Action[chromedp.Void] {
	return at(from, func(x0, y0 float64) chromedp.Action[chromedp.Void] {
		return pointAt(to, fx, fy, dx, dy, func(x1, y1 float64) chromedp.Action[chromedp.Void] { return dragFrom(x0, y0, x1-x0, y1-y0, mods...) })
	})
}

// js runs a script in the page.
func js(src string) chromedp.Action[chromedp.Void] {
	return chromedp.Evaluate[chromedp.Void](src)
}

// menu opens the context menu on sel and chooses item.
func menu(sel, item string) chromedp.Action[chromedp.Void] {
	return chromedp.Steps(
		click(sel, chromedp.ButtonRight),
		chromedp.Poll[chromedp.Void](`!document.getElementById('context-menu').hidden`),
		js(`(() => { const items = [...document.querySelectorAll('#context-menu button')]; const b = items.find((b) => b.querySelector('.text').textContent === `+jsString(item)+`); if (!b) throw new Error('the menu holds ' + items.map((b) => b.innerText).join(', ')); b.click(); })()`),
	)
}

// fill sets the fields of the open dialog and submits it.
func fill(fields map[string]string) chromedp.Action[chromedp.Void] {
	var b strings.Builder
	for k, v := range fields {
		fmt.Fprintf(&b, "d.querySelector('[name=%q]').value = %s;", k, jsString(v))
	}
	return chromedp.Steps(
		chromedp.Poll[chromedp.Void](`document.getElementById('dialog').open`),
		js(`(() => { const d = document.getElementById('dialog'); `+b.String()+` d.querySelector('button[value="ok"]').click(); })()`),
		sentOp(),
	)
}

// pick types query into the open choice dialog and picks the choice value.
func pick(query, value string) chromedp.Action[chromedp.Void] {
	return chromedp.Steps(
		chromedp.Poll[chromedp.Void](`document.getElementById('dialog').open`),
		chromedp.KeyEvent(query),
		chromedp.Poll[chromedp.Void](`document.querySelector('#dialog [data-value=`+jsString(value)+`]') !== null`),
		chromedp.Click(chromedp.CSS(`#dialog [data-value=`+jsString(value)+`]`)),
		sentOp(),
	)
}

// sentOp waits until the page sent an operation, which a dialog sends once it closed.
func sentOp() chromedp.Action[chromedp.Void] {
	return chromedp.Poll[chromedp.Void](`window.sent.some((m) => m.type === 'op')`)
}

// inspect sets the inspector's control sel to value, as a change.
func inspect(sel, value string) chromedp.Action[chromedp.Void] {
	return js(`(() => { const c = document.querySelector('#inspector ' + ` + jsString(sel) + `); c.value = ` + jsString(value) + `; c.dispatchEvent(new Event('change')); })()`)
}

// edit waits for the label editor, types text and presses Enter.
func edit(text string) chromedp.Action[chromedp.Void] {
	return chromedp.Steps(chromedp.Poll[chromedp.Void](`document.activeElement?.classList.contains('dg-inline')`), chromedp.KeyEvent(text), chromedp.KeyEvent("\r"))
}

// escape closes the label editor a created node opens.
func escape() chromedp.Action[chromedp.Void] {
	return chromedp.Steps(chromedp.Poll[chromedp.Void](`document.activeElement?.classList.contains('dg-inline')`), chromedp.KeyEvent(kbEscape))
}

func dblclick(sel string) chromedp.Action[chromedp.Void] {
	return at(sel, func(x, y float64) chromedp.Action[chromedp.Void] {
		return chromedp.MouseClickXY(x, y, chromedp.ClickCount(2))
	})
}

// TestEditorModel drives every model editing interaction of FR-11 in Chromium: each sends one
// operation or batch, which writes the files asserted and is one undo step: undo restores the
// files byte for byte, and redo the files the interaction wrote.
func TestEditorModel(t *testing.T) {
	e := newEditor(t, modelFiles)
	ctx := e.tab()
	ctx, cancel := context.WithTimeout(ctx, 240*time.Second)
	defer cancel()
	cmd := chromedp.KeyModifiers(chromedp.ModifierCtrl)
	alt := chromedp.ModifierAlt
	type files = map[string][]string
	for i, g := range []struct {
		name    string
		actions []chromedp.Action[chromedp.Void]
		op      string
		want    files // lines each file holds after
		lacks   files // and does not
	}{
		{"palette onto the canvas", []chromedp.Action[chromedp.Void]{dragOnto(`[data-kind="service"]`, `[data-node="c"]`, 1, 0.5, 300, 0), escape()}, "create_node",
			files{"model.hcl": {"node \"service\" {\n  kind = \"service\"\n}"}, "views/main.layout.yaml": {"\n  service: {x: "}}, nil},
		{"first label gives the ID", []chromedp.Action[chromedp.Void]{dblclick(`[data-node="service"]`), edit("Billing API")}, "set_label",
			files{"model.hcl": {"node \"billing-api\" {\n  kind  = \"service\"\n  label = \"Billing API\"\n}"}, "views/main.layout.yaml": {"\n  billing-api: {x: "}}, files{"model.hcl": {"node \"service\""}}},
		{"palette into a grid", []chromedp.Action[chromedp.Void]{dragOnto(`[data-kind="node"]`, `[data-node="s2"]`, 0.1, 0.5, 0, 0), escape()}, "create_node",
			files{"model.hcl": {"node \"node-2\" {\n  parent = node.lane\n}"}, "views/main.layout.yaml": {"order: [s1, node-2, s2, s3]"}}, nil},
		{"label in place", []chromedp.Action[chromedp.Void]{dblclick(`[data-node="b"]`), edit("Bravo")}, "set_label",
			files{"model.hcl": {"label = \"Bravo\""}}, nil},
		{"label in this view", []chromedp.Action[chromedp.Void]{click(`[data-node="b"]`), chromedp.Poll[chromedp.Void](`document.querySelector('#inspector .id')?.textContent === 'b'`), inspect(`textarea[placeholder]`, "Here")}, "set_view_label",
			files{"views.hcl": {"node \"b\" {\n    label = \"Here\"\n  }"}}, nil},
		{"view label in place", []chromedp.Action[chromedp.Void]{chromedp.Poll[chromedp.Void](`document.querySelector('[data-node="b"] text')?.textContent === 'Here'`), dblclick(`[data-node="b"]`), edit("There")}, "set_view_label",
			files{"views.hcl": {"label = \"There\""}, "model.hcl": {"label = \"Bravo\""}}, nil},
		{"label in this view emptied", []chromedp.Action[chromedp.Void]{click(`[data-node="b"]`), chromedp.Poll[chromedp.Void](`document.querySelector('#inspector textarea[placeholder]')?.value === 'There'`), inspect(`textarea[placeholder]`, "")}, "set_view_label",
			nil, files{"views.hcl": {"node \"b\""}}},
		{"edge label in place", []chromedp.Action[chromedp.Void]{dblclick(`[data-edge="ab"] text`), edit("invokes")}, "set_edge_labels",
			files{"model.hcl": {"label = \"invokes\""}}, files{"model.hcl": {"\"calls\""}}},
		{"connect two nodes", []chromedp.Action[chromedp.Void]{chromedp.Click(chromedp.CSS(`#connect`)), toolMenu("edge-kind", "async"), dragOnto(`[data-node="a"]`, `[data-node="c"]`, 0.5, 0.5, 0, 0)}, "connect",
			files{"model.hcl": {"edge \"a-c\" {\n  from = node.a\n  to   = node.c\n  kind = \"async\"\n}"}}, nil},
		{"connect from a port", []chromedp.Action[chromedp.Void]{toolMenu("edge-kind", "default"), pointAt(`[data-node="c"]`, 0.5, 0, 0, 0, func(x, y float64) chromedp.Action[chromedp.Void] {
			return pointAt(`[data-node="b"]`, 0.5, 0.5, 0, 0, func(x1, y1 float64) chromedp.Action[chromedp.Void] { return dragFrom(x, y, x1-x, y1-y) })
		})}, "connect", files{"model.hcl": {"edge \"c-b\" {\n  from = node.c.port.in\n  to   = node.b\n}"}}, nil},
		{"connect from a row", []chromedp.Action[chromedp.Void]{rowText("t", "total", func(x, y float64) chromedp.Action[chromedp.Void] {
			return pointAt(`[data-node="srv"]`, 0.5, 0.5, 0, 0, func(x1, y1 float64) chromedp.Action[chromedp.Void] { return dragFrom(x, y, x1-x, y1-y) })
		})}, "connect", files{"model.hcl": {"edge \"t-srv\" {\n  from = node.t.row.total\n  to   = node.srv\n}"}}, nil},
		{"rows in the inspector", []chromedp.Action[chromedp.Void]{chromedp.Click(chromedp.CSS(`#connect`)), click(`[data-node="t"]`), chromedp.Poll[chromedp.Void](`document.querySelector('#inspector .id')?.textContent === 't'`), js(`(() => { const r = document.querySelectorAll('#inspector input[aria-label="Row name"]')[0]; r.value = 'order_id'; r.dispatchEvent(new Event('input')); [...document.querySelectorAll('#inspector button')].find((b) => b.textContent === 'Apply rows').click(); })()`)}, "set_rows",
			files{"model.hcl": {"row \"order_id\" {\n    type = \"uuid\"\n    key  = \"pk\"\n  }"}}, nil},
		{"kind in the inspector", []chromedp.Action[chromedp.Void]{click(`[data-node="b"]`), chromedp.Poll[chromedp.Void](`document.querySelector('#inspector .id')?.textContent === 'b'`), inspect(`select`, "database")}, "set_kind",
			files{"model.hcl": {"kind  = \"database\""}}, nil},
		{"tags in the inspector", []chromedp.Action[chromedp.Void]{inspect(`input[list="dg-tags"]`, "hot new")}, "set_tags",
			files{"model.hcl": {"tags  = [\"hot\", \"new\"]"}}, nil},
		{"properties typed by the kind", []chromedp.Action[chromedp.Void]{click(`[data-node="srv"]`), chromedp.Poll[chromedp.Void](`document.querySelector('#inspector .id')?.textContent === 'srv'`), inspect(`input[type="number"]`, "3")}, "set_props",
			files{"model.hcl": {"props = {\n    replicas = 3\n  }"}}, nil},
		{"copy and paste", []chromedp.Action[chromedp.Void]{click(`[data-node="a"]`), click(`[data-node="b"]`, chromedp.ButtonModifiers(chromedp.ModifierShift)), chromedp.KeyEvent("c", cmd),
			pointAt(`[data-node="ext"]`, 1, 0, 120, 0, func(x, y float64) chromedp.Action[chromedp.Void] {
				return chromedp.MouseEvent(chromedp.MouseMoved, x, y)
			}), chromedp.KeyEvent("v", cmd)}, "batch",
			files{"model.hcl": {"node \"alpha\" {", "node \"bravo\" {", "edge \"alpha-bravo\" {\n  from  = node.alpha\n  to    = node.bravo\n  label = \"invokes\"\n}"}, "views/main.layout.yaml": {"\n  alpha: {x: ", "\n  bravo: {x: "}}, nil},
		{"hide from view", []chromedp.Action[chromedp.Void]{menu(`[data-node="c"]`, "Hide from view")}, "hide_node",
			files{"views.hcl": {"view \"main\" {\n  include = [all]\n  exclude = [node.c]\n}"}}, nil},
		{"add a node of the model", []chromedp.Action[chromedp.Void]{chromedp.Click(chromedp.CSS(`#add-node`)), pick("Gam", "c")}, "show_node",
			files{"views.hcl": {"include = [all]"}}, files{"views.hcl": {"exclude"}}},
		{"rename an ID", []chromedp.Action[chromedp.Void]{menu(`[data-node="d"]`, "Rename ID…"), fill(map[string]string{"id": "delta"})}, "rename_id",
			files{"model.hcl": {"node \"delta\" {", "to   = node.delta"}, "views/main.layout.yaml": {"\n  delta: {in: g, "}}, nil},
		{"add a port", []chromedp.Action[chromedp.Void]{menu(`[data-node="a"]`, "Add port…"), fill(map[string]string{"name": "out", "side": "right"})}, "add_port",
			files{"model.hcl": {"port \"out\" {\n    side = \"right\"\n  }"}}, nil},
		{"re-parent with Alt", []chromedp.Action[chromedp.Void]{dragOnto(`[data-node="b"]`, `[data-node="g"]`, 0.5, 0.85, 0, 0, alt), chromedp.Poll[chromedp.Void](`!document.getElementById('toast').hidden`)}, "batch",
			files{"model.hcl": {"parent = node.g\n}\n\nnode \"c\""}, "views/main.layout.yaml": {"\n  b: {in: g, "}}, nil},
		{"re-parent onto the canvas", []chromedp.Action[chromedp.Void]{dragOnto(`[data-node="e"]`, `[data-node="srv"]`, 1, 0, 160, -40, alt)}, "batch",
			files{"views/main.layout.yaml": {"\n  e: {x: "}}, files{"model.hcl": {"label  = \"Epsilon\"\n  parent"}}},
		{"re-parent into a grid", []chromedp.Action[chromedp.Void]{dragOnto(`[data-node="a"]`, `[data-node="s1"]`, 0.1, 0.5, 0, 0, alt)}, "batch",
			files{"model.hcl": {"parent = node.lane"}, "views/main.layout.yaml": {"order: [a, s1, node-2, s2, s3]"}}, files{"views/main.layout.yaml": {"\n  a: {x:"}}},
		{"move a read-only node", []chromedp.Action[chromedp.Void]{dragBy(`[data-node="ext"]`, 40, 0)}, "move",
			files{"views/main.layout.yaml": {"\n  ext: {x: 540, y: 400}"}}, nil},
		{"delete from the model", []chromedp.Action[chromedp.Void]{click(`[data-node="srv"]`), chromedp.KeyEvent(kb.Delete)}, "delete_node",
			files{}, files{"model.hcl": {"node \"srv\"", "edge \"t-srv\""}, "views/main.layout.yaml": {"\n  srv:"}}},
		{"add a decoration", []chromedp.Action[chromedp.Void]{chromedp.KeyEvent(kbEscape), toolMenu("add-decoration", "Text"), fill(map[string]string{"text": "Note"})}, "add_decoration",
			files{"views/main.layout.yaml": {"decorations:\n  - {id: text, type: text, x: "}}, nil},
		{"promote a decoration", []chromedp.Action[chromedp.Void]{menu(`[data-decoration="text"] text`, "Promote to node…"), fill(map[string]string{"kind": "note"})}, "promote_decoration",
			files{"model.hcl": {"node \"note\" {\n  kind  = \"note\"\n  label = \"Note\"\n}"}, "views/main.layout.yaml": {"\n  note: {x: "}}, files{"views/main.layout.yaml": {"decorations:"}}},
		{"decoration attached to the selected node", []chromedp.Action[chromedp.Void]{click(`[data-node="t"]`), toolMenu("add-decoration", "Rectangle")}, "add_decoration",
			files{"views/main.layout.yaml": {"{id: rect, type: rect, attach: t, dx: 0, dy: "}}, nil},
		{"detach a decoration", []chromedp.Action[chromedp.Void]{menu(`[data-decoration="rect"] rect`, "Detach from node")}, "update_decoration",
			files{"views/main.layout.yaml": {"{id: rect, type: rect, x: "}}, files{"views/main.layout.yaml": {"attach"}}},
		{"attach a decoration", []chromedp.Action[chromedp.Void]{menu(`[data-decoration="rect"] rect`, "Attach to node…"), pick("ex", "ext")}, "update_decoration",
			files{"views/main.layout.yaml": {"{id: rect, type: rect, attach: ext, dx: "}}, nil},
		{"new view", []chromedp.Action[chromedp.Void]{chromedp.Click(chromedp.CSS(`#new-view`)), fill(map[string]string{"title": "Orders", "include": "node.t node.note", "overlays": "hot: node.t | Hot"}), chromedp.Poll[chromedp.Void](`document.getElementById('views').value === 'orders'`)}, "create_view",
			files{"views.hcl": {"view \"orders\" {\n  title   = \"Orders\"\n  include = [node.t, node.note]\n  overlay \"hot\" {\n    members = [node.t]\n    label   = \"Hot\"\n  }\n}"}}, nil},
	} {
		before := tree(t, e.dir)
		if err := chromedp.Do(ctx, append(g.actions, e.idle())...); err != nil {
			t.Fatalf("%s: %v", g.name, err)
		}
		e.shot(ctx, fmt.Sprintf("m%02d-%s", i, strings.ReplaceAll(g.name, " ", "-")))
		reqs := sent(ctx, t)
		if len(reqs) != 1 || reqs[0]["type"] != "op" || reqs[0]["op"].(map[string]any)["op"] != g.op {
			t.Fatalf("%s sent %v, want one %s", g.name, reqs, g.op)
		}
		after := tree(t, e.dir)
		for name, lines := range g.want {
			for _, w := range lines {
				if !strings.Contains(after[name], w) {
					t.Errorf("%s: %s lacks %q:\n%s", g.name, name, w, after[name])
				}
			}
		}
		for name, lines := range g.lacks {
			for _, w := range lines {
				if strings.Contains(after[name], w) {
					t.Errorf("%s: %s holds %q:\n%s", g.name, name, w, after[name])
				}
			}
		}
		if err := chromedp.Do(ctx, chromedp.KeyEvent("z", cmd), e.idle()); err != nil {
			t.Fatal(err)
		}
		if got := tree(t, e.dir); !reflect.DeepEqual(got, before) {
			t.Fatalf("%s: one undo left %v, want %v", g.name, got, before)
		}
		if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.getElementById('redo').ariaDisabled === 'false'`), chromedp.Click(chromedp.CSS(`#redo`)), e.idle()); err != nil {
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

// rowText runs f at the center of the text of row name in table node.
func rowText(node, name string, f func(x, y float64) chromedp.Action[chromedp.Void]) chromedp.Action[chromedp.Void] {
	return func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
		r, err := chromedp.Evaluate[[]float64](`(() => { const el = [...document.querySelectorAll('[data-node=`+jsString(node)+`] text')].find((x) => x.textContent === `+jsString(name)+`); const r = el.getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; })()`)(ctx, t)
		if err != nil {
			return chromedp.Void{}, err
		}
		return f(r[0], r[1])(ctx, t)
	}
}

// TestEditorModelRefusals checks in Chromium that a node in a read-only file shows a lock and
// refuses model edits while it can still be moved, that the context menu works from the
// keyboard and opens a drill-down view, that the palette creates a node from the keyboard, and
// that the toast after re-parenting undoes it.
func TestEditorModelRefusals(t *testing.T) {
	e := newEditor(t, modelFiles)
	ctx := e.tab()
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	before := tree(t, e.dir)
	status := func(want string) chromedp.Action[chromedp.Void] {
		return chromedp.Poll[chromedp.Void](`document.getElementById('status').textContent.includes(` + jsString(want) + `)`)
	}
	for i, step := range [][]chromedp.Action[chromedp.Void]{
		{chromedp.Poll[chromedp.Void](`document.querySelector('[data-lock="ext"]') !== null && document.querySelector('[data-lock="a"]') === null`)},
		{dblclick(`[data-node="ext"]`), status("read-only")},
		{chromedp.KeyEvent(kb.Delete), status("cannot be deleted")},
		{dragOnto(`[data-node="ext"]`, `[data-node="g"]`, 0.5, 0.85, 0, 0, chromedp.ModifierAlt), status("its parent cannot change")},
		{click(`[data-node="ext"]`, chromedp.ButtonRight), chromedp.Poll[chromedp.Void](`!document.getElementById('context-menu').hidden`),
			chromedp.Poll[chromedp.Void](`(() => { const b = [...document.querySelectorAll('#context-menu button')]; const by = (t) => b.find((x) => x.querySelector('.text').textContent === t); return by('Delete from model').disabled && by('Rename ID…').disabled && !by('Hide from view').disabled && !by('Copy').disabled; })()`),
			chromedp.KeyEvent(kbEscape)},
		{chromedp.Poll[chromedp.Void](`document.querySelector('#inspector textarea')?.disabled && document.querySelector('#inspector .lock') !== null`), e.idle()},
	} {
		if err := chromedp.Do(ctx, step...); err != nil {
			t.Fatalf("read-only step %d: %v", i, err)
		}
	}
	if reqs := sent(ctx, t); len(reqs) != 0 {
		t.Errorf("model edits of a read-only node sent %v", reqs)
	}
	if got := tree(t, e.dir); !reflect.DeepEqual(got, before) {
		t.Errorf("model edits of a read-only node changed files")
	}

	// Shift+F10 opens the menu of the selection with the focus on its first item; arrow keys
	// move through it and Enter chooses one.
	err := chromedp.Do(ctx,
		click(`[data-node="c"]`),
		chromedp.KeyEvent(kb.F10, chromedp.KeyModifiers(chromedp.ModifierShift)),
		chromedp.Poll[chromedp.Void](`document.activeElement?.querySelector('.text')?.textContent === 'Edit label'`),
		chromedp.KeyEvent(kb.ArrowDown), chromedp.KeyEvent(kb.ArrowDown),
		chromedp.Poll[chromedp.Void](`document.activeElement?.querySelector('.text')?.textContent === 'Hide from view'`),
		chromedp.KeyEvent("\r"),
		e.idle(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if reqs := sent(ctx, t); len(reqs) != 1 || reqs[0]["op"].(map[string]any)["op"] != "hide_node" {
		t.Fatalf("the keyboard menu sent %v, want hide_node", reqs)
	}

	// A kind chosen in the palette by keyboard is created in the middle of the canvas, and its
	// label editor opens.
	err = chromedp.Do(ctx,
		chromedp.Focus(chromedp.CSS(`[data-kind="queue"]`)),
		chromedp.KeyEvent("\r"),
		edit("Jobs"),
		e.idle(),
		chromedp.Poll[chromedp.Void](`document.querySelector('[data-node="jobs"]') !== null`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if reqs := sent(ctx, t); len(reqs) != 2 || reqs[0]["op"].(map[string]any)["op"] != "create_node" || reqs[1]["op"].(map[string]any)["op"] != "set_label" {
		t.Fatalf("the palette sent %v", reqs)
	}

	// The toast after re-parenting undoes it.
	beforeMove := tree(t, e.dir)
	err = chromedp.Do(ctx,
		dragOnto(`[data-node="a"]`, `[data-node="g"]`, 0.5, 0.85, 0, 0, chromedp.ModifierAlt),
		e.idle(),
		chromedp.Poll[chromedp.Void](`!document.getElementById('toast').hidden`),
		chromedp.Click(chromedp.CSS(`#toast button`)),
		e.idle(),
		chromedp.Poll[chromedp.Void](`document.getElementById('toast').hidden`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := tree(t, e.dir); !reflect.DeepEqual(got, beforeMove) {
		t.Errorf("the toast's undo left %v", got)
	}

	// A later change takes the toast away, so its Undo never undoes another change.
	err = chromedp.Do(ctx,
		dragOnto(`[data-node="a"]`, `[data-node="g"]`, 0.5, 0.85, 0, 0, chromedp.ModifierAlt),
		e.idle(),
		chromedp.Poll[chromedp.Void](`!document.getElementById('toast').hidden`),
		dblclick(`[data-node="b"]`), edit("Bee"), e.idle(),
		chromedp.Poll[chromedp.Void](`document.getElementById('toast').hidden`),
	)
	if err != nil {
		t.Fatalf("the toast after another change: %v", err)
	}
}

// TestEditorDrillMenu checks that the context menu opens the views that show a node's inside.
func TestEditorDrillMenu(t *testing.T) {
	e := newEditor(t, map[string]string{
		"model.hcl": editorModel,
		"views.hcl": "view \"main\" {}\n\nview \"inside\" {\n  of = node.g\n}\n",
	})
	ctx := e.tab()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	err := chromedp.Do(ctx,
		menu(`[data-node="g"]`, "Open view inside"),
		chromedp.Poll[chromedp.Void](`document.getElementById('views').value === 'inside' && document.querySelector('[data-node="d"]') !== null && document.querySelector('[data-node="g"]') === null`),
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestEditorFromScratch builds a small diagram in an empty project with the editor alone:
// palette, labels, rows, connections, re-parenting, copy and paste, delete and undo.
func TestEditorFromScratch(t *testing.T) {
	e := newEditor(t, map[string]string{})
	ctx, _ := chromedp.NewContext(e.browser)
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	ready := chromedp.Poll[chromedp.Void](`document.body.classList.contains('editable')`, chromedp.WithPollingInterval(10*time.Millisecond))
	if err := chromedp.Do(ctx, chromedp.Navigate(e.url), ready); err != nil {
		t.Fatal(err)
	}
	// create drops kind at (x, y) in the canvas and labels the new node.
	create := func(kind string, x, y float64, label string) chromedp.Action[chromedp.Void] {
		return chromedp.Steps(js(`document.querySelector('[data-kind=`+kind+`]').scrollIntoView({ block: 'nearest' })`), dragOnto(`[data-kind=`+jsString(kind)+`]`, `#canvas`, 0, 0, x, y), edit(label), ready)
	}
	steps := []struct {
		name    string
		actions []chromedp.Action[chromedp.Void]
	}{
		{"service", []chromedp.Action[chromedp.Void]{create("service", 140, 100, "API")}},
		{"database", []chromedp.Action[chromedp.Void]{create("database", 460, 100, "Orders DB")}},
		{"table", []chromedp.Action[chromedp.Void]{create("table", 460, 320, "Orders")}},
		{"rows", []chromedp.Action[chromedp.Void]{click(`[data-node="orders"]`), chromedp.Poll[chromedp.Void](`document.querySelector('#inspector .id')?.textContent === 'orders'`),
			js(`[...document.querySelectorAll('#inspector button')].find((b) => b.textContent === 'Add row').click()`),
			js(`[...document.querySelectorAll('#inspector button')].find((b) => b.textContent === 'Add row').click()`),
			js(`(() => { const r = document.querySelectorAll('#inspector input[aria-label="Row name"]'); r[0].value = 'id'; r[0].dispatchEvent(new Event('input')); r[1].value = 'total'; r[1].dispatchEvent(new Event('input')); const k = document.querySelectorAll('#inspector input[aria-label="Key"]')[0]; k.value = 'pk'; k.dispatchEvent(new Event('input')); [...document.querySelectorAll('#inspector button')].find((b) => b.textContent === 'Apply rows').click(); })()`),
			ready}},
		{"connect", []chromedp.Action[chromedp.Void]{chromedp.KeyEvent("c"), dragOnto(`[data-node="api"]`, `[data-node="orders-db"]`, 0.5, 0.5, 0, 0), ready,
			rowText("orders", "id", func(x, y float64) chromedp.Action[chromedp.Void] {
				return pointAt(`[data-node="orders-db"]`, 0.5, 0.5, 0, 0, func(x1, y1 float64) chromedp.Action[chromedp.Void] { return dragFrom(x1, y1, x-x1, y-y1) })
			}), ready, chromedp.KeyEvent(kbEscape)}},
		{"edge label", []chromedp.Action[chromedp.Void]{click(`[data-edge="api-orders-db"] path`), chromedp.KeyEvent(kb.F2), edit("SQL"), ready}},
		{"group", []chromedp.Action[chromedp.Void]{create("group", 100, 520, "Backend")}},
		{"re-parent", []chromedp.Action[chromedp.Void]{dragOnto(`[data-node="api"]`, `[data-node="backend"]`, 0.3, 0.6, 0, 0, chromedp.ModifierAlt), ready}},
		{"paste", []chromedp.Action[chromedp.Void]{click(`[data-node="api"]`), chromedp.KeyEvent("c", chromedp.KeyModifiers(chromedp.ModifierCtrl)), chromedp.KeyEvent("v", chromedp.KeyModifiers(chromedp.ModifierCtrl)), ready}},
		{"delete", []chromedp.Action[chromedp.Void]{click(`[data-node="api-2"]`), chromedp.KeyEvent(kb.Delete), ready}},
		{"undo", []chromedp.Action[chromedp.Void]{chromedp.KeyEvent("z", chromedp.KeyModifiers(chromedp.ModifierCtrl)), ready, chromedp.Poll[chromedp.Void](`document.querySelector('[data-node="api-2"]') !== null`)}},
	}
	for i, s := range steps {
		if err := chromedp.Do(ctx, s.actions...); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		e.shot(ctx, fmt.Sprintf("s%02d-%s", i, s.name))
	}
	model := tree(t, e.dir)["model.hcl"]
	for _, want := range []string{
		"node \"api\" {\n  kind   = \"service\"\n  label  = \"API\"\n  parent = node.backend\n}",
		"node \"orders-db\" {\n  kind  = \"database\"\n  label = \"Orders DB\"\n}",
		"node \"orders\" {\n  kind  = \"table\"\n  label = \"Orders\"\n  row \"id\" {\n    key = \"pk\"\n  }\n  row \"total\" {}\n}",
		"edge \"api-orders-db\" {\n  from  = node.api\n  to    = node.orders-db\n  label = \"SQL\"\n}",
		"edge \"orders-db-orders\" {\n  from = node.orders-db\n  to   = node.orders.row.id\n}",
		"node \"api-2\" {\n  kind   = \"service\"\n  label  = \"API\"\n  parent = node.backend\n}",
	} {
		if !strings.Contains(model, want) {
			t.Errorf("model.hcl lacks %q:\n%s", want, model)
		}
	}
}

// TestEditorKeyboard drives the editor from the keyboard alone: Enter on a toolbar button runs
// it rather than the canvas's keys, a dialog is filled by typing and submitted by Enter, a field
// of the inspector committed by Tab keeps the focus on the next field, and a narrow window keeps
// the toolbar in one row, what does not fit in its overflow menu.
func TestEditorKeyboard(t *testing.T) {
	e := newEditor(t, modelFiles)
	ctx := e.tab()
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	op := func(want string) {
		t.Helper()
		reqs := sent(ctx, t)
		if len(reqs) != 1 || reqs[0]["op"].(map[string]any)["op"] != want {
			t.Fatalf("sent %v, want one %s", reqs, want)
		}
	}
	err := chromedp.Do(ctx,
		click(`[data-node="a"]`),
		chromedp.Focus(chromedp.CSS(`#new-view`)),
		chromedp.KeyEvent("\r"),
		chromedp.Poll[chromedp.Void](`document.getElementById('dialog').open && document.activeElement?.name === 'title'`),
		chromedp.KeyEvent("Orders"),
		chromedp.KeyEvent("\r"),
		chromedp.Poll[chromedp.Void](`document.getElementById('views').value === 'orders'`),
		e.idle(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tree(t, e.dir)["views.hcl"], "view \"orders\" {\n  title   = \"Orders\"\n  include = [node.a]\n}") {
		t.Errorf("views.hcl:\n%s", tree(t, e.dir)["views.hcl"])
	}
	op("create_view")

	err = chromedp.Do(ctx,
		chromedp.SetValue(`#views`, "main"), js(`document.getElementById('views').dispatchEvent(new Event('change'))`),
		e.idle(), chromedp.Poll[chromedp.Void](`document.querySelector('[data-node="b"]') !== null`),
		click(`[data-node="b"]`),
		chromedp.KeyEvent(kb.F10, chromedp.KeyModifiers(chromedp.ModifierShift)),
		chromedp.Poll[chromedp.Void](`document.activeElement?.querySelector('.text')?.textContent === 'Edit label'`),
		chromedp.KeyEvent(kb.ArrowDown), chromedp.KeyEvent(kb.ArrowDown), chromedp.KeyEvent(kb.ArrowDown),
		chromedp.Poll[chromedp.Void](`document.activeElement?.querySelector('.text')?.textContent === 'Rename ID…'`),
		chromedp.KeyEvent("\r"),
		chromedp.Poll[chromedp.Void](`document.getElementById('dialog').open && document.activeElement?.name === 'id'`),
		chromedp.KeyEvent("x\r"),
		e.idle(), chromedp.Poll[chromedp.Void](`document.querySelector('[data-node="bx"]') !== null`),
	)
	if err != nil {
		t.Fatal(err)
	}
	op("rename_id")

	err = chromedp.Do(ctx,
		click(`[data-node="c"]`),
		chromedp.Poll[chromedp.Void](`document.querySelector('#inspector .id')?.textContent === 'c'`),
		chromedp.Focus(chromedp.CSS(`#inspector textarea`)),
		chromedp.KeyEvent("!"),
		chromedp.KeyEvent(kb.Tab),
		e.idle(),
		chromedp.Poll[chromedp.Void](`document.activeElement?.placeholder === 'Gamma!'`),
		chromedp.KeyEvent(kb.Tab),
		chromedp.Poll[chromedp.Void](`document.activeElement?.tagName === 'SELECT' && document.activeElement.closest('#inspector') !== null && document.activeElement.value === 'node'`),
	)
	if err != nil {
		t.Fatal(err)
	}
	op("set_label")
	if !strings.Contains(tree(t, e.dir)["model.hcl"], `label = "Gamma!"`) {
		t.Error("the label typed in the inspector was not written")
	}

	err = chromedp.Do(ctx,
		chromedp.EmulateViewport(700, 800),
		chromedp.Poll[chromedp.Void](`(() => { const b = document.getElementById('bar'); return b.scrollWidth <= b.clientWidth && !document.getElementById('more').hidden && [...b.querySelectorAll('button:not([hidden]), select')].every((c) => c.getBoundingClientRect().right <= 700); })()`),
		chromedp.Click(chromedp.CSS(`#more`)),
		chromedp.Poll[chromedp.Void](`document.activeElement?.closest('#overflow-menu') !== null && document.querySelector('#overflow-menu #help') !== null`),
		chromedp.KeyEvent(kbEscape),
		chromedp.Poll[chromedp.Void](`document.getElementById('overflow-menu').hidden && document.activeElement?.id === 'more'`),
	)
	if err != nil {
		t.Fatalf("the toolbar at 700 pixels: %v", err)
	}
}

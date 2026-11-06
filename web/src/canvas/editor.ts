import { dia } from '@joint/core';
import { scene, type Op, type Scene } from '../edit/edit';
import { clearOf, createOps, nodeSelector, type DecorationType } from '../edit/model';
import type { Point } from '../geometry/geometry';
import type { Connect, Diagnostic, ModelInfo, Result, ServerMessage, ViewInfo } from '../protocol/client';
import { connect } from '../protocol/client';
import type { ResolvedView } from '../protocol/view';
import { actions, type Selection } from './actions';
import { diagnosticList, diagnosticsBar } from './diagnostics';
import { h, sheet } from './dialog';
import { checkStyle, layerMarkup, layers, toPNG, toSVG, picture, type Drawing } from './export';
import { inPanel, openDiagnostic } from './host';
import { interact } from './interact';
import { arrangeItems, bindMenu, closeMenu, openerOf, showMenu, type Entry } from './menu';
import { inspector, palette } from './panels';
import { defsMarkup, flowCSS, flowPeriods, nodeMarkup, shiftNode } from './markup';
import { shortcutSheet } from './shortcuts';
import { tooltips } from './tip';
import { toolbar } from './toolbar';

const UNPINNED_OPACITY = 0.55;

const Markup = dia.Element.define('dg.Markup', {});
const namespace = { dg: { Markup } };

// WIDE is the narrowest window, in pixels, that shows the palette and the inspector at first.
const WIDE = 1000;
// MENU_GAP is the space, in pixels, between a toolbar button and its menu.
const MENU_GAP = 4;

// Page is what a page other than dg serve's changes in the editor.
export interface Page {
  name: string;
  connect?: Connect;
  openDiagnostic?: (d: Diagnostic) => void;
  saves?: () => Entry[];
}

// startEditor runs the editor: it draws each snapshot the server sends on a JointJS paper, with
// the markup the SVG export uses, fading unpinned nodes in a view with pinned ones, and lists
// diagnostics. Gestures on the canvas (canvas/interact.ts), the palette, the inspector
// (canvas/panels.ts) and the toolbar send operations; undo and redo go to the server, whose
// history the clients share (FR-10). While the project has errors the last good drawing stays,
// read-only. The view switcher, the breadcrumbs and the drill-down badges select views (spec 8.2).
export function startEditor(render: (view: ResolvedView) => Promise<{ drawing: Drawing; family: string }>, page: Page = { name: 'dg serve' }) {
  const bar = toolbar(document.getElementById('bar')!, document.getElementById('overflow-menu')!);
  const select = bar.views;
  const crumbs = bar.crumbs;
  const menu = document.getElementById('drill-menu')!;
  const toolMenu = document.getElementById('tool-menu')!;
  const status = document.getElementById('status')!;
  const hint = document.getElementById('hint')!;
  const guide = document.getElementById('guide')!;
  const empty = document.getElementById('empty')!;
  const broken = document.getElementById('broken')!;
  const ctxMenu = document.getElementById('context-menu')!;
  const toastEl = document.getElementById('toast')!;
  const diagnosticsEl = document.getElementById('diagnostics')!;
  const opener = page.openDiagnostic ?? (inPanel ? openDiagnostic : undefined);
  const diagBar = diagnosticsBar(diagnosticsEl, opener);
  bindMenu(menu);
  bindMenu(toolMenu);
  const graph = new dia.Graph({}, { cellNamespace: namespace });
  const paper = new dia.Paper({
    el: document.getElementById('paper')!,
    model: graph,
    cellViewNamespace: namespace,
    interactive: false,
    async: false,
    width: 1,
    height: 1,
  });
  // A drag can move nodes past the drawing's bounds until the drawing after the drop.
  paper.svg.style.overflow = 'visible';

  const flowStyle = document.head.appendChild(document.createElement('style'));
  let serverDiags: Diagnostic[] = [];
  let tooSmall: string[] = [];
  let overflowing: string[] = [];
  let model: ModelInfo = { kinds: [], edge_kinds: [], tags: [], nodes: [], edges: [] };
  let pending: { view: ResolvedView; model: ModelInfo } | undefined;
  let drawing = false;
  let shown: ResolvedView | undefined;
  let received: ResolvedView | undefined;
  let current: { scene: Scene; family: string } | undefined;
  let ready: { view: ResolvedView; model: ModelInfo; drawing: Drawing; family: string } | undefined;
  // then are the callbacks of requests waiting for their ack; after, the callback of the request
  // answered last, waiting for the drawing its change brings.
  const then = new Map<number, (results: Result[]) => void>();
  let after: (() => void) | undefined;
  let open = false;
  // waiting is the seq of the request the server has not answered yet; redraw is set when a
  // snapshot arrived since it was sent, whose drawing ends the wait.
  let waiting: number | undefined;
  let redraw = false;
  let titles = new Map<string, string>();
  const title = (id: string) => titles.get(id) || id;

  const setPanel = (cls: string, id: string, on: boolean) => {
    document.body.classList.toggle(cls, !on);
    bar.button(id).setAttribute('aria-pressed', String(on));
  };

  const showDiagnostics = () => {
    const all = [
      ...serverDiags,
      ...tooSmall.map((id): Diagnostic => ({ severity: 'warning', summary: 'Container too small', detail: `Node "${id}" has a pinned size too small for its children.` })),
      ...overflowing.map((id): Diagnostic => ({ severity: 'warning', summary: 'Label overflows node', detail: `Node "${id}" has a pinned size too small for its label.` })),
    ];
    const errors = all.filter((d) => d.severity === 'error');
    const stale = errors.length > 0;
    document.body.classList.toggle('stale', stale);
    diagBar.show(all, stale && current ? 'Showing the last good drawing, read-only.' : '');
    const blank = stale && !current;
    diagnosticsEl.hidden ||= blank;
    broken.hidden = !blank;
    if (blank) broken.replaceChildren(h('h2', {}, 'The project has errors'), diagnosticList(errors, opener));
    controls();
  };

  const stale = () => document.body.classList.contains('stale');
  const editable = () => open && !stale() && waiting === undefined && !redraw && !!current && !ui.busy();
  let canUndo = false;
  let canRedo = false;
  // blocked says why nothing can be edited now.
  const blocked = () => (!open ? `Waiting for ${page.name}` : stale() ? 'Fix the errors in the project first' : current ? 'Saving…' : 'Loading…');
  const controls = () => {
    const on = editable();
    document.body.classList.toggle('editable', on);
    const tool = (id: string, ok: boolean, need: string) => bar.enable(id, on && ok, on ? need : blocked());
    tool('undo', canUndo, 'Nothing to undo');
    tool('redo', canRedo, 'Nothing to redo');
    tool('unpin', ui.can('unpin'), 'Select a pinned node');
    tool('arrange', ui.can(2), 'Select two nodes or more');
    for (const id of ['new-view', 'add-node', 'connect', 'edge-kind', 'add-decoration']) tool(id, true, '');
    for (const id of ['zoom-out', 'zoom', 'zoom-in', 'fit']) bar.enable(id, !!current, 'Nothing to zoom yet');
    bar.enable('save', !!current, 'Nothing to export yet');
    bar.button('connect').setAttribute('aria-pressed', String(ui.connecting()));
    guide.hidden = !ui.connecting();
    const s = current?.scene;
    hint.hidden = !s || ui.connecting() || s.drawing.nodes.length === 0 || s.drawing.nodes.some((n) => n.pinned);
    pal.update(s?.model, on);
    panel.show(s, ui.selection(), !open || stale() || !s);
  };

  const show = (d: Drawing, family: string) => {
    paper.svg.setAttribute('font-family', `${family}, sans-serif`);
    paper.defs.querySelectorAll('[data-dg]').forEach((el) => el.remove());
    const defs = new DOMParser().parseFromString(`<svg xmlns="http://www.w3.org/2000/svg">${defsMarkup(d.nodes, d).join('')}</svg>`, 'image/svg+xml');
    for (const el of [...defs.documentElement.children]) {
      el.setAttribute('data-dg', '');
      paper.defs.appendChild(document.importNode(el, true));
    }
    // In a view with nothing pinned every node is unpinned, so fading tells nothing; a hint says
    // the layout is not saved instead (spec 7.4).
    const anyPinned = d.nodes.some((n) => n.pinned);
    empty.hidden = d.nodes.length > 0;
    document.body.classList.toggle('blank', !d.nodes.length);
    if (!d.nodes.length) setPanel('no-palette', 'show-palette', true);
    const periods = flowPeriods(d.edges);
    const cells = layers(d).map((l, z) => {
      if ('node' in l) {
        const n = l.node;
        return new Markup({
          id: 'n:' + n.id,
          position: { x: n.box.x, y: n.box.y },
          size: { width: n.box.w, height: n.box.h },
          markup: nodeMarkup(shiftNode(n, -n.box.x, -n.box.y), d, d).join(''),
          attrs: { root: { opacity: n.pinned || !anyPinned ? 1 : UNPINNED_OPACITY } },
          z,
        });
      }
      const id = 'edge' in l ? 'e:' + l.edge.id : 'overlay' in l ? 'o:' + l.overlay.id : 'd:' + l.decoration.id;
      return new Markup({ id, position: { x: 0, y: 0 }, size: { width: 1, height: 1 }, markup: layerMarkup(l, d, periods).join(''), z });
    });
    flowStyle.textContent = flowCSS(periods);
    checkStyle(flowStyle);
    graph.resetCells(cells);
    fit(d);
    const opened = paper.el.dataset.view !== d.view;
    paper.el.dataset.view = d.view;
    ui.shown();
    if (opened) ui.fit();
  };

  // fit sizes the paper to the drawing at the zoom.
  const fit = (d: Drawing) => {
    const z = ui.zoom();
    paper.scale(z, z);
    paper.setDimensions(d.bounds.w * z, d.bounds.h * z);
    paper.translate(-d.bounds.x * z, -d.bounds.y * z);
    const level = `${Math.round(z * 100)}%`;
    bar.setText('zoom', level, `Zoom level ${level}`);
  };

  // display shows a drawing of view now, or after the gesture in progress.
  const display = (view: ResolvedView, model: ModelInfo, d: Drawing, family: string) => {
    if (ui.busy()) {
      ready = { view, model, drawing: d, family };
      return;
    }
    ready = undefined;
    current = { scene: scene(view, d, model), family };
    showKinds(model.edge_kinds);
    show(d, family);
    shown = view;
    tooSmall = d.nodes.filter((n) => n.tooSmall).map((n) => n.id);
    overflowing = d.nodes.filter((n) => n.overflows).map((n) => n.id);
  };

  const drawLatest = async () => {
    if (drawing) return;
    drawing = true;
    try {
      while (pending) {
        const { view, model } = pending;
        pending = undefined;
        try {
          const { drawing: d, family } = await render(view);
          display(view, model, d, family);
        } catch (err) {
          status.textContent = `Drawing failed: ${err}`;
        }
        if (!pending) redraw = false;
        showDiagnostics();
        if (!pending && waiting === undefined && after) {
          const f = after;
          after = undefined;
          f();
        }
      }
    } finally {
      drawing = false;
    }
  };

  // openMenu lists views at the client point (x, y); choosing one selects it.
  const openMenu = (views: string[], x: number, y: number, from?: Element) => {
    showMenu(menu, views.map((v): Entry => ({ label: title(v), view: v, on: true, run: () => conn.selectView(v) })), x, y, from);
  };

  // A breadcrumb leads to a view, or, when several views are one level up, to a menu of them.
  const showCrumbs = (view: ResolvedView) => {
    const items: Node[] = [];
    for (const level of view.breadcrumbs ?? []) {
      const b = document.createElement('button');
      b.type = 'button';
      b.textContent = level.length === 1 ? title(level[0]) : `${title(level[0])} (+${level.length - 1}) ▾`;
      b.dataset.views = level.join(' ');
      b.dataset.tip = b.textContent;
      b.addEventListener('click', (ev) => {
        if (level.length === 1) return conn.selectView(level[0]);
        const r = b.getBoundingClientRect();
        openMenu(level, r.left, r.bottom, b);
        ev.stopPropagation();
      });
      items.push(b, h('span', { class: 'sep' }, ' › '));
    }
    if (items.length) {
      const here = document.createElement('span');
      here.textContent = title(view.view);
      here.dataset.tip = title(view.view);
      items.push(here);
    }
    crumbs.replaceChildren(...items);
    crumbs.hidden = items.length === 0;
    bar.layout();
  };

  // A drill-down badge opens the view of its node, or, when several views show its inside, a
  // menu of them.
  paper.el.addEventListener('click', (ev) => {
    // At a low zoom the resize handle of a selected node lies over the badge in its corner.
    const p = ui.at({ x: ev.clientX, y: ev.clientY });
    const id = (ev.target as Element).closest('[data-drill]')?.getAttribute('data-drill') ?? current?.scene.drawing.nodes.find((n) => p && n.drill && p.x >= n.drill.x && p.x <= n.drill.x + n.drill.w && p.y >= n.drill.y && p.y <= n.drill.y + n.drill.h)?.id;
    const latest = received?.view === shown?.view ? received : shown;
    const views = latest?.nodes.find((n) => n.id === id)?.drill ?? [];
    if (views.length === 1) conn.selectView(views[0]);
    if (views.length < 2) return;
    openMenu(views, ev.clientX, ev.clientY);
    ev.stopPropagation();
  });
  tooltips(document.getElementById('tip')!, (el) => {
    const id = el.getAttribute('data-drill');
    if (id === null) return undefined;
    const views = shown?.nodes.find((n) => n.id === id)?.drill ?? [];
    return views.length === 1 ? `Open view ${title(views[0])}` : `Choose one of ${views.length} views`;
  });

  const showViews = (views: ViewInfo[], current: string) => {
    titles = new Map(views.map((v) => [v.id, v.title ?? '']));
    select.replaceChildren(
      ...views.map((v) => {
        const o = document.createElement('option');
        o.value = v.id;
        o.textContent = v.title ? `${v.title} (${v.id})` : v.id;
        return o;
      }),
    );
    select.value = current;
    bar.layout();
  };

  const send = (op: Op, f?: (results: Result[]) => void) => request({ type: 'op', op }, f);
  const setStatus = (message: string) => {
    status.textContent = message;
  };
  // toast shows message with a button that undoes the change it tells of (spec 7.6), until the
  // next change or drawing, after which undo would mean something else.
  const toast = (message: string) => {
    const button = h('button', { type: 'button', onclick: () => (hideToast(), request({ type: 'undo' })) }, 'Undo');
    toastEl.replaceChildren(h('span', {}, message), button);
    toastEl.hidden = false;
  };
  const hideToast = () => {
    toastEl.hidden = true;
  };
  const host = {
    scene: () => current?.scene,
    editable,
    send,
    select: (sel: Selection) => ui.select(sel),
    selectView: (id: string) => conn.selectView(id),
    status: setStatus,
  };
  const act = actions(host);
  const help = () => void sheet('Keyboard shortcuts', shortcutSheet());

  const ui = interact(paper, document.getElementById('canvas')!, ctxMenu, {
    ...host,
    restore: () => current && show(current.scene.drawing, current.family),
    ended: () => {
      if (ready) display(ready.view, ready.model, ready.drawing, ready.family);
      controls();
    },
    toast,
    edgeKind: () => edgeKind,
    help,
  }, act, () => current && fit(current.scene.drawing), () => controls());

  // create creates a node of kind at p, then selects it and edits its label in place, so the
  // first label gives it its ID (spec 6.3).
  const create = (kind: string, p: Point) => {
    const s = current?.scene;
    const op = s && createOps(s, kind, p);
    if (!op || !editable()) return;
    send(op, (results) => {
      const id = results.find((r) => r.op === 'create_node')?.id;
      if (id) ui.editLabel(id);
    });
  };
  let edgeKind = 'default';
  const pal = palette(document.getElementById('palette')!, (kind) => current && create(kind, clearOf(current.scene, ui.center())), (kind, client) => {
    const p = ui.at(client);
    if (p) create(kind, p);
  });
  const panel = inspector(document.getElementById('inspector')!, host, act);

  const showKinds = (kinds: string[]) => {
    if (!kinds.includes(edgeKind)) edgeKind = kinds[0] ?? '';
    bar.setText('edge-kind', `Edge: ${edgeKind}`);
    bar.layout();
  };

  // request sends r unless the editor cannot edit now; f gets the results of an operation once
  // the drawing it brings is shown.
  const request = (r: Parameters<typeof conn.send>[0], f?: (results: Result[]) => void) => {
    if (!editable()) return false;
    waiting = conn.send(r);
    if (waiting !== undefined && f) then.set(waiting, f);
    status.textContent = '';
    controls();
    return waiting !== undefined;
  };

  // answered ends the wait for request seq: after an error, or a change that drew nothing new,
  // the current drawing is shown again without what the gesture moved. An operation's callback
  // runs once its drawing is shown.
  const answered = (seq: number | undefined, failed: boolean, results: Result[] = []) => {
    if (seq === undefined || seq !== waiting) return;
    waiting = undefined;
    const f = then.get(seq);
    then.clear();
    after = f && !failed ? () => f(results) : undefined;
    if (failed || !redraw) {
      redraw = false;
      if (current) show(current.scene.drawing, current.family);
      const g = after;
      after = undefined;
      g?.();
    }
    controls();
  };

  const conn = (page.connect ?? connect)(
    (m: ServerMessage) => {
      switch (m.type) {
        case 'model':
          model = m.model;
          if (current) current = { ...current, scene: { ...current.scene, model } };
          if (ready) ready.model = model;
          showKinds(model.edge_kinds);
          controls();
          break;
        case 'snapshot':
          showViews(m.views, m.resolved.view);
          showCrumbs(m.resolved);
          hideToast();
          received = m.resolved;
          pending = { view: m.resolved, model };
          if (waiting !== undefined) redraw = true;
          void drawLatest();
          break;
        case 'diagnostics':
          serverDiags = m.diagnostics;
          showDiagnostics();
          break;
        case 'history':
          hideToast();
          canUndo = m.undo;
          canRedo = m.redo;
          controls();
          break;
        case 'ack':
          status.textContent = (m.warnings ?? []).map((d) => `warning: ${d.summary}${d.detail ? `: ${d.detail}` : ''}`).join(' ');
          answered(m.seq, false, m.results ?? []);
          break;
        case 'error':
          status.textContent = m.message;
          answered(m.seq, true);
          break;
      }
    },
    (up) => {
      open = up;
      if (!up) {
        waiting = undefined;
        redraw = false;
      }
      setStatus(up ? '' : `Disconnected from ${page.name}.`);
      controls();
    },
  );
  // The palette and the inspector start hidden in a narrow window, such as a VS Code panel.
  for (const [id, cls] of [
    ['show-palette', 'no-palette'],
    ['show-inspector', 'no-inspector'],
  ]) {
    setPanel(cls, id, window.innerWidth >= WIDE);
    bar.click(id, () => setPanel(cls, id, document.body.classList.contains(cls)));
  }

  // openTool opens entries as the menu of tool id, or closes it when it is already open.
  const openTool = (id: string, entries: () => Entry[]) => {
    bar.click(id, () => {
      const b = bar.button(id);
      if (openerOf(toolMenu) === b) return closeMenu(toolMenu);
      const r = b.getBoundingClientRect();
      showMenu(toolMenu, entries(), r.left, r.bottom + MENU_GAP, b);
    });
  };
  openTool('edge-kind', () => model.edge_kinds.map((k) => ({ label: k, on: true, checked: k === edgeKind, run: () => ((edgeKind = k), showKinds(model.edge_kinds)) })));
  const decoration = (label: string, type: DecorationType): Entry => ({
    label,
    on: true,
    run: () => {
      const sel = ui.selection();
      void act.addDecoration(type, ui.center(), sel.nodes.length === 1 ? sel.nodes[0] : undefined);
    },
  });
  openTool('add-decoration', () => [decoration('Text', 'text'), decoration('Rectangle', 'rect'), decoration('Ellipse', 'ellipse'), decoration('Band', 'band'), decoration('Image', 'image'), decoration('Arrow', 'arrow'), decoration('Freehand', 'freehand')]);
  openTool('arrange', () => arrangeItems(ui.arrange, ui.movable()));
  const zoomTo = (z: number): Entry => ({ label: `${z * 100}%`, on: true, run: () => ui.setZoom(z) });
  const zoomToSelection = () => {
    const b = ui.selectionBox();
    if (b) ui.fit(b);
  };
  openTool('zoom', () => [zoomTo(0.5), zoomTo(1), zoomTo(2), { label: '', on: true, run: () => {} }, { label: 'Fit', keys: 'Shift+1', on: true, run: () => ui.fit() }, { label: 'Zoom to selection', keys: 'Shift+2', on: !!ui.selectionBox(), why: 'Select something first', run: zoomToSelection }]);
  const save = async (name: string, data: () => Blob | Promise<Blob>) => {
    try {
      const a = h('a', { href: URL.createObjectURL(await data()), download: name });
      a.click();
      URL.revokeObjectURL(a.href);
    } catch (err) {
      setStatus(`Export failed: ${err}`);
    }
  };
  openTool('save', () => {
    const d = picture(current!.scene.drawing);
    const view = current!.scene.view;
    return [
      { label: 'Export SVG', on: true, run: () => void save(`${d.view}.svg`, () => new Blob([toSVG(d)], { type: 'image/svg+xml' })) },
      { label: 'Export PNG', on: true, run: () => void save(`${d.view}.png`, () => toPNG(d, view.font)) },
      ...(page.saves?.() ?? []),
    ];
  });
  select.addEventListener('change', () => conn.selectView(select.value));
  bar.click('new-view', () => void act.newView(ui.selection().nodes.map(nodeSelector)));
  bar.click('add-node', () => void act.addNode());
  bar.click('connect', () => ui.connecting(!ui.connecting()));
  bar.click('undo', () => request({ type: 'undo' }));
  bar.click('redo', () => request({ type: 'redo' }));
  bar.click('unpin', () => ui.unpin());
  bar.click('zoom-out', () => ui.setZoom(ui.zoom() / 1.25));
  bar.click('zoom-in', () => ui.setZoom(ui.zoom() * 1.25));
  bar.click('fit', () => ui.fit());
  bar.click('help', help);
  document.addEventListener('keydown', (ev) => {
    const target = ev.target as HTMLElement;
    if (['INPUT', 'SELECT', 'TEXTAREA'].includes(target.tagName) || target.isContentEditable || !(ev.ctrlKey || ev.metaKey)) return;
    const key = ev.key.toLowerCase();
    if (key === 'z' && !ev.shiftKey) request({ type: 'undo' });
    else if ((key === 'z' && ev.shiftKey) || key === 'y') request({ type: 'redo' });
    else return;
    ev.preventDefault();
  });
}

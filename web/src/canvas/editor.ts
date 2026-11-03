import { dia } from '@joint/core';
import { scene, type Op, type Scene } from '../edit/edit';
import { createOps, nodeSelector, type DecorationType } from '../edit/model';
import type { Point } from '../geometry/geometry';
import type { Diagnostic, ModelInfo, Result, ServerMessage, ViewInfo } from '../protocol/client';
import { connect } from '../protocol/client';
import type { ResolvedView } from '../protocol/view';
import { actions, type Selection } from './actions';
import { h } from './dialog';
import { checkStyle, layerMarkup, layers, type Drawing } from './export';
import { interact } from './interact';
import { inspector, palette } from './panels';
import { defsMarkup, flowCSS, flowPeriods, nodeMarkup, shiftNode } from './markup';

const UNPINNED_OPACITY = 0.55;

const Markup = dia.Element.define('dg.Markup', {});
const namespace = { dg: { Markup } };

// WIDE is the narrowest window, in pixels, that shows the palette and the inspector at first.
const WIDE = 1000;

// startEditor runs the editor: it draws each snapshot the server sends on a JointJS paper, with
// the markup the SVG export uses, fading unpinned nodes in a view with pinned ones, and lists
// diagnostics. Gestures on the canvas (canvas/interact.ts), the palette, the inspector
// (canvas/panels.ts) and the toolbar send operations; undo and redo go to the server, whose
// history the clients share (FR-10). While the project has errors the last good drawing stays,
// read-only. The view switcher, the breadcrumbs and the drill-down badges select views (spec 8.2).
export function startEditor(render: (view: ResolvedView) => Promise<{ drawing: Drawing; family: string }>) {
  const select = document.getElementById('views') as HTMLSelectElement;
  const crumbs = document.getElementById('crumbs')!;
  const menu = document.getElementById('drill-menu')!;
  const status = document.getElementById('status')!;
  const hint = document.getElementById('hint')!;
  const list = document.getElementById('diagnostics')!;
  const undo = document.getElementById('undo') as HTMLButtonElement;
  const redo = document.getElementById('redo') as HTMLButtonElement;
  const arrange = document.getElementById('arrange') as HTMLSelectElement;
  const unpin = document.getElementById('unpin') as HTMLButtonElement;
  const zoomLabel = document.getElementById('zoom')!;
  const ctxMenu = document.getElementById('context-menu')!;
  const newView = document.getElementById('new-view') as HTMLButtonElement;
  const addNode = document.getElementById('add-node') as HTMLButtonElement;
  const connectBtn = document.getElementById('connect') as HTMLButtonElement;
  const edgeKind = document.getElementById('edge-kind') as HTMLSelectElement;
  const addDecoration = document.getElementById('add-decoration') as HTMLSelectElement;
  const toastEl = document.getElementById('toast')!;
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

  const showDiagnostics = () => {
    const all = [...serverDiags, ...tooSmall.map((id): Diagnostic => ({ severity: 'warning', summary: 'Container too small', detail: `Node "${id}" has a pinned size too small for its children.` }))];
    list.replaceChildren(
      ...all.map((d) => {
        const li = document.createElement('li');
        li.className = d.severity;
        li.textContent = (d.file ? `${d.file}:${d.line}:${d.column}: ` : '') + `${d.severity}: ${d.summary}` + (d.detail ? `: ${d.detail}` : '');
        return li;
      }),
    );
    list.hidden = all.length === 0;
    const errors = serverDiags.some((d) => d.severity === 'error');
    document.body.classList.toggle('stale', errors);
    status.textContent = errors ? 'The project has errors; showing the last good drawing (read-only).' : '';
    controls();
  };

  const editable = () => open && !document.body.classList.contains('stale') && waiting === undefined && !redraw && !!current && !ui.busy();
  let canUndo = false;
  let canRedo = false;
  const controls = () => {
    const on = editable();
    document.body.classList.toggle('editable', on);
    undo.disabled = !canUndo || !on;
    redo.disabled = !canRedo || !on;
    unpin.disabled = !on || !ui.can('unpin');
    arrange.disabled = !on || !ui.can(2);
    for (const o of arrange.options) if (o.value === 'x' || o.value === 'y') o.disabled = !ui.can(3);
    for (const c of [newView, addNode, connectBtn, edgeKind, addDecoration]) c.disabled = !on;
    connectBtn.setAttribute('aria-pressed', String(ui.connecting()));
    const s = current?.scene;
    pal.update(s?.model.kinds ?? [], on);
    panel.show(s, ui.selection(), !open || document.body.classList.contains('stale') || !s);
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
    hint.hidden = anyPinned;
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
    if (paper.el.dataset.view !== d.view) paper.el.parentElement!.scrollTo(0, 0);
    paper.el.dataset.view = d.view;
    ui.shown();
  };

  // fit sizes the paper to the drawing at the zoom.
  const fit = (d: Drawing) => {
    const z = ui.zoom();
    paper.scale(z, z);
    paper.setDimensions(d.bounds.w * z, d.bounds.h * z);
    paper.translate(-d.bounds.x * z, -d.bounds.y * z);
    zoomLabel.textContent = `${Math.round(z * 100)}%`;
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

  // openMenu lists views at the page position (x, y); choosing one selects it.
  const openMenu = (views: string[], x: number, y: number) => {
    menu.replaceChildren(
      ...views.map((v) => {
        const b = document.createElement('button');
        b.type = 'button';
        b.textContent = title(v);
        b.dataset.view = v;
        b.addEventListener('click', () => {
          menu.hidden = true;
          conn.selectView(v);
        });
        return b;
      }),
    );
    menu.style.left = `${x}px`;
    menu.style.top = `${y}px`;
    menu.hidden = false;
  };

  // A breadcrumb leads to a view, or, when several views are one level up, to a menu of them.
  const showCrumbs = (view: ResolvedView) => {
    const items: Node[] = [];
    for (const level of view.breadcrumbs ?? []) {
      const b = document.createElement('button');
      b.type = 'button';
      b.textContent = level.length === 1 ? title(level[0]) : `${title(level[0])} (+${level.length - 1}) ▾`;
      b.dataset.views = level.join(' ');
      b.addEventListener('click', (ev) => {
        if (level.length === 1) return conn.selectView(level[0]);
        const r = b.getBoundingClientRect();
        openMenu(level, r.left + window.scrollX, r.bottom + window.scrollY);
        ev.stopPropagation();
      });
      items.push(b, document.createTextNode(' › '));
    }
    if (items.length) {
      const here = document.createElement('span');
      here.textContent = title(view.view);
      items.push(here);
    }
    crumbs.replaceChildren(...items);
    crumbs.hidden = items.length === 0;
  };

  // A drill-down badge opens the view of its node, or, when several views show its inside, a
  // menu of them.
  paper.el.addEventListener('click', (ev) => {
    const id = (ev.target as Element).closest('[data-drill]')?.getAttribute('data-drill');
    const current = received?.view === shown?.view ? received : shown;
    const views = current?.nodes.find((n) => n.id === id)?.drill ?? [];
    if (views.length === 1) conn.selectView(views[0]);
    if (views.length < 2) return;
    openMenu(views, ev.pageX, ev.pageY);
    ev.stopPropagation();
  });
  document.addEventListener('click', (ev) => {
    if (!menu.contains(ev.target as Node)) menu.hidden = true;
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

  const ui = interact(paper, document.getElementById('canvas')!, ctxMenu, {
    ...host,
    restore: () => current && show(current.scene.drawing, current.family),
    ended: () => {
      if (ready) display(ready.view, ready.model, ready.drawing, ready.family);
      controls();
    },
    toast,
    edgeKind: () => edgeKind.value,
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
  const pal = palette(document.getElementById('palette')!, (kind) => create(kind, ui.center()), (kind, client) => {
    const p = ui.at(client);
    if (p) create(kind, p);
  });
  const panel = inspector(document.getElementById('inspector')!, host, act);

  const showKinds = (kinds: string[]) => {
    if ([...edgeKind.options].map((o) => o.value).join('\n') === kinds.join('\n')) return;
    const was = edgeKind.value || 'default';
    edgeKind.replaceChildren(...kinds.map((k) => h('option', { value: k }, k)));
    edgeKind.value = kinds.includes(was) ? was : kinds[0] ?? '';
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

  const conn = connect(
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
      document.body.classList.toggle('disconnected', !up);
      controls();
    },
  );
  // The palette and the inspector start hidden in a narrow window, such as a VS Code panel.
  for (const [id, cls] of [
    ['show-palette', 'no-palette'],
    ['show-inspector', 'no-inspector'],
  ]) {
    const b = document.getElementById(id)!;
    const set = (on: boolean) => {
      document.body.classList.toggle(cls, !on);
      b.setAttribute('aria-pressed', String(on));
    };
    set(window.innerWidth >= WIDE);
    b.addEventListener('click', () => set(document.body.classList.contains(cls)));
  }
  select.addEventListener('change', () => conn.selectView(select.value));
  newView.addEventListener('click', () => act.newView(ui.selection().nodes.map(nodeSelector)));
  addNode.addEventListener('click', () => act.addNode());
  connectBtn.addEventListener('click', () => ui.connecting(!ui.connecting()));
  addDecoration.addEventListener('change', () => {
    const type = addDecoration.value as DecorationType;
    addDecoration.value = '';
    const sel = ui.selection();
    if (type) void act.addDecoration(type, ui.center(), sel.nodes.length === 1 ? sel.nodes[0] : undefined);
  });
  undo.addEventListener('click', () => request({ type: 'undo' }));
  redo.addEventListener('click', () => request({ type: 'redo' }));
  unpin.addEventListener('click', () => ui.unpin());
  arrange.addEventListener('change', () => {
    const how = arrange.value as Parameters<typeof ui.arrange>[0];
    arrange.value = '';
    if (how) ui.arrange(how);
  });
  document.getElementById('zoom-out')!.addEventListener('click', () => ui.setZoom(ui.zoom() / 1.25));
  document.getElementById('zoom-in')!.addEventListener('click', () => ui.setZoom(ui.zoom() * 1.25));
  zoomLabel.addEventListener('click', () => ui.setZoom(1));
  document.addEventListener('keydown', (ev) => {
    const tag = (ev.target as Element).tagName;
    if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA' || !(ev.ctrlKey || ev.metaKey)) return;
    const key = ev.key.toLowerCase();
    if (key === 'z' && !ev.shiftKey) request({ type: 'undo' });
    else if ((key === 'z' && ev.shiftKey) || key === 'y') request({ type: 'redo' });
    else return;
    ev.preventDefault();
  });
}

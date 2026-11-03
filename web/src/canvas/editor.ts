import { dia } from '@joint/core';
import { scene, type Op, type Scene } from '../edit/edit';
import type { Diagnostic, ServerMessage, ViewInfo } from '../protocol/client';
import { connect } from '../protocol/client';
import type { ResolvedView } from '../protocol/view';
import { checkStyle, layerMarkup, layers, type Drawing } from './export';
import { interact } from './interact';
import { defsMarkup, flowCSS, flowPeriods, nodeMarkup, shiftNode } from './markup';

const UNPINNED_OPACITY = 0.55;

const Markup = dia.Element.define('dg.Markup', {});
const namespace = { dg: { Markup } };

// startEditor runs the editor: it draws each snapshot the server sends on a JointJS paper, with
// the markup the SVG export uses, fading unpinned nodes in a view with pinned ones, and lists
// diagnostics. Layout gestures (canvas/interact.ts) send operations; undo and redo go to the
// server, whose history the clients share (FR-10). While the project has errors the last good
// drawing stays, read-only. The view switcher, the breadcrumbs and the drill-down badges select
// views (spec 8.2).
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
  let pending: ResolvedView | undefined;
  let drawing = false;
  let shown: ResolvedView | undefined;
  let received: ResolvedView | undefined;
  let current: { scene: Scene; family: string } | undefined;
  let ready: { view: ResolvedView; drawing: Drawing; family: string } | undefined;
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
    document.body.classList.toggle('editable', editable());
    undo.disabled = !canUndo || !editable();
    redo.disabled = !canRedo || !editable();
    unpin.disabled = !editable() || !ui.can('unpin');
    arrange.disabled = !editable() || !ui.can(2);
    for (const o of arrange.options) if (o.value === 'x' || o.value === 'y') o.disabled = !ui.can(3);
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
    if (paper.el.dataset.view !== d.view) window.scrollTo(0, 0);
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
  const display = (view: ResolvedView, d: Drawing, family: string) => {
    if (ui.busy()) {
      ready = { view, drawing: d, family };
      return;
    }
    ready = undefined;
    current = { scene: scene(view, d), family };
    show(d, family);
    shown = view;
    tooSmall = d.nodes.filter((n) => n.tooSmall).map((n) => n.id);
  };

  const drawLatest = async () => {
    if (drawing) return;
    drawing = true;
    try {
      while (pending) {
        const view = pending;
        pending = undefined;
        try {
          const { drawing: d, family } = await render(view);
          display(view, d, family);
        } catch (err) {
          status.textContent = `Drawing failed: ${err}`;
        }
        if (!pending) redraw = false;
        showDiagnostics();
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

  const ui = interact(paper, document.getElementById('canvas')!, ctxMenu, {
    scene: () => current?.scene,
    editable,
    send: (op: Op) => request({ type: 'op', op }),
    restore: () => current && show(current.scene.drawing, current.family),
    ended: () => {
      if (ready) display(ready.view, ready.drawing, ready.family);
    },
  }, () => current && fit(current.scene.drawing), () => controls());

  const request = (r: Parameters<typeof conn.send>[0]) => {
    if (!editable()) return false;
    waiting = conn.send(r);
    status.textContent = '';
    controls();
    return waiting !== undefined;
  };

  // answered ends the wait for request seq: after an error, or a change that drew nothing new,
  // the current drawing is shown again without what the gesture moved.
  const answered = (seq: number | undefined, failed: boolean) => {
    if (seq === undefined || seq !== waiting) return;
    waiting = undefined;
    if (failed || !redraw) {
      redraw = false;
      if (current) show(current.scene.drawing, current.family);
    }
    controls();
  };

  const conn = connect(
    (m: ServerMessage) => {
      switch (m.type) {
        case 'snapshot':
          showViews(m.views, m.resolved.view);
          showCrumbs(m.resolved);
          pending = received = m.resolved;
          if (waiting !== undefined) redraw = true;
          void drawLatest();
          break;
        case 'diagnostics':
          serverDiags = m.diagnostics;
          showDiagnostics();
          break;
        case 'history':
          canUndo = m.undo;
          canRedo = m.redo;
          controls();
          break;
        case 'ack':
          status.textContent = (m.warnings ?? []).map((d) => `warning: ${d.summary}${d.detail ? `: ${d.detail}` : ''}`).join(' ');
          answered(m.seq, false);
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
  select.addEventListener('change', () => conn.selectView(select.value));
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

import type { dia } from '@joint/core';
import {
  alignOps,
  batch,
  distributeOps,
  edgeAt,
  grown,
  hintOp,
  inside,
  labelPos,
  landing,
  movable,
  moved as movedWith,
  moveOps,
  pinPortOps,
  portPin,
  reorderOps,
  resizedBox,
  resizeOps,
  snapDelta,
  unpinOps,
  withPins,
  withVia,
  type Align,
  type Handle,
  type Op,
  type Scene,
} from '../edit/edit';
import type { Box, Point } from '../geometry/geometry';
import { liveEdges, liveRequest, liveRoute, type LiveEdge } from '../route/live';
import { nearestSide } from '../route/edges';
import { marker } from '../route/markers';
import { onSide, pointAlong } from '../route/route';
import { liveRouter, type LiveRouter } from './avoid';
import { edgeMarkup, flowPeriods, nodeMarkup, shiftNode, type DrawnEdge } from './markup';

// Host is what the interactions need from the editor.
export interface Host {
  scene(): Scene | undefined;
  // editable reports whether layout edits can be sent: the project parses, the server is
  // connected and no edit is waiting for its answer and drawing.
  editable(): boolean;
  // send sends an operation, and reports whether it did.
  send(op: Op): boolean;
  // restore draws the current drawing again, dropping what a gesture moved.
  restore(): void;
  // ended is called when a gesture ends, so a drawing that arrived during it can be shown.
  ended(): void;
}

// DRAG starts a drag after the pointer moved this many screen pixels.
const DRAG = 3;
// HIT is how near, in screen pixels, a pointer selects an edge.
const HIT = 6;
// BUDGET is the longest a live route may take, in milliseconds, before the rest of a drag
// draws its moving edges as lines between their ends (spec 7.5).
const BUDGET = 8;
// DRAGGED_OPACITY lets the siblings a dragged child of a stack or grid passes over show through.
const DRAGGED_OPACITY = 0.6;
const ZOOMS = [0.25, 0.33, 0.5, 0.67, 0.75, 0.9, 1, 1.1, 1.25, 1.5, 2, 3, 4];
const SVG = 'http://www.w3.org/2000/svg';

type Gesture =
  | { kind: 'move'; ids: string[]; moved: Set<string>; d: Point; live: Live; room: Set<string> }
  | { kind: 'reorder'; id: string; p: Point }
  | { kind: 'resize'; id: string; handle: Handle; box: Box }
  | { kind: 'port'; id: string; port: string; p: Point }
  | { kind: 'end'; edge: string; end: 0 | 1; p: Point }
  | { kind: 'label'; edge: string; p: Point }
  | { kind: 'via'; edge: string; index: number; p: Point }
  | { kind: 'box'; p: Point; add: boolean }
  | { kind: 'pan'; left: number; top: number };

// Live is the live routing of a move of ids, which moves the nodes of moved: router is null
// once a route took longer than BUDGET.
interface Live {
  ids: string[];
  moved: Set<string>;
  edges: LiveEdge[];
  router?: LiveRouter | null;
}

interface Press {
  client: Point;
  at: Point;
  shift: boolean;
  target: Element;
  node?: string;
  edge?: string;
  handle?: string;
  pan: boolean;
  // live is prepared when the press is on a node that a drag would move.
  live?: Live;
}

export interface Interaction {
  // shown is called after a drawing is shown, to draw the selection again.
  shown(): void;
  busy(): boolean;
  zoom(): number;
  // can reports whether the unpin, align (2) and distribute (3) actions apply to the selection.
  can(action: 'unpin' | 2 | 3): boolean;
  arrange(how: Align | 'x' | 'y'): void;
  unpin(): void;
  setZoom(z: number): void;
}

// interact handles the layout gestures on the paper (FR-11): selection by click, shift-click and
// box; pan and zoom; move, resize, reorder, port, edge end, label and via drags, each sending one
// operation or batch on drop; align, distribute and unpin; a context menu.
export function interact(paper: dia.Paper, scroller: HTMLElement, menu: HTMLElement, host: Host, onZoom: () => void, onSelect: () => void): Interaction {
  const tools = paper.getLayerView('tools').el;
  const preview = tools.appendChild(document.createElementNS(SVG, 'g'));
  const chrome = tools.appendChild(document.createElementNS(SVG, 'g'));
  preview.setAttribute('data-preview', '');
  chrome.setAttribute('data-chrome', '');
  let selected = new Set<string>();
  let edge: string | undefined;
  let press: Press | undefined;
  let gesture: Gesture | undefined;
  let pointer: Point = { x: 0, y: 0 };
  let frame = 0;
  let zoom = 1;
  let space = false;

  const local = (client: Point) => {
    const p = paper.clientToLocalPoint(client.x, client.y);
    return { x: p.x, y: p.y };
  };
  const sc = () => host.scene();
  const cellEl = (id: string) => paper.findViewByModel(id)?.el as SVGElement | undefined;

  const el = (tag: string, a: Record<string, string | number>, parent: Element = chrome) => {
    const e = document.createElementNS(SVG, tag);
    for (const [k, v] of Object.entries(a)) e.setAttribute(k, String(v));
    parent.appendChild(e);
    return e;
  };
  const rect = (b: Box, cls: string, extra: Record<string, string | number> = {}) => el('rect', { x: b.x, y: b.y, width: b.w, height: b.h, class: cls, ...extra });
  const dot = (p: Point, handle: string, cls = 'dg-handle') => el('circle', { cx: p.x, cy: p.y, r: 5 / zoom, class: cls, 'data-handle': handle, 'stroke-width': 1.5 / zoom });

  // drawChrome draws the selection: outlines, with resize and port handles for a single node,
  // and the end, via and label handles of a selected edge; boxes moves some nodes.
  const drawChrome = (boxes = new Map<string, Box>()) => {
    chrome.replaceChildren();
    const s = sc();
    if (!s) return;
    const box = (id: string) => boxes.get(id) ?? s.nodes.get(id)!.box;
    for (const id of selected) rect(box(id), 'dg-selected', { 'stroke-width': 1.5 / zoom });
    if (selected.size === 1 && !gesture) {
      const id = [...selected][0];
      const n = s.nodes.get(id)!;
      const b = n.box;
      const corners: [Handle, Point][] = [
        ['nw', { x: b.x, y: b.y }],
        ['ne', { x: b.x + b.w, y: b.y }],
        ['sw', { x: b.x, y: b.y + b.h }],
        ['se', { x: b.x + b.w, y: b.y + b.h }],
      ];
      for (const [h, p] of corners) if (!n.orderedIn || h === 'se') rect({ x: p.x - 4 / zoom, y: p.y - 4 / zoom, w: 8 / zoom, h: 8 / zoom }, `dg-handle dg-${h}`, { 'data-handle': 'resize:' + h, 'stroke-width': 1.5 / zoom });
      const anchors = s.drawing.shapes[n.custom ?? '']?.anchors ?? {};
      for (const p of n.ports) if (!anchors[p.name]) dot(p, 'port:' + p.name, 'dg-handle dg-port');
    }
    const e = edge ? s.drawing.edges.find((x) => x.id === edge) : undefined;
    const ve = edge ? s.view.edges.find((x) => x.id === edge) : undefined;
    if (e && ve && !gesture) {
      el('path', { d: 'M' + e.points.map((p) => `${p.x} ${p.y}`).join(' L'), class: 'dg-edge-selected', 'stroke-width': 3 / zoom });
      const first = e.points[0];
      const last = e.points[e.points.length - 1];
      if (!ve.from.port && !ve.from.row) dot(first, 'end:0');
      if (!ve.to.port && !ve.to.row) dot(last, 'end:1');
      (ve.hint?.via ?? []).forEach(([x, y], i) => dot({ x, y }, 'via:' + i, 'dg-handle dg-via'));
      if (ve.labels.mid && e.labels[0]) rect(e.labels[0].box, 'dg-handle dg-label', { 'data-handle': 'label', 'stroke-width': 1.5 / zoom });
    }
  };

  const select = (ids: Iterable<string>, e?: string) => {
    const next = new Set(ids);
    if (e === edge && next.size === selected.size && [...next].every((id) => selected.has(id))) return;
    selected = next;
    edge = e;
    drawChrome();
    onSelect();
  };

  const send = (ops: Op[]) => {
    const op = batch(ops);
    return !!op && host.send(op);
  };

  // reshape draws node id with box b, which grown gave it.
  const reshape = (s: Scene, id: string, b: Box) => {
    const n = s.nodes.get(id)!;
    const cell = paper.model.getCell('n:' + id) as dia.Element | undefined;
    if (!cell) return;
    if (b.w !== cell.size().width || b.h !== cell.size().height) {
      const local = { ...shiftNode(n, -n.box.x, -n.box.y), box: { x: 0, y: 0, w: b.w, h: b.h } };
      cell.set({ size: { width: b.w, height: b.h }, markup: nodeMarkup(local, s.drawing, s.drawing).join('') });
    }
    translate('n:' + id, b, { x: 0, y: 0 });
  };

  // translate moves the drawn cell of a node, edge, overlay or decoration by d.
  const translate = (cellId: string, base: Point, d: Point) => cellEl(cellId)?.setAttribute('transform', `translate(${base.x + d.x},${base.y + d.y})`);

  // prepare starts the live routing of a move of ids, routing once while they are still.
  const prepare = (s: Scene, ids: string[]): Live => {
    const set = movedWith(s, ids);
    const boxes = new Map(s.drawing.nodes.map((n) => [n.id, n.box]));
    const routes = new Map(s.drawing.edges.map((e) => [e.id, e.points]));
    const live: Live = { ids, moved: set, edges: liveEdges(s.view, boxes, routes, set) };
    if (!live.edges.length) return live;
    const obstacles = new Map(s.drawing.nodes.filter((n) => !n.container).map((n) => [n.id, n.box]));
    const still = live.edges.map((e) => liveRequest(e, { x: 0, y: 0 }));
    live.router = liveRouter(obstacles, new Set([...set].filter((id) => obstacles.has(id))), still);
    live.router?.route(still, { x: 0, y: 0 });
    return live;
  };

  // moving returns the nodes a press on node id moves: the selection if it holds id, else id.
  const moving = (s: Scene, id: string, shift: boolean) => movable(s, selected.has(id) ? selected : shift ? [...selected, id] : [id]);

  const start = (p: Press) => {
    const s = sc()!;
    const at = p.at;
    if (p.pan) return { kind: 'pan', left: scroller.scrollLeft, top: scroller.scrollTop } as Gesture;
    if (!host.editable()) {
      return p.handle || p.node || p.edge ? undefined : ({ kind: 'box', p: at, add: p.shift } as Gesture);
    }
    if (p.handle) {
      const [kind, arg] = p.handle.split(/:(.*)/s);
      const id = [...selected][0];
      switch (kind) {
        case 'resize':
          return { kind: 'resize', id, handle: arg as Handle, box: s.nodes.get(id)!.box } as Gesture;
        case 'port':
          return { kind: 'port', id, port: arg, p: at } as Gesture;
        case 'end':
          return { kind: 'end', edge: edge!, end: Number(arg) as 0 | 1, p: at } as Gesture;
        case 'label':
          return { kind: 'label', edge: edge!, p: at } as Gesture;
        case 'via':
          return { kind: 'via', edge: edge!, index: Number(arg), p: at } as Gesture;
      }
    }
    if (p.node) {
      const n = s.nodes.get(p.node)!;
      if (n.orderedIn !== undefined) {
        select([p.node]);
        return { kind: 'reorder', id: p.node, p: at } as Gesture;
      }
      const ids = moving(s, p.node, p.shift);
      if (!selected.has(p.node)) select(p.shift ? [...selected, p.node] : [p.node]);
      if (!ids.length) return undefined;
      const live = p.live?.ids.join('\n') === ids.join('\n') ? p.live : prepare(s, ids);
      if (live !== p.live) p.live?.router?.destroy();
      p.live = undefined;
      return { kind: 'move', ids, moved: live.moved, d: { x: NaN, y: NaN }, live, room: new Set() } as Gesture;
    }
    return { kind: 'box', p: at, add: p.shift } as Gesture;
  };

  // update draws the gesture at the pointer.
  const update = () => {
    frame = 0;
    const s = sc();
    const g = gesture;
    if (!s || !g || !press) return;
    const p = local(pointer);
    const raw = { x: p.x - press.at.x, y: p.y - press.at.y };
    chrome.replaceChildren();
    switch (g.kind) {
      case 'pan':
        scroller.scrollLeft = g.left - (pointer.x - press.client.x);
        scroller.scrollTop = g.top - (pointer.y - press.client.y);
        break;
      case 'box': {
        g.p = p;
        drawChrome();
        rect(norm(press.at, p), 'dg-band', { 'stroke-width': 1 / zoom });
        break;
      }
      case 'move': {
        const d = snapDelta(s, g.ids, raw.x, raw.y);
        if (d.x === g.d.x && d.y === g.d.y) {
          drawChrome(movedBoxes(s, g.moved, d));
          break;
        }
        g.d = d;
        drawMove(s, g);
        break;
      }
      case 'reorder': {
        g.p = p;
        const boxes = movedBoxes(s, movedWith(s, [g.id]), raw);
        for (const [id, b] of boxes) {
          translate('n:' + id, s.nodes.get(id)!.box, { x: b.x - s.nodes.get(id)!.box.x, y: b.y - s.nodes.get(id)!.box.y });
          cellEl('n:' + id)?.setAttribute('opacity', String(DRAGGED_OPACITY));
        }
        drawChrome(boxes);
        const l = landing(s, g.id, p);
        if (l && 'line' in l.mark) {
          const [a, b] = l.mark.line;
          el('path', { d: `M${a.x} ${a.y} L${b.x} ${b.y}`, class: 'dg-side', 'stroke-width': 3 / zoom });
        } else if (l && 'box' in l.mark) rect(l.mark.box, 'dg-ghost', { 'stroke-width': 2 / zoom });
        break;
      }
      case 'resize': {
        const n = s.nodes.get(g.id)!;
        const frameOrigin = n.frame ? s.nodes.get(n.frame)!.origin : undefined;
        g.box = resizedBox(n.box, g.handle, raw.x, raw.y, s.view.grid, s.drawing.shapes[n.custom ?? '']?.stretch === 'uniform', frameOrigin);
        drawChrome(new Map([[g.id, g.box]]));
        rect(g.box, 'dg-ghost', { 'stroke-width': 1.5 / zoom });
        break;
      }
      case 'port': {
        g.p = p;
        const b = s.nodes.get(g.id)!.box;
        const pin = portPin(b, p);
        drawChrome();
        dot(onSide(b, pin.side, pin.offset), '', 'dg-handle dg-port');
        break;
      }
      case 'end': {
        g.p = p;
        const ve = s.view.edges.find((e) => e.id === g.edge)!;
        const b = s.nodes.get(g.end === 0 ? ve.from.node : ve.to.node)!.box;
        const side = nearestSide(b, p);
        const a = onSide(b, side, 0);
        const z = onSide(b, side, 1);
        drawChrome();
        el('path', { d: `M${a.x} ${a.y} L${z.x} ${z.y}`, class: 'dg-side', 'stroke-width': 4 / zoom });
        dot(onSide(b, side, 0.5), '');
        break;
      }
      case 'label': {
        g.p = p;
        const e = s.drawing.edges.find((x) => x.id === g.edge)!;
        const at = pointAlong(e.points, labelPos(e.points, p));
        const b = e.labels[0].box;
        drawChrome();
        rect({ x: at.x - b.w / 2, y: at.y - b.h / 2, w: b.w, h: b.h }, 'dg-ghost', { 'stroke-width': 1.5 / zoom });
        break;
      }
      case 'via': {
        g.p = p;
        drawChrome();
        dot(p, '', 'dg-handle dg-via');
        break;
      }
    }
  };

  // drawMove draws a move by g.d: the moved cells shifted, the containers around them grown, the
  // edges between moved nodes shifted and the edges with one end on them routed live.
  const drawMove = (s: Scene, g: Extract<Gesture, { kind: 'move' }>) => {
    const d = g.d;
    const boxes = movedBoxes(s, g.moved, d);
    for (const id of g.moved) translate('n:' + id, s.nodes.get(id)!.box, d);
    const room = grown(s, boxes);
    for (const id of g.room) if (!room.has(id)) reshape(s, id, s.nodes.get(id)!.box);
    for (const [id, b] of room) reshape(s, id, b);
    g.room = new Set(room.keys());
    const live = g.live;
    for (const e of s.view.edges) if (g.moved.has(e.from.node) && g.moved.has(e.to.node)) translate('e:' + e.id, { x: 0, y: 0 }, d);
    let routes = new Map<string, Point[]>();
    if (live.router) {
      const t = performance.now();
      routes = live.router.route(
        live.edges.map((e) => liveRequest(e, d)),
        d,
      );
      if (performance.now() - t > BUDGET) {
        live.router.destroy();
        live.router = null;
      }
    }
    const box = (id: string) => boxes.get(id) ?? s.nodes.get(id)!.box;
    const drawn = new Map(s.drawing.edges.map((e) => [e.id, e]));
    const periods = flowPeriods(s.drawing.edges);
    const markup: string[] = [];
    for (const le of live.edges) {
      const de = drawn.get(le.id)!;
      const ve = s.view.edges.find((e) => e.id === le.id)!;
      const pts = liveRoute(le, routes.get(le.id), d, box);
      cellEl('e:' + le.id)?.setAttribute('display', 'none');
      const labels = [...de.labels];
      let i = 0;
      if (ve.labels.mid) {
        const at = pointAlong(pts, ve.hint?.label_pos ?? 0.5);
        const b = labels[0].box;
        labels[0] = { ...labels[0], box: { ...b, x: at.x - b.w / 2, y: at.y - b.h / 2 } };
        i = 1;
      }
      for (const [text, end] of [[ve.labels.start, le.from], [ve.labels.end, le.to]] as const) {
        if (!text || !labels[i]) continue;
        if (end.moves) labels[i] = { ...labels[i], box: shift(labels[i].box, d) };
        i++;
      }
      const drawnEdge: DrawnEdge = { ...de, points: pts, labels, markers: [...marker(ve.style.arrow_end, pts), ...marker(ve.style.arrow_start, [...pts].reverse())] };
      markup.push(...edgeMarkup(drawnEdge, s.drawing, periods));
    }
    preview.innerHTML = markup.join('');
    preview.setAttribute('data-live', live.router ? 'routed' : live.edges.length ? 'lines' : 'none');
    drawChrome(boxes);
  };

  const drop = () => {
    const s = sc();
    const g = gesture;
    gesture = undefined;
    if (!s || !g || !press) return;
    const p = local(pointer);
    let sent = false;
    switch (g.kind) {
      case 'box': {
        const ids = inside(s.drawing, norm(press.at, p));
        select(g.add ? [...selected, ...ids] : ids);
        break;
      }
      case 'move':
        g.live.router?.destroy();
        if (g.d.x || g.d.y) sent = send(withPins(s, moveOps(s, g.ids, g.d.x, g.d.y), new Set(g.ids)));
        break;
      case 'reorder':
        sent = send(reorderOps(s, g.id, g.p));
        break;
      case 'resize': {
        const b = s.nodes.get(g.id)!.box;
        if (g.box.x !== b.x || g.box.y !== b.y || g.box.w !== b.w || g.box.h !== b.h) sent = send(resizeOps(s, g.id, g.box));
        break;
      }
      case 'port':
        sent = send(pinPortOps(s, g.id, g.port, g.p));
        break;
      case 'end': {
        const ve = s.view.edges.find((e) => e.id === g.edge)!;
        const side = nearestSide(s.nodes.get(g.end === 0 ? ve.from.node : ve.to.node)!.box, g.p);
        sent = send([hintOp(s, ve, g.end === 0 ? { from_side: side } : { to_side: side })]);
        break;
      }
      case 'label': {
        const e = s.drawing.edges.find((x) => x.id === g.edge)!;
        sent = send([hintOp(s, s.view.edges.find((x) => x.id === g.edge)!, { label_pos: labelPos(e.points, g.p) })]);
        break;
      }
      case 'via': {
        const ve = s.view.edges.find((e) => e.id === g.edge)!;
        const via = [...(ve.hint?.via ?? [])];
        via[g.index] = [Math.round(g.p.x / s.view.grid) * s.view.grid, Math.round(g.p.y / s.view.grid) * s.view.grid];
        sent = send([hintOp(s, ve, { via })]);
        break;
      }
      case 'pan':
        break;
    }
    if (!sent && g.kind !== 'box' && g.kind !== 'pan') host.restore();
    drawChrome();
    host.ended();
  };

  // cancel ends a gesture without sending anything.
  const cancel = () => {
    const g = gesture;
    gesture = undefined;
    press?.live?.router?.destroy();
    press = undefined;
    if (!g) return;
    if (g.kind === 'move') g.live.router?.destroy();
    if (g.kind !== 'box' && g.kind !== 'pan') host.restore();
    drawChrome();
    host.ended();
  };

  const target = (ev: PointerEvent | MouseEvent): Omit<Press, 'client' | 'shift' | 'pan'> => {
    const t = ev.target as Element;
    const at = local({ x: ev.clientX, y: ev.clientY });
    const handle = t.closest('[data-handle]')?.getAttribute('data-handle') || undefined;
    if (handle) return { at, target: t, handle };
    const node = t.closest('[data-node]')?.getAttribute('data-node') ?? undefined;
    const s = sc();
    let e: string | undefined;
    if (s) {
      e = s.drawing.edges.find((x) => x.labels.some((l) => within(l.box, at)))?.id ?? edgeAt(s.drawing, at, HIT / zoom);
      if (node && e) {
        const n = s.nodes.get(node)!;
        if (!n.container) e = undefined;
      }
    }
    return { at, target: t, ...(e ? { edge: e } : node ? { node } : {}) };
  };

  paper.el.addEventListener('pointerdown', (ev) => {
    menu.hidden = true;
    if (ev.button === 2 || !sc()) return;
    if (gesture) return;
    const t = target(ev);
    press = { ...t, client: { x: ev.clientX, y: ev.clientY }, shift: ev.shiftKey, pan: ev.button === 1 || space };
    pointer = press.client;
    const s = sc()!;
    if (press.node && !press.pan && host.editable() && s.nodes.get(press.node)!.orderedIn === undefined) {
      const ids = moving(s, press.node, press.shift);
      if (ids.length) press.live = prepare(s, ids);
    }
    // Keys need the focus, which a press inside a frame, such as the VS Code panel's, does not
    // always move there; the middle button's autoscroll is in the way.
    window.focus();
    if (ev.button === 1) ev.preventDefault();
  });
  paper.el.addEventListener('pointermove', (ev) => {
    if (!press) return;
    pointer = { x: ev.clientX, y: ev.clientY };
    if (!gesture) {
      if (Math.hypot(pointer.x - press.client.x, pointer.y - press.client.y) < DRAG) return;
      gesture = start(press);
      if (!gesture) {
        press = undefined;
        return;
      }
      // Captured only once dragging, so a click keeps its target, such as a drill-down badge.
      paper.el.setPointerCapture(ev.pointerId);
    }
    frame ||= requestAnimationFrame(update);
  });
  paper.el.addEventListener('pointerup', (ev) => {
    if (!press) return;
    pointer = { x: ev.clientX, y: ev.clientY };
    if (frame) {
      cancelAnimationFrame(frame);
      update();
    }
    press.live?.router?.destroy();
    if (gesture) drop();
    else if (!press.pan) {
      if (press.node) select(press.shift ? toggle(selected, press.node) : [press.node]);
      else if (press.edge) select([], press.edge);
      else if (!press.handle && !press.shift) select([]);
    }
    press = undefined;
  });
  paper.el.addEventListener('pointercancel', cancel);

  // A double click on a via point of the selected edge removes it; elsewhere on the edge it adds
  // one there.
  paper.el.addEventListener('dblclick', (ev) => {
    const s = sc();
    if (!s || !host.editable()) return;
    const ve = edge ? s.view.edges.find((e) => e.id === edge) : undefined;
    if (!ve) return;
    const p = local({ x: ev.clientX, y: ev.clientY });
    const via = ve.hint?.via ?? [];
    const hit = via.findIndex(([x, y]) => Math.hypot(x - p.x, y - p.y) <= HIT / zoom);
    const e = s.drawing.edges.find((x) => x.id === edge)!;
    if (hit >= 0) send([hintOp(s, ve, { via: via.filter((_, i) => i !== hit) })]);
    else if (edgeAt({ ...s.drawing, edges: [e] }, p, HIT / zoom)) send([hintOp(s, ve, { via: withVia(e.points, via, p, s.view.grid) })]);
  });

  const actions = (): [string, () => void, boolean][] => {
    const s = sc();
    const n = s ? movable(s, selected).length : 0;
    const ve = edge && s?.view.edges.find((e) => e.id === edge);
    return [
      ['Unpin', () => api.unpin(), !!s && unpinOps(s, selected).length > 0],
      ['Align left', () => api.arrange('left'), n > 1],
      ['Align center', () => api.arrange('center'), n > 1],
      ['Align right', () => api.arrange('right'), n > 1],
      ['Align top', () => api.arrange('top'), n > 1],
      ['Align middle', () => api.arrange('middle'), n > 1],
      ['Align bottom', () => api.arrange('bottom'), n > 1],
      ['Distribute horizontally', () => api.arrange('x'), n > 2],
      ['Distribute vertically', () => api.arrange('y'), n > 2],
      ['Reset route', () => s && ve && ve.hint && send([{ op: 'set_edge_hint', view: s.view.view, id: ve.id, hint: {} }]), !!ve && !!ve.hint],
    ];
  };

  paper.el.addEventListener('contextmenu', (ev) => {
    ev.preventDefault();
    const s = sc();
    if (!s || gesture) return;
    const t = target(ev);
    if (t.node && !selected.has(t.node)) select([t.node]);
    else if (t.edge) select([], t.edge);
    const list = actions().filter(([, , on]) => on);
    if (!list.length || !host.editable()) return;
    menu.replaceChildren(
      ...list.map(([label, run]) => {
        const b = document.createElement('button');
        b.type = 'button';
        b.textContent = label;
        b.addEventListener('click', () => {
          menu.hidden = true;
          run();
        });
        return b;
      }),
    );
    menu.style.left = `${ev.pageX}px`;
    menu.style.top = `${ev.pageY}px`;
    menu.hidden = false;
  });

  document.addEventListener('click', (ev) => {
    if (!menu.contains(ev.target as Node)) menu.hidden = true;
  });

  scroller.addEventListener(
    'wheel',
    (ev) => {
      if (!ev.ctrlKey && !ev.metaKey) return;
      ev.preventDefault();
      api.setZoom(zoom * Math.exp(-ev.deltaY / 300), { x: ev.clientX, y: ev.clientY });
    },
    { passive: false },
  );

  document.addEventListener('keydown', (ev) => {
    const tag = (ev.target as Element).tagName;
    if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA') return;
    if (ev.key === ' ') space = true;
    const mod = ev.ctrlKey || ev.metaKey;
    if (ev.key === 'Escape') {
      if (gesture) cancel();
      else select([]);
    } else if (mod && ev.key.toLowerCase() === 'a') {
      const s = sc();
      if (s) select(s.drawing.nodes.map((n) => n.id));
    } else if (!mod && ev.key.toLowerCase() === 'u') {
      api.unpin();
    } else if (!mod && (ev.key === '+' || ev.key === '=')) {
      api.setZoom(step(zoom, 1));
    } else if (!mod && ev.key === '-') {
      api.setZoom(step(zoom, -1));
    } else if (!mod && ev.key === '0') {
      api.setZoom(1);
    } else return;
    ev.preventDefault();
  });
  document.addEventListener('keyup', (ev) => {
    if (ev.key === ' ') space = false;
  });

  const api = {
    shown() {
      const s = sc();
      if (s) {
        selected = new Set([...selected].filter((id) => s.nodes.has(id)));
        if (edge && !s.drawing.edges.some((e) => e.id === edge)) edge = undefined;
      }
      preview.replaceChildren();
      drawChrome();
      onSelect();
    },
    busy: () => !!gesture,
    zoom: () => zoom,
    can(action: 'unpin' | 2 | 3) {
      const s = sc();
      if (!s) return false;
      return action === 'unpin' ? unpinOps(s, selected).length > 0 : movable(s, selected).length >= action;
    },
    arrange(how: Align | 'x' | 'y') {
      const s = sc();
      if (!s || !host.editable()) return;
      send(how === 'x' || how === 'y' ? distributeOps(s, selected, how) : alignOps(s, selected, how));
    },
    unpin() {
      const s = sc();
      if (s && host.editable()) send(unpinOps(s, selected));
    },
    setZoom(z: number, at?: Point) {
      const next = Math.min(4, Math.max(0.25, z));
      if (next === zoom) return;
      const r = scroller.getBoundingClientRect();
      const c = at ?? { x: r.left + r.width / 2, y: r.top + r.height / 2 };
      const fixed = local(c);
      zoom = next;
      onZoom();
      const moved = paper.localToClientPoint(fixed.x, fixed.y);
      scroller.scrollLeft += moved.x - c.x;
      scroller.scrollTop += moved.y - c.y;
      drawChrome();
    },
  };
  return api;
}

function movedBoxes(s: Scene, moved: Set<string>, d: Point): Map<string, Box> {
  return new Map([...moved].map((id) => [id, shift(s.nodes.get(id)!.box, d)]));
}

function shift(b: Box, d: Point): Box {
  return { ...b, x: b.x + d.x, y: b.y + d.y };
}

function norm(a: Point, b: Point): Box {
  return { x: Math.min(a.x, b.x), y: Math.min(a.y, b.y), w: Math.abs(a.x - b.x), h: Math.abs(a.y - b.y) };
}

function within(b: Box, p: Point): boolean {
  return p.x >= b.x && p.x <= b.x + b.w && p.y >= b.y && p.y <= b.y + b.h;
}

function toggle(set: Set<string>, id: string): string[] {
  return set.has(id) ? [...set].filter((x) => x !== id) : [...set, id];
}

function step(z: number, dir: 1 | -1): number {
  if (dir > 0) return ZOOMS.find((v) => v > z + 1e-9) ?? z;
  return [...ZOOMS].reverse().find((v) => v < z - 1e-9) ?? z;
}

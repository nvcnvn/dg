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
import { connectOp, copy, dropTarget, endAt, pasteOps, reparentOps, type Clip } from '../edit/model';
import { roundTo, type Box, type Point } from '../geometry/geometry';
import type { Result } from '../protocol/client';
import type { End, Labels } from '../protocol/view';
import { liveEdges, liveRequest, liveRoute, type LiveEdge } from '../route/live';
import { nearestSide } from '../route/edges';
import { marker } from '../route/markers';
import { onSide, pointAlong } from '../route/route';
import { liveRouter, type LiveRouter } from './avoid';
import type { Actions, Selection } from './actions';
import { menuAndKeys } from './menu';
import { editInPlace } from './inline';
import { edgeMarkup, flowPeriods, nodeMarkup, shiftNode, type DrawnEdge } from './markup';

// Host is what the interactions need from the editor.
export interface Host {
  scene(): Scene | undefined;
  // editable reports whether layout edits can be sent: the project parses, the server is
  // connected and no edit is waiting for its answer and drawing.
  editable(): boolean;
  // send sends an operation, and reports whether it did; then gets its results once the
  // drawing it brings is shown.
  send(op: Op, then?: (results: Result[]) => void): boolean;
  // restore draws the current drawing again, dropping what a gesture moved.
  restore(): void;
  // ended is called when a gesture ends, so a drawing that arrived during it can be shown.
  ended(): void;
  // status shows a message, such as why a gesture was refused.
  status(message: string): void;
  // toast offers to undo a change that was sent (spec 7.6).
  toast(message: string): void;
  // edgeKind is the kind of new edges.
  edgeKind(): string;
  selectView(id: string): void;
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
const SVG = 'http://www.w3.org/2000/svg';

type Gesture =
  | { kind: 'move'; ids: string[]; moved: Set<string>; d: Point; live: Live; room: Set<string>; alt: boolean }
  | { kind: 'connect'; from: End; p: Point }
  | { kind: 'decoration'; id: string; d: Point }
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
  decoration?: string;
  // end is where an edge drawn in connect mode starts.
  end?: End;
  handle?: string;
  pan: boolean;
  alt: boolean;
  // live is prepared when the press is on a node that a drag would move.
  live?: Live;
}

export interface Interaction {
  // shown is called after a drawing is shown, to draw the selection again.
  shown(): void;
  busy(): boolean;
  selection(): Selection;
  select(sel: Selection): void;
  // editLabel edits the label of node id in place.
  editLabel(id: string): void;
  // connecting reports whether connect mode is on, and toggles it with on.
  connecting(on?: boolean): boolean;
  // center is the point in the middle of what the canvas shows.
  center(): Point;
  // at returns the drawing's point under the client point, if the canvas shows it.
  at(client: Point): Point | undefined;
  zoom(): number;
  // can reports whether the unpin, align (2) and distribute (3) actions apply to the selection.
  can(action: 'unpin' | 2 | 3): boolean;
  arrange(how: Align | 'x' | 'y'): void;
  unpin(): void;
  setZoom(z: number): void;
}

// interact handles the gestures on the paper (FR-11): selection by click, shift-click and box;
// pan and zoom; move, resize, reorder, port, edge end, label and via drags; re-parenting with
// Alt; drawing edges in connect mode; moving decorations; label editing in place; copy and
// paste; each sending one operation or batch on drop; and the context menu and keys.
export function interact(paper: dia.Paper, scroller: HTMLElement, menu: HTMLElement, host: Host, act: Actions, onZoom: () => void, onSelect: () => void): Interaction {
  const tools = paper.getLayerView('tools').el;
  const locks = tools.appendChild(document.createElementNS(SVG, 'g'));
  const preview = tools.appendChild(document.createElementNS(SVG, 'g'));
  const chrome = tools.appendChild(document.createElementNS(SVG, 'g'));
  locks.setAttribute('data-locks', '');
  preview.setAttribute('data-preview', '');
  chrome.setAttribute('data-chrome', '');
  let selected = new Set<string>();
  let edge: string | undefined;
  let deco: string | undefined;
  let connecting = false;
  let editing = false;
  let clip: Clip | undefined;
  // pasted is where the last paste went, and how many pastes went there.
  let pasted: (Point & { n: number }) | undefined;
  // hover is the drawing's point last under the pointer, while it is over the canvas.
  let hover: Point | undefined;
  let press: Press | undefined;
  let gesture: Gesture | undefined;
  let pointer: Point = { x: 0, y: 0 };
  let frame = 0;
  let zoom = 1;
  let space = false;
  // alt is whether the re-parent modifier was down at the pointer's last event.
  let alt = false;

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
    const dd = deco ? s.drawing.decorations.find((x) => x.id === deco) : undefined;
    if (dd) rect(dd.box, 'dg-selected', { 'stroke-width': 1.5 / zoom, 'data-selected-decoration': dd.id });
    if (selected.size === 1 && !gesture && !connecting) {
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

  const select = (ids: Iterable<string>, e?: string, d?: string) => {
    const next = new Set(ids);
    if (e === edge && d === deco && next.size === selected.size && [...next].every((id) => selected.has(id))) return;
    selected = next;
    edge = e;
    deco = d;
    drawChrome();
    onSelect();
  };

  const send = (ops: Op[], then?: (results: Result[]) => void) => {
    const op = batch(ops);
    return !!op && host.send(op, then);
  };

  // drawLocks marks the nodes defined in read-only files with a lock in their top right corner.
  const drawLocks = () => {
    locks.replaceChildren();
    const s = sc();
    if (!s) return;
    for (const n of s.view.nodes) {
      const b = n.read_only && s.nodes.get(n.id)?.box;
      if (!b) continue;
      const g = el('g', { 'data-lock': n.id, transform: `translate(${b.x + b.w - 16},${b.y + 3})`, fill: '#5F6368' }, locks);
      el('title', {}, g).textContent = 'Read-only: only the layout can change';
      el('rect', { x: 1, y: 6, width: 10, height: 7, rx: 1 }, g);
      el('path', { d: 'M3 6 V4 a3 3 0 0 1 6 0 V6', fill: 'none', stroke: '#5F6368', 'stroke-width': 1.5 }, g);
    }
  };

  const readOnly = (id: string) => !!sc()?.model.nodes.find((n) => n.id === id)?.read_only;

  // endPoint is where an edge end is drawn from while connecting: a port's point, the middle of
  // a row's line, or a node's center.
  const endPoint = (s: Scene, e: End): Point => {
    const n = s.nodes.get(e.node)!;
    const port = e.port && n.ports.find((p) => p.name === e.port);
    if (port) return port;
    const row = e.row && n.table?.rows.find((r) => r.name === e.row);
    return { x: n.box.x + n.box.w / 2, y: row ? row.y : n.box.y + n.box.h / 2 };
  };

  // editText edits text in place over box, drawn at fontSize, and commits it unless Escape is
  // pressed.
  const editText = (box: Box, value: string, fontSize: number, label: string, commit: (v: string) => void) => {
    const tl = paper.localToPagePoint(box.x, box.y);
    editing = true;
    editInPlace({ x: tl.x, y: tl.y, w: box.w * zoom, h: box.h * zoom }, value, fontSize * zoom, label, (text) => {
      editing = false;
      host.ended();
      if (text !== undefined && text !== value) commit(text);
    });
  };

  const editLabel = (id: string) => {
    const s = sc();
    const n = s?.nodes.get(id);
    const mn = s?.model.nodes.find((x) => x.id === id);
    if (!s || !n || !mn || !host.editable() || editing) return;
    if (mn.read_only) return host.status(`Node "${id}" is defined in a read-only file; its label cannot change.`);
    select([id]);
    editText(n.box, mn.label, n.style.font_size, `Label of ${id}`, (label) => host.send({ op: 'set_label', id, label }, act.pick('set_label', 'node')));
  };

  const editEdgeLabel = (id: string) => {
    const s = sc();
    const e = s?.model.edges.find((x) => x.id === id);
    const de = s?.drawing.edges.find((x) => x.id === id);
    if (!s || !e || !de || !host.editable() || editing) return;
    if (e.read_only) return host.status(`Edge "${id}" is defined in a read-only file; its label cannot change.`);
    const at = e.labels.mid && de.labels[0] ? de.labels[0].box : (() => {
      const p = pointAlong(de.points, 0.5);
      return { x: p.x - 60, y: p.y - 12, w: 120, h: 24 };
    })();
    editText(at, e.labels.mid ?? '', de.style.font_size, `Label of ${id}`, (mid) => {
      const labels: Labels = { ...e.labels };
      if (mid) labels.mid = mid;
      else delete labels.mid;
      host.send({ op: 'set_edge_labels', id, ...(Object.keys(labels).length ? { labels } : {}) });
    });
  };

  const doCopy = () => {
    const s = sc();
    if (s && selected.size) clip = copy(s, selected);
  };

  // paste pastes what was copied with its top left corner at p: the given point, else the
  // pointer's, else two grid units from where the copies were.
  const paste = (p?: Point) => {
    const s = sc();
    if (!s || !clip || !host.editable()) return;
    const tops = clip.nodes.filter((c) => !c.frame);
    const base = p ?? hover ?? { x: Math.min(...tops.map((c) => c.box.x)) + 2 * s.view.grid, y: Math.min(...tops.map((c) => c.box.y)) + 2 * s.view.grid };
    // Pasting again at the same point steps each copy a grid unit further, so none hides another.
    pasted = pasted && pasted.x === base.x && pasted.y === base.y ? { ...base, n: pasted.n + 1 } : { ...base, n: 0 };
    const step = pasted.n * s.view.grid;
    const op = pasteOps(s, clip, { x: base.x + step, y: base.y + step });
    if (op) host.send(op, act.pick('create_node', 'node'));
  };

  // remove deletes the selection, unless part of it is in a read-only file.
  const remove = (recursive = false) => {
    const sel = api.selection();
    if (sel.edge && !sc()?.model.edges.some((e) => e.id === sel.edge)) return host.status('A lifted edge stands for the edges inside a collapsed container; delete those in a view that draws them.');
    const ro = sel.nodes.find(readOnly) ?? (sel.edge && sc()?.model.edges.find((e) => e.id === sel.edge)?.read_only ? sel.edge : undefined);
    if (ro) return host.status(`"${ro}" is defined in a read-only file; it cannot be deleted.`);
    if (host.editable()) act.remove(sel, recursive);
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
      return p.handle || p.node || p.edge || p.decoration || p.end ? undefined : ({ kind: 'box', p: at, add: p.shift } as Gesture);
    }
    if (p.end) return { kind: 'connect', from: p.end, p: at } as Gesture;
    if (p.decoration) {
      select([], undefined, p.decoration);
      return { kind: 'decoration', id: p.decoration, d: { x: 0, y: 0 } } as Gesture;
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
      if (n.orderedIn !== undefined && !p.alt) {
        select([p.node]);
        return { kind: 'reorder', id: p.node, p: at } as Gesture;
      }
      const ids = n.orderedIn !== undefined ? [p.node] : moving(s, p.node, p.shift);
      if (!selected.has(p.node)) select(p.shift ? [...selected, p.node] : [p.node]);
      if (!ids.length) return undefined;
      const live = p.live?.ids.join('\n') === ids.join('\n') ? p.live : prepare(s, ids);
      if (live !== p.live) p.live?.router?.destroy();
      p.live = undefined;
      return { kind: 'move', ids, moved: live.moved, d: { x: NaN, y: NaN }, live, room: new Set(), alt: p.alt } as Gesture;
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
        g.alt = alt;
        const d = alt ? { x: roundTo(raw.x, s.view.grid), y: roundTo(raw.y, s.view.grid) } : snapDelta(s, g.ids, raw.x, raw.y);
        if (d.x === g.d.x && d.y === g.d.y) drawChrome(movedBoxes(s, g.moved, d));
        else {
          g.d = d;
          drawMove(s, g);
        }
        // With the re-parent modifier the container the node would go into is highlighted.
        if (alt) {
          const t = dropTarget(s, p, g.moved);
          if (t) rect(s.nodes.get(t)!.box, 'dg-target', { 'stroke-width': 2 / zoom, 'data-target': t });
        }
        break;
      }
      case 'connect': {
        g.p = p;
        drawChrome();
        const a = endPoint(s, g.from);
        el('path', { d: `M${a.x} ${a.y} L${p.x} ${p.y}`, class: 'dg-rubber', 'stroke-width': 1.5 / zoom });
        const to = endAt(s, p, HIT / zoom);
        if (to) rect(s.nodes.get(to.node)!.box, 'dg-target', { 'stroke-width': 2 / zoom, 'data-target': to.node });
        break;
      }
      case 'decoration': {
        g.d = { x: roundTo(raw.x, s.view.grid), y: roundTo(raw.y, s.view.grid) };
        translate('d:' + g.id, { x: 0, y: 0 }, g.d);
        drawChrome();
        const dd = s.drawing.decorations.find((x) => x.id === g.id);
        if (dd) rect(shift(dd.box, g.d), 'dg-ghost', { 'stroke-width': 1.5 / zoom });
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
    const room = g.alt ? new Map<string, Box>() : grown(s, boxes);
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
      case 'move': {
        g.live.router?.destroy();
        const r = g.alt ? reparentOps(s, g.ids, g.d, p) : undefined;
        if (r?.refused) host.status(r.refused);
        else if (r?.ops) {
          const what = g.ids.length === 1 ? `"${g.ids[0]}"` : `${g.ids.length} nodes`;
          const message = r.target ? `Moved ${what} into "${r.target}".` : `Moved ${what} out of its container.`;
          sent = send(r.ops, () => host.toast(message));
        } else {
          const d = g.alt ? snapDelta(s, g.ids, g.d.x, g.d.y) : g.d;
          if ((d.x || d.y) && !g.ids.some((id) => s.nodes.get(id)!.orderedIn !== undefined)) sent = send(withPins(s, moveOps(s, g.ids, d.x, d.y), new Set(g.ids)));
        }
        break;
      }
      case 'connect': {
        const to = endAt(s, p, HIT / zoom);
        if (to) sent = send([connectOp(s, g.from, to, host.edgeKind())], act.pick('connect', 'edge'));
        break;
      }
      case 'decoration': {
        const vd = s.view.decorations?.find((x) => x.id === g.id);
        if (vd && (g.d.x || g.d.y)) {
          const decoration = vd.attach ? { dx: vd.x + g.d.x, dy: vd.y + g.d.y } : { x: vd.x + g.d.x, y: vd.y + g.d.y };
          sent = send([{ op: 'update_decoration', view: s.view.view, id: g.id, decoration }]);
        }
        break;
      }
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
    if (!sent && g.kind !== 'box' && g.kind !== 'pan' && g.kind !== 'connect') host.restore();
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

  const target = (ev: PointerEvent | MouseEvent): Omit<Press, 'client' | 'shift' | 'pan' | 'alt'> => {
    const t = ev.target as Element;
    const at = local({ x: ev.clientX, y: ev.clientY });
    const handle = t.closest('[data-handle]')?.getAttribute('data-handle') || undefined;
    if (handle) return { at, target: t, handle };
    const s = sc();
    if (connecting && s) {
      const end = endAt(s, at, HIT / zoom);
      if (end) return { at, target: t, end };
    }
    const decoration = t.closest('[data-decoration]')?.getAttribute('data-decoration') ?? undefined;
    if (decoration) return { at, target: t, decoration };
    const node = t.closest('[data-node]')?.getAttribute('data-node') ?? undefined;
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
    menuCtl.close();
    // A press on the canvas takes the keys from a control around it, such as a toolbar button.
    if (document.activeElement instanceof HTMLElement && document.activeElement !== document.body) document.activeElement.blur();
    if (ev.button === 2 || !sc()) return;
    if (gesture || editing) return;
    alt = ev.altKey;
    const t = target(ev);
    press = { ...t, client: { x: ev.clientX, y: ev.clientY }, shift: ev.shiftKey, pan: ev.button === 1 || space, alt };
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
  scroller.addEventListener('pointermove', (ev) => {
    hover = local({ x: ev.clientX, y: ev.clientY });
  });
  scroller.addEventListener('pointerleave', () => {
    hover = undefined;
  });
  paper.el.addEventListener('pointermove', (ev) => {
    if (!press) return;
    pointer = { x: ev.clientX, y: ev.clientY };
    alt = ev.altKey;
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
    alt = ev.altKey;
    if (frame) {
      cancelAnimationFrame(frame);
      update();
    }
    press.live?.router?.destroy();
    if (gesture) drop();
    else if (!press.pan) {
      const node = press.node ?? press.end?.node;
      if (node) select(press.shift ? toggle(selected, node) : [node]);
      else if (press.decoration) select([], undefined, press.decoration);
      else if (press.edge) select([], press.edge);
      else if (!press.handle && !press.shift) select([]);
    }
    press = undefined;
  });
  paper.el.addEventListener('pointercancel', cancel);

  // A double click on an edge's middle label edits it; on a via point of the selected edge it
  // removes the point, elsewhere on that edge it adds one there; on a node it edits its label.
  paper.el.addEventListener('dblclick', (ev) => {
    const s = sc();
    if (!s || !host.editable() || connecting) return;
    const t = target(ev);
    const p = t.at;
    const onLabel = t.handle === 'label' || (t.edge && s.drawing.edges.find((x) => x.id === t.edge)?.labels.some((l, i) => i === 0 && s.view.edges.find((v) => v.id === t.edge)?.labels.mid && within(l.box, p)));
    if (onLabel && (t.edge ?? edge)) return editEdgeLabel((t.edge ?? edge)!);
    const ve = edge ? s.view.edges.find((e) => e.id === edge) : undefined;
    if (ve) {
      const via = ve.hint?.via ?? [];
      const hit = via.findIndex(([x, y]) => Math.hypot(x - p.x, y - p.y) <= HIT / zoom);
      const e = s.drawing.edges.find((x) => x.id === edge)!;
      if (hit >= 0) return void send([hintOp(s, ve, { via: via.filter((_, i) => i !== hit) })]);
      if (edgeAt({ ...s.drawing, edges: [e] }, p, HIT / zoom)) return void send([hintOp(s, ve, { via: withVia(e.points, via, p, s.view.grid) })]);
    }
    if (t.node) editLabel(t.node);
  });

  paper.el.addEventListener('contextmenu', (ev) => {
    ev.preventDefault();
    const s = sc();
    if (!s || gesture) return;
    const t = target(ev);
    const node = t.node ?? t.end?.node;
    if (node && !selected.has(node)) select([node]);
    else if (t.decoration) select([], undefined, t.decoration);
    else if (t.edge) select([], t.edge);
    else if (!node) select([]);
    menuCtl.open(ev.pageX, ev.pageY, t.at);
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

  const api = {
    shown() {
      const s = sc();
      if (s) {
        selected = new Set([...selected].filter((id) => s.nodes.has(id)));
        if (edge && !s.drawing.edges.some((e) => e.id === edge)) edge = undefined;
        if (deco && !s.drawing.decorations.some((d) => d.id === deco)) deco = undefined;
      }
      preview.replaceChildren();
      drawLocks();
      drawChrome();
      onSelect();
    },
    busy: () => !!gesture || editing,
    selection: (): Selection => ({ nodes: [...selected].sort(), ...(edge ? { edge } : {}), ...(deco ? { decoration: deco } : {}) }),
    select(sel: Selection) {
      select(sel.nodes, sel.edge, sel.decoration);
    },
    editLabel,
    connecting(on?: boolean) {
      if (on !== undefined && on !== connecting) {
        connecting = on;
        document.body.classList.toggle('connecting', on);
        drawChrome();
        onSelect();
      }
      return connecting;
    },
    center(): Point {
      const r = scroller.getBoundingClientRect();
      return local({ x: r.left + r.width / 2, y: r.top + r.height / 2 });
    },
    at(client: Point): Point | undefined {
      const r = scroller.getBoundingClientRect();
      if (client.x < r.left || client.x > r.right || client.y < r.top || client.y > r.bottom) return undefined;
      return local(client);
    },
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
  const menuCtl = menuAndKeys(menu, {
    scene: sc,
    editable: () => host.editable(),
    selection: api.selection,
    select: api.select,
    hasClip: () => !!clip,
    copy: doCopy,
    paste,
    remove,
    editLabel,
    editEdgeLabel,
    unpin: api.unpin,
    arrange: api.arrange,
    resetRoute: (id) => {
      const s = sc();
      if (s) send([{ op: 'set_edge_hint', view: s.view.view, id, hint: {} }]);
    },
    connecting: api.connecting,
    cancel: () => {
      const had = !!gesture;
      if (had) cancel();
      return had;
    },
    zoom: () => zoom,
    setZoom: (z) => api.setZoom(z),
    center: api.center,
    pagePoint: (p) => paper.localToPagePoint(p.x, p.y),
    selectView: host.selectView,
    space: (down) => {
      space = down;
    },
  }, act);
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

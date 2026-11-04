import type { Drawing } from '../canvas/export';
import type { DrawnNode } from '../canvas/markup';
import { ceilTo, cmp, roundTo, type Box, type Point } from '../geometry/geometry';
import type { ModelInfo } from '../protocol/client';
import type { Op } from '../protocol/ops';
import type { Hint, ResolvedView, Side, ViewEdge } from '../protocol/view';
import { nearestSide } from '../route/edges';

// Operations (spec FR-12) that layout gestures send. Every gesture is one operation or one batch,
// so it is one undo step (FR-11).

export type { Op };

// batch returns ops as one operation: nothing, the only one, or a batch.
export function batch(ops: Op[]): Op | undefined {
  if (ops.length < 2) return ops[0];
  return { op: 'batch', ops };
}

// Scene is a drawing with the resolved view it was drawn from, indexed by ID, and the model it
// shows part of.
export interface Scene {
  view: ResolvedView;
  drawing: Drawing;
  nodes: Map<string, DrawnNode>;
  model: ModelInfo;
}

const NO_MODEL: ModelInfo = { kinds: [], edge_kinds: [], tags: [], nodes: [], edges: [] };

export function scene(view: ResolvedView, drawing: Drawing, model: ModelInfo = NO_MODEL): Scene {
  return { view, drawing, nodes: new Map(drawing.nodes.map((n) => [n.id, n])), model };
}

// viewLabelOp sets the label the scene's view draws node id with, or, empty, removes it.
export function viewLabelOp(s: Scene, id: string, label: string): Op {
  return { op: 'set_view_label', view: s.view.view, id, ...(label ? { label } : {}) };
}

function resolved(s: Scene, id: string) {
  return s.view.nodes.find((n) => n.id === id);
}

// ancestors returns the frames of node id, innermost first.
export function ancestors(s: Scene, id: string): string[] {
  const out: string[] = [];
  for (let f = s.nodes.get(id)?.frame; f; f = s.nodes.get(f)?.frame) out.push(f);
  return out;
}

// movable returns the nodes of ids a move drags, in ID order: those a stack or grid container
// does not place, without the ones inside another of ids, which move with it.
export function movable(s: Scene, ids: Iterable<string>): string[] {
  const set = new Set(ids);
  return [...set].filter((id) => s.nodes.has(id) && !s.nodes.get(id)!.orderedIn && !ancestors(s, id).some((a) => set.has(a))).sort(cmp);
}

// moved returns the nodes that move with ids: they and everything inside them.
export function moved(s: Scene, ids: string[]): Set<string> {
  const set = new Set(ids);
  return new Set(s.drawing.nodes.filter((n) => set.has(n.id) || ancestors(s, n.id).some((a) => set.has(a))).map((n) => n.id));
}

// pinnedSize returns which sides of node id's size its layout entry pins.
function pinnedSize(s: Scene, id: string): { w: boolean; h: boolean } {
  const pin = resolved(s, id)?.pin;
  return { w: pin?.w !== undefined, h: pin?.h !== undefined };
}

// clampDelta keeps a move of ids by (dx, dy) inside each node's frame (spec 7.6): a node never
// moves above or left of its container's content origin, nor, in a container whose size is
// pinned, past its inside. The canvas and a container without a pinned size do not limit it.
export function clampDelta(s: Scene, ids: string[], dx: number, dy: number): Point {
  let [lx, hx, ly, hy] = [-Infinity, Infinity, -Infinity, Infinity];
  for (const id of ids) {
    const n = s.nodes.get(id)!;
    if (!n.frame || !n.rel) continue;
    const f = s.nodes.get(n.frame)!;
    const o = f.origin ?? f.box;
    lx = Math.max(lx, -n.rel.x);
    ly = Math.max(ly, -n.rel.y);
    const pad = f.style.padding;
    const fixed = pinnedSize(s, f.id);
    if (fixed.w) hx = Math.min(hx, Math.max(-n.rel.x, f.box.x + f.box.w - pad - o.x - n.box.w - n.rel.x));
    if (fixed.h) hy = Math.min(hy, Math.max(-n.rel.y, f.box.y + f.box.h - pad - o.y - n.box.h - n.rel.y));
  }
  return { x: Math.min(Math.max(dx, lx), hx), y: Math.min(Math.max(dy, ly), hy) };
}

// snapDelta returns the move by (dx, dy), on the grid and kept inside the frames.
export function snapDelta(s: Scene, ids: string[], dx: number, dy: number): Point {
  return clampDelta(s, ids, roundTo(dx, s.view.grid), roundTo(dy, s.view.grid));
}

// grown returns the boxes of the nodes that make room for nodes moved to boxes (spec 7.6): every
// frame around them without a pinned size grows, innermost first, as section 7.2 fits it; in a
// stack, a child that grew pushes the children after it along the stack, which grows by as much.
export function grown(s: Scene, boxes: Map<string, Box>): Map<string, Box> {
  const out = new Map<string, Box>();
  const g = s.view.grid;
  const box = (id: string) => out.get(id) ?? boxes.get(id) ?? s.nodes.get(id)!.box;
  const was = (id: string) => s.nodes.get(id)!.box;
  const push = (id: string, dx: number, dy: number) => {
    for (const n of s.drawing.nodes) {
      if (n.id === id || ancestors(s, n.id).includes(id)) out.set(n.id, { ...box(n.id), x: box(n.id).x + dx, y: box(n.id).y + dy });
    }
  };
  const frames = [...new Set([...boxes.keys()].flatMap((id) => ancestors(s, id)))]
    .filter((f) => !boxes.has(f) && !ancestors(s, f).some((a) => boxes.has(a)))
    .sort((a, b) => ancestors(s, b).length - ancestors(s, a).length || cmp(a, b));
  for (const f of frames) {
    const fb = box(f);
    const n = s.nodes.get(f)!;
    const policy = resolved(s, f)?.policy;
    const kids = s.drawing.nodes.filter((k) => k.frame === f).map((k) => k.id);
    const fixed = pinnedSize(s, f);
    const pad = n.style.padding;
    let w = Math.max(fb.w, ceilTo(Math.max(...kids.map((k) => box(k).x + box(k).w)) + pad - fb.x, g));
    let h = Math.max(fb.h, ceilTo(Math.max(...kids.map((k) => box(k).y + box(k).h)) + pad - fb.y, g));
    if (policy?.layout === 'stack') {
      const k = policy.direction === 'vertical' ? 'h' : 'w';
      let along = 0;
      for (const id of policy.order) {
        if (along) push(id, k === 'w' ? along : 0, k === 'h' ? along : 0);
        along += box(id)[k] - was(id)[k];
      }
      if (k === 'h') h = fb.h + along;
      else w = fb.w + along;
    }
    if (fixed.w) w = fb.w;
    if (fixed.h) h = fb.h;
    if (w !== fb.w || h !== fb.h) out.set(f, { ...fb, w, h });
  }
  for (const id of boxes.keys()) out.delete(id);
  return out;
}

// moveOps returns the moves of ids by (dx, dy), which snapDelta gave.
export function moveOps(s: Scene, ids: string[], dx: number, dy: number): Op[] {
  return ids.map((id) => {
    const r = s.nodes.get(id)!.rel ?? { x: 0, y: 0 };
    return { op: 'move', view: s.view.view, id, x: r.x + dx, y: r.y + dy };
  });
}

// pinnedSelf reports whether node id is pinned by its own layout entry.
function pinnedSelf(s: Scene, id: string): boolean {
  const n = s.nodes.get(id)!;
  return n.pinned && !n.orderedIn;
}

// withPins returns ops with the moves that pin nodes where they are drawn put first: in a view
// with nothing pinned, every node a stack or grid container does not place, so the rest of the
// first layout stays (spec 7.4); otherwise the nodes of need that ops require to be pinned. The
// nodes of skip are left out, the ops moving them.
export function withPins(s: Scene, ops: Op[], skip: Set<string>, need: string[] = []): Op[] {
  const nothingPinned = !s.drawing.nodes.some((n) => n.pinned);
  const pin = nothingPinned ? s.drawing.nodes.map((n) => n.id) : need.filter((id) => !pinnedSelf(s, id));
  const pins = [...new Set(pin)]
    .filter((id) => !skip.has(id) && !s.nodes.get(id)!.orderedIn)
    .sort(cmp)
    .flatMap((id) => moveOps(s, [id], 0, 0));
  return [...pins, ...ops];
}

// unpinOps returns the unpin operations of the nodes of ids that their layout entries pin.
export function unpinOps(s: Scene, ids: Iterable<string>): Op[] {
  return [...ids]
    .filter((id) => {
      const pin = resolved(s, id)?.pin;
      if (!pin) return false;
      return !s.nodes.get(id)?.orderedIn || pin.w !== undefined || pin.h !== undefined || !!pin.ports;
    })
    .sort(cmp)
    .map((id) => ({ op: 'unpin', view: s.view.view, id }));
}

export type Align = 'left' | 'center' | 'right' | 'top' | 'middle' | 'bottom';

// alignOps lines up the movable nodes of ids (at least two) on the extreme edge of the box
// around them, or its center, each kept inside its frame.
export function alignOps(s: Scene, ids: Iterable<string>, how: Align): Op[] {
  const list = movable(s, ids);
  if (list.length < 2) return [];
  const boxes = list.map((id) => s.nodes.get(id)!.box);
  const x0 = Math.min(...boxes.map((b) => b.x));
  const x1 = Math.max(...boxes.map((b) => b.x + b.w));
  const y0 = Math.min(...boxes.map((b) => b.y));
  const y1 = Math.max(...boxes.map((b) => b.y + b.h));
  const ops = list.flatMap((id, i): Op[] => {
    const b = boxes[i];
    const want: Record<Align, Point> = {
      left: { x: x0 - b.x, y: 0 },
      center: { x: (x0 + x1) / 2 - (b.x + b.w / 2), y: 0 },
      right: { x: x1 - b.x - b.w, y: 0 },
      top: { x: 0, y: y0 - b.y },
      middle: { x: 0, y: (y0 + y1) / 2 - (b.y + b.h / 2) },
      bottom: { x: 0, y: y1 - b.y - b.h },
    };
    const d = snapDelta(s, [id], want[how].x, want[how].y);
    return d.x || d.y || !pinnedSelf(s, id) ? moveOps(s, [id], d.x, d.y) : [];
  });
  return withPins(s, ops, new Set(list));
}

// distributeOps spaces the movable nodes of ids (at least three) evenly between the first and the
// last along an axis, in order of their centers, with equal gaps between their boxes.
export function distributeOps(s: Scene, ids: Iterable<string>, axis: 'x' | 'y'): Op[] {
  const list = movable(s, ids);
  if (list.length < 3) return [];
  const size = axis === 'x' ? 'w' : 'h';
  const sorted = list.map((id) => ({ id, b: s.nodes.get(id)!.box })).sort((a, b) => a.b[axis] + a.b[size] / 2 - (b.b[axis] + b.b[size] / 2) || cmp(a.id, b.id));
  const first = sorted[0].b;
  const last = sorted[sorted.length - 1].b;
  const room = last[axis] + last[size] - first[axis] - sorted.reduce((a, x) => a + x.b[size], 0);
  const gap = room / (sorted.length - 1);
  let at = first[axis];
  const ops = sorted.flatMap(({ id, b }): Op[] => {
    const want = at - b[axis];
    at += b[size] + gap;
    const d = snapDelta(s, [id], axis === 'x' ? want : 0, axis === 'y' ? want : 0);
    return d.x || d.y || !pinnedSelf(s, id) ? moveOps(s, [id], d.x, d.y) : [];
  });
  return withPins(s, ops, new Set(list));
}

export type Handle = 'nw' | 'ne' | 'sw' | 'se';

// resizedBox returns box b with its corner handle moved by (dx, dy), its size on the grid and at
// least a grid unit, its top-left corner not past origin, the content origin of its container,
// if any; uniform keeps its aspect ratio (spec 7.2).
export function resizedBox(b: Box, handle: Handle, dx: number, dy: number, grid: number, uniform: boolean, origin?: Point): Box {
  const west = handle === 'nw' || handle === 'sw';
  const north = handle === 'nw' || handle === 'ne';
  let w = Math.max(grid, roundTo(b.w + (west ? -dx : dx), grid));
  let h = Math.max(grid, roundTo(b.h + (north ? -dy : dy), grid));
  if (uniform) {
    const byW = Math.abs(w - b.w) / b.w >= Math.abs(h - b.h) / b.h;
    const [a, c] = byW ? keepRatio(w, b.w / b.h, grid) : keepRatio(h, b.h / b.w, grid);
    [w, h] = byW ? [a, c] : [c, a];
  }
  const x = west ? Math.max(origin?.x ?? -Infinity, b.x + b.w - w) : b.x;
  const y = north ? Math.max(origin?.y ?? -Infinity, b.y + b.h - h) : b.y;
  return { x, y, w: west ? b.x + b.w - x : w, h: north ? b.y + b.h - y : h };
}

// keepRatio returns a side near t and the other side, both grid multiples, whose ratio is
// nearest r: of t and its neighbors on the grid, the one that keeps the ratio best, and the
// nearest to t on a tie.
function keepRatio(t: number, r: number, grid: number): [number, number] {
  let best: [number, number] = [t, Math.max(grid, roundTo(t / r, grid))];
  let least = Infinity;
  for (const a of [t, t - grid, t + grid]) {
    if (a < grid) continue;
    const c = Math.max(grid, roundTo(a / r, grid));
    const err = Math.abs(a / c - r);
    if (err < least - 1e-9) [best, least] = [[a, c], err];
  }
  return best;
}

// resizeOps resizes node id to box b, moving it when its top or left side moved.
export function resizeOps(s: Scene, id: string, b: Box): Op[] {
  const n = s.nodes.get(id)!;
  const ops: Op[] = [];
  if (!n.orderedIn && (b.x !== n.box.x || b.y !== n.box.y)) ops.push(...moveOps(s, [id], b.x - n.box.x, b.y - n.box.y));
  ops.push({ op: 'resize', view: s.view.view, id, w: b.w, h: b.h });
  return withPins(s, ops, new Set(ops.length > 1 ? [id] : []), [id]);
}

// Landing is where a dragged child of a stack or grid container lands: its container, the new
// order, and a mark to draw there: a line across the stack between the siblings it goes
// between, or, in a grid, the box of the sibling whose place it takes.
export interface Landing {
  container: string;
  order: string[];
  mark: { line: [Point, Point] } | { box: Box };
}

// landing returns where child id of a stack or grid container lands when dropped at p: before
// the first sibling past p along a stack, or in the place of the sibling nearest p in a grid.
export function landing(s: Scene, id: string, p: Point): Landing | undefined {
  const c = s.nodes.get(id)?.orderedIn;
  if (c === undefined) return undefined;
  const policy = c === s.view.policy?.container ? s.view.policy : resolved(s, c)?.policy;
  if (!policy) return undefined;
  const others = policy.order.filter((o) => o !== id);
  const box = (o: string) => s.nodes.get(o)!.box;
  const mid = (o: string) => ({ x: box(o).x + box(o).w / 2, y: box(o).y + box(o).h / 2 });
  if (policy.layout === 'grid') {
    const d = policy.order.map((o) => Math.hypot(mid(o).x - p.x, mid(o).y - p.y));
    const at = d.indexOf(Math.min(...d));
    return { container: c, order: [...others.slice(0, at), id, ...others.slice(at)], mark: { box: box(policy.order[at]) } };
  }
  const k = policy.direction === 'vertical' ? 'y' : 'x';
  const size = k === 'y' ? 'h' : 'w';
  const across = k === 'y' ? 'x' : 'y';
  const at = others.filter((o) => mid(o)[k] < p[k]).length;
  const prev = others[at - 1];
  const next = others[at];
  const v = prev && next ? (box(prev)[k] + box(prev)[size] + box(next)[k]) / 2 : next ? box(next)[k] - policy.gap / 2 : box(prev)[k] + box(prev)[size] + policy.gap / 2;
  const all = policy.order.map(box);
  const lo = Math.min(...all.map((b) => b[across]));
  const hi = Math.max(...all.map((b) => b[across] + b[size === 'h' ? 'w' : 'h']));
  const line: [Point, Point] = k === 'y' ? [{ x: lo, y: v }, { x: hi, y: v }] : [{ x: v, y: lo }, { x: v, y: hi }];
  return { container: c, order: [...others.slice(0, at), id, ...others.slice(at)], mark: { line } };
}

// reorderOps moves child id of a stack or grid container to where it lands when dropped at p.
export function reorderOps(s: Scene, id: string, p: Point): Op[] {
  const l = landing(s, id, p);
  const policy = l && (l.container === s.view.policy?.container ? s.view.policy : resolved(s, l.container)?.policy);
  if (!l || !policy || l.order.every((o, i) => o === policy.order[i])) return [];
  return [{ op: 'reorder', view: s.view.view, container: l.container, order: l.order }];
}

// portPin returns the side of b nearest p and where along it, from its top or left end, p is,
// kept to two decimals as layout files keep it.
export function portPin(b: Box, p: Point): { side: Side; offset: number } {
  const side = nearestSide(b, p);
  const f = side === 'top' || side === 'bottom' ? (p.x - b.x) / b.w : (p.y - b.y) / b.h;
  return { side, offset: Math.round(Math.min(1, Math.max(0, f)) * 100) / 100 };
}

// pinPortOps pins port of node id where p is on its border.
export function pinPortOps(s: Scene, id: string, port: string, p: Point): Op[] {
  const { side, offset } = portPin(s.nodes.get(id)!.box, p);
  return withPins(s, [{ op: 'pin_port', view: s.view.view, id, port, side, offset }], new Set(), [id]);
}

// hintOp sets the fields of patch in edge e's hint, keeping the others; a field set to
// undefined is removed.
export function hintOp(s: Scene, e: ViewEdge, patch: Partial<Hint>): Op {
  const hint: Hint = { ...e.hint, ...patch };
  for (const k of Object.keys(hint) as (keyof Hint)[]) if (hint[k] === undefined || (k === 'via' && !hint.via!.length)) delete hint[k];
  return { op: 'set_edge_hint', view: s.view.view, id: e.id, hint };
}

// project returns where along route, as a length from its start, the point nearest p is, and
// that distance.
export function project(route: Point[], p: Point): { at: number; dist: number } {
  let best = { at: 0, dist: Infinity };
  let len = 0;
  for (let i = 0; i + 1 < route.length; i++) {
    const a = route[i];
    const b = route[i + 1];
    const l = Math.hypot(b.x - a.x, b.y - a.y);
    const t = l ? Math.min(1, Math.max(0, ((p.x - a.x) * (b.x - a.x) + (p.y - a.y) * (b.y - a.y)) / (l * l))) : 0;
    const dist = Math.hypot(a.x + t * (b.x - a.x) - p.x, a.y + t * (b.y - a.y) - p.y);
    if (dist < best.dist) best = { at: len + t * l, dist };
    len += l;
  }
  return best;
}

function length(route: Point[]): number {
  return route.slice(1).reduce((a, p, i) => a + Math.hypot(p.x - route[i].x, p.y - route[i].y), 0);
}

// labelPos returns the fraction along route nearest p, kept to two decimals.
export function labelPos(route: Point[], p: Point): number {
  const total = length(route);
  return total ? Math.round((project(route, p).at / total) * 100) / 100 : 0.5;
}

// withVia returns the via points with p added among them in the order of where they lie along
// route.
export function withVia(route: Point[], via: [number, number][], p: Point, grid: number): [number, number][] {
  const add: [number, number] = [roundTo(p.x, grid), roundTo(p.y, grid)];
  const at = (v: [number, number]) => project(route, { x: v[0], y: v[1] }).at;
  const i = via.findIndex((v) => at(v) > at(add));
  return i < 0 ? [...via, add] : [...via.slice(0, i), add, ...via.slice(i)];
}

// edgeAt returns the edge whose route passes within tol of p, the nearest.
export function edgeAt(d: Drawing, p: Point, tol: number): string | undefined {
  let best: { id: string; dist: number } | undefined;
  for (const e of d.edges) {
    const { dist } = project(e.points, p);
    if (dist <= tol && (!best || dist < best.dist)) best = { id: e.id, dist };
  }
  return best?.id;
}

// inside returns the nodes whose boxes lie inside r, in ID order.
export function inside(d: Drawing, r: Box): string[] {
  return d.nodes
    .filter((n) => n.box.x >= r.x && n.box.y >= r.y && n.box.x + n.box.w <= r.x + r.w && n.box.y + n.box.h <= r.y + r.h)
    .map((n) => n.id)
    .sort(cmp);
}

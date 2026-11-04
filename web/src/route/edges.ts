import { ceilTo, center, cmp, type Box, type Point } from '../geometry/geometry';
import { outline } from '../geometry/shapes';
import { lineHeight, textBlock, type TextBlock, type TextMeasure } from '../geometry/text';
import type { End, ResolvedView, Side, ViewEdge } from '../protocol/view';
import { MARKER_HALF, markerLength, type MarkerPart } from './markers';
import { clip, insidePolygon, keepsVia, onSegment, onSide, overlapScore, placeEndLabel, placeLabel, portPositions, rowSide, segmentHitsBox, sideFacing, simplify, type PortPos } from './route';

// BUFFER is the clearance the router keeps around every obstacle.
export const BUFFER = 10;
// STUB is how far outside the buffer a free end with a direction is routed from.
export const STUB = BUFFER + 2;
export const LABEL_PAD = 4;

// RouteEnd is where a route starts or ends: at the center of the obstacle it names, which the
// route may leave, or at a free point in any direction.
export interface RouteEnd {
  at: Point;
  obstacle?: string;
}

export interface RouteRequest {
  id: string;
  type: 'orthogonal' | 'polyline';
  from: RouteEnd;
  to: RouteEnd;
  via: Point[];
}

// Router routes requests around obstacles. Each call routes the whole set; a router that keeps
// its obstacles between calls may route only what changed.
export type Router = (obstacles: Map<string, Box>, edges: RouteRequest[]) => Promise<Map<string, Point[]>>;

type Dir = Side | 'all';

// Plan is how an edge end is routed: where it is and the directions it may leave in, the
// obstacle whose center it starts from (dir 'all'), the outline a route from that center is
// clipped to, whether it may be spread along the side it enters by, the node whose outline an
// end on its bounding box is moved onto, and how far it must run straight from its side.
interface Plan {
  at: Point;
  dir: Dir;
  obstacle?: string;
  clipTo?: Point[];
  spread?: boolean;
  outlineOf?: string;
  stub?: number;
}

export const step: Record<Side, Point> = { top: { x: 0, y: -1 }, bottom: { x: 0, y: 1 }, left: { x: -1, y: 0 }, right: { x: 1, y: 0 } };
const opposite: Record<Side, Side> = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' };

export interface EdgeGeometry {
  view: ResolvedView;
  boxes: Map<string, Box>;
  // headers holds the boxes of the containers' labels.
  headers: Map<string, Box>;
  // rows holds the y of each row line of each table, in canvas coordinates.
  rows: Map<string, Map<string, number>>;
  grid: number;
}

// planPorts places every node's ports (spec 7.5); an auto port faces the other end of the first
// edge, by ID, that uses it.
export function planPorts(g: EdgeGeometry): Map<string, Map<string, PortPos>> {
  const shapes = g.view.shapes ?? {};
  const toward = new Map<string, Map<string, Point>>();
  for (const e of g.view.edges) {
    for (const [end, other] of [[e.from, e.to], [e.to, e.from]]) {
      if (!end.port) continue;
      const m = toward.get(end.node) ?? new Map<string, Point>();
      if (!m.has(end.port)) m.set(end.port, center(g.boxes.get(other.node)!));
      toward.set(end.node, m);
    }
  }
  return new Map(g.view.nodes.map((n) => [n.id, portPositions(n, g.boxes.get(n.id)!, shapes[n.style.shape], toward.get(n.id) ?? new Map())]));
}

// routeEdges routes every edge of the view (spec 7.5). Leaf nodes and the labels of containers
// are obstacles. An end at a leaf without a port or hint is first routed from the leaf's center
// to find the side it enters by; the ends entering one side are then spread along it, ordered by
// their other ends, and routed again. An end with a direction (a port, a row, a side hint, a spread
// end, an end at a container) is routed from a free point a stub outside it, then joined to it.
// A self-loop between two plain ends of a node is drawn around one of its corners.
export async function routeEdges(g: EdgeGeometry, ports: Map<string, Map<string, PortPos>>, router: Router): Promise<Map<string, Point[]>> {
  const { view, boxes, grid } = g;
  const shapes = view.shapes ?? {};
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const outlineOf = (id: string) => outline(shapes[byId.get(id)!.style.shape] ? 'rect' : byId.get(id)!.style.shape, boxes.get(id)!);
  const port = (end: End) => (end.port ? ports.get(end.node)!.get(end.port) : undefined);
  const endBox = (end: End): Box => {
    const p = port(end);
    return p ? { x: p.x, y: p.y, w: 0, h: 0 } : boxes.get(end.node)!;
  };
  // A row of a node not drawn as a table has no line, so an edge ends at the node itself.
  const rowEnd = (end: End, other: End): PortPos | undefined => {
    const y = end.row !== undefined ? g.rows.get(end.node)?.get(end.row) : undefined;
    if (y === undefined) return undefined;
    const b = boxes.get(end.node)!;
    const side = rowSide(b, endBox(other));
    return { x: side === 'left' ? b.x : b.x + b.w, y, side };
  };
  const at = (end: End, other: End): Point => port(end) ?? rowEnd(end, other) ?? center(boxes.get(end.node)!);

  const plan = (end: End, other: End, hint?: Side): Plan => {
    const n = byId.get(end.node)!;
    const b = boxes.get(n.id)!;
    const fixed = port(end) ?? rowEnd(end, other);
    if (fixed) return { at: fixed, dir: fixed.side };
    if (!n.container) {
      if (hint) return { at: onSide(b, hint, 0.5), dir: hint, outlineOf: n.id };
      return { at: center(b), dir: 'all', obstacle: n.id, clipTo: outlineOf(n.id), spread: true };
    }
    const p = at(other, end);
    const inside = p.x > b.x && p.x < b.x + b.w && p.y > b.y && p.y < b.y + b.h;
    const side = hint ?? (inside ? nearestSide(b, p) : sideFacing(b, p));
    return { at: project(b, side, p), dir: inside ? opposite[side] : side };
  };
  const loops = new Set(view.edges.filter(isCornerLoop).map((e) => e.id));
  const routed = view.edges.filter((e) => !loops.has(e.id));
  const plans = new Map(routed.map((e) => [e.id, [plan(e.from, e.to, e.hint?.from_side), plan(e.to, e.from, e.hint?.to_side)] as [Plan, Plan]]));
  const markerAt = (e: ViewEdge, i: number) => (i === 0 ? e.style.arrow_start : e.style.arrow_end);

  // Ends at one row end with the same marker share its point; ends with different markers are
  // spread along the side, ordered by their other ends. A row end's stub reaches a grid unit past
  // the longest marker at its side of the table, so no line turns among the markers, unless
  // clearStub or keepStub keep it shorter.
  const atRow = new Map<string, { plan: Plan; marker: string; key: number }[]>();
  const longest = new Map<string, number>();
  for (const e of routed) {
    [e.from, e.to].forEach((end, i) => {
      const other = i === 0 ? e.to : e.from;
      if (!rowEnd(end, other)) return;
      const p = plans.get(e.id)![i];
      const k = [end.node, p.dir, end.row].join('\n');
      atRow.set(k, [...(atRow.get(k) ?? []), { plan: p, marker: markerAt(e, i), key: at(other, end).y }]);
      const side = end.node + '\n' + p.dir;
      longest.set(side, Math.max(longest.get(side) ?? 0, markerLength(markerAt(e, i))));
    });
  }
  for (const [k, ends] of atRow) {
    ends.sort((a, b) => a.key - b.key || cmp(a.marker, b.marker));
    const markers = [...new Set(ends.map((x) => x.marker))];
    const reach = longest.get(k.split('\n').slice(0, 2).join('\n'))! + grid;
    for (const x of ends) {
      x.plan.at = { ...x.plan.at, y: x.plan.at.y + (markers.indexOf(x.marker) - (markers.length - 1) / 2) * ROW_SPREAD };
      x.plan.stub = Math.max(stubLength(x.marker), reach);
    }
  }

  // A label's key starts with a control character, which no node ID has. Its box is shortened by
  // the buffer on the side facing the container's content, so the buffer there covers the text
  // and leaves room for an edge between the label and the children, and a free end by a child is
  // not inside it, where libavoid would route straight through.
  const obstacles = new Map<string, Box>([
    ...view.nodes.filter((n) => !n.container).map((n): [string, Box] => [n.id, boxes.get(n.id)!]),
    ...[...g.headers].map(([id, b]): [string, Box] => ['\0' + id, byId.get(id)!.style.label_side === 'left' ? { ...b, w: Math.max(1, b.w - BUFFER) } : { ...b, h: Math.max(1, b.h - BUFFER) }]),
  ]);
  const blocks = [...obstacles.values()];
  const viaOf = (e: ViewEdge): Point[] => (e.hint?.via ?? []).map(([x, y]) => ({ x, y }));

  const routeAll = async () => {
    const requests: RouteRequest[] = routed
      .filter((e) => e.style.routing !== 'straight')
      .map((e) => {
        const [from, to] = plans.get(e.id)!;
        const type = e.style.routing === 'polyline' ? 'polyline' : 'orthogonal';
        const via = viaOf(e);
        // A via on a stub ends it: past the via, the legs on either side of it would overlap.
        const end = (p: Plan, marker: string, v: Point | undefined) => {
          const r = request(p, marker, blocks);
          return v && p.dir !== 'all' && onSegment(v, p.at, r.at) ? { at: v } : r;
        };
        return { id: e.id, type, from: end(from, markerAt(e, 0), via[0]), to: end(to, markerAt(e, 1), via[via.length - 1]), via };
      });
    const routes = requests.length ? await router(obstacles, requests) : new Map<string, Point[]>();
    return new Map(
      routed.map((e) => {
        const [from, to] = plans.get(e.id)!;
        let raw = e.style.routing === 'straight' ? [from.at, to.at] : [...(routes.get(e.id) ?? [])];
        if (e.style.routing !== 'straight') {
          if (from.dir !== 'all') raw = [...join(from, raw[0]), ...raw];
          if (to.dir !== 'all') raw = [...raw, ...join(to, raw[raw.length - 1]).reverse()];
        }
        return [e.id, clip(raw, from.clipTo ?? null, to.clipTo ?? null, viaOf(e))];
      }),
    );
  };

  const first = await routeAll();

  // Each self-loop takes the free corner of its node where it and its end labels overlap the
  // least, clockwise from the top right on ties; once all four are taken, loops nest further out.
  const corners = new Map<string, { c: number; level: number; reach: number }>();
  const used = new Map<string, number[]>();
  const placed = [...first.values()];
  for (const e of view.edges.filter((e) => loops.has(e.id))) {
    const id = e.from.node;
    const b = boxes.get(id)!;
    const taken = used.get(id) ?? [];
    const level = Math.floor(taken.length / CORNERS.length);
    const reach = loopReach(e.style.arrow_start, e.style.arrow_end, grid);
    const others = [...obstacles].filter(([k]) => k !== id).map(([, o]) => o);
    let best = { c: 0, level, reach };
    let bestRoute: Point[] = [];
    let least = Infinity;
    CORNERS.forEach((_, c) => {
      if (taken.slice(level * CORNERS.length).includes(c)) return;
      const [loop] = cornerLoop(b, c, level, reach);
      const xs = loop.map((p) => p.x);
      const ys = loop.map((p) => p.y);
      const room = { x: Math.min(...xs) - LOOP_ROOM, y: Math.min(...ys) - LOOP_ROOM, w: Math.max(...xs) - Math.min(...xs) + 2 * LOOP_ROOM, h: Math.max(...ys) - Math.min(...ys) + 2 * LOOP_ROOM };
      const score = overlapScore(room, others, placed);
      if (score < least) [best, bestRoute, least] = [{ c, level, reach }, loop, score];
    });
    used.set(id, [...taken, best.c]);
    corners.set(e.id, best);
    placed.push(bestRoute);
  }

  // A loop's ends take slots among the spread ends on the sides they are on, the slot nearest
  // their corner, so no edge attaches where a loop does.
  const sides = new Map<string, { edge: string; end: 0 | 1; key: number }[]>();
  for (const e of routed) {
    if (e.style.routing === 'straight') continue;
    const pts = first.get(e.id)!;
    [e.from, e.to].forEach((end, i) => {
      if (!plans.get(e.id)![i].spread || pts.length < 2) return;
      const side = nearestSide(boxes.get(end.node)!, i === 0 ? pts[0] : pts[pts.length - 1]);
      const o = i === 0 ? at(e.to, e.from) : at(e.from, e.to);
      const k = end.node + '\n' + side;
      sides.set(k, [...(sides.get(k) ?? []), { edge: e.id, end: i as 0 | 1, key: side === 'top' || side === 'bottom' ? o.x : o.y }]);
    });
  }
  for (const e of view.edges.filter((e) => loops.has(e.id))) {
    const [sx, sy] = CORNERS[corners.get(e.id)!.c];
    const loopSides: [Side, number][] = [[sx > 0 ? 'right' : 'left', sy * FAR], [sy > 0 ? 'bottom' : 'top', sx * FAR]];
    loopSides.forEach(([side, key], i) => sides.get(e.from.node + '\n' + side)?.push({ edge: e.id, end: i as 0 | 1, key }));
  }
  const loopEnds = new Map<string, Point>();
  for (const [k, ends] of [...sides].sort((a, b) => cmp(a[0], b[0]))) {
    const [id, side] = k.split('\n') as [string, Side];
    ends.sort((a, b) => a.key - b.key || cmp(a.edge, b.edge) || a.end - b.end);
    ends.forEach((x, i) => {
      const at = onSide(boxes.get(id)!, side, (i + 1) / (ends.length + 1));
      if (loops.has(x.edge)) loopEnds.set(x.edge + '\n' + x.end, at);
      else plans.get(x.edge)![x.end] = { at, dir: side, outlineOf: id, spread: true };
    });
  }
  for (const [from, to] of plans.values()) straighten(from, to, boxes, grid);

  const second = sides.size ? await routeAll() : first;
  const containers = view.nodes.filter((n) => n.container).map((n) => boxes.get(n.id)!);
  const solid = [...view.nodes.filter((n) => !n.container).map((n) => boxes.get(n.id)!), ...g.headers.values()];
  const out = new Map<string, Point[]>();
  for (const e of routed) {
    // libavoid draws a straight line where it finds no route; such an edge keeps its first route.
    let pts = e.style.routing === 'orthogonal' && !orthogonal(second.get(e.id)!) ? first.get(e.id)! : second.get(e.id)!;
    pts = [...pts];
    const [from, to] = plans.get(e.id)!;
    if (from.outlineOf && pts.length > 1) pts[0] = ontoOutline(pts[0], from.dir as Side, boxes.get(from.outlineOf)!, outlineOf(from.outlineOf));
    if (to.outlineOf && pts.length > 1) pts[pts.length - 1] = ontoOutline(pts[pts.length - 1], to.dir as Side, boxes.get(to.outlineOf)!, outlineOf(to.outlineOf));
    const via = viaOf(e);
    pts = simplify(pts.map((p) => ({ x: Math.round(p.x), y: Math.round(p.y) })), via);
    if (e.style.routing === 'orthogonal') {
      pts = awayFromBorders(pts, containers, solid, grid, via);
      [from, to].forEach((p, i) => {
        const marker = markerAt(e, i);
        const label = i === 0 ? e.labels.start : e.labels.end;
        if (p.dir === 'all' || (marker === 'none' && p.stub === undefined && !label)) return;
        const len = Math.max(p.stub ?? stubLength(marker), label ? 2 * LABEL_PAD + lineHeight(e.style.font_size) : 0);
        const done = routed.filter((o) => o.id !== e.id).map((o) => out.get(o.id) ?? second.get(o.id)!);
        pts = i === 0 ? keepStub(pts, p.dir, len, solid, done, grid, via) : keepStub([...pts].reverse(), p.dir, len, solid, done, grid, [...via].reverse()).reverse();
      });
    }
    out.set(e.id, pts);
  }

  for (const e of view.edges.filter((e) => loops.has(e.id))) {
    const id = e.from.node;
    const b = boxes.get(id)!;
    const { c, level, reach } = corners.get(e.id)!;
    const s0 = loopEnds.get(e.id + '\n0');
    const s1 = loopEnds.get(e.id + '\n1');
    const [sx, sy] = CORNERS[c];
    const cx = sx > 0 ? b.x + b.w : b.x;
    const cy = sy > 0 ? b.y + b.h : b.y;
    const [pts, fromSide, toSide] = cornerLoop(b, c, level, reach, s0 && Math.abs(s0.y - cy), s1 && Math.abs(s1.x - cx));
    pts[0] = ontoOutline(pts[0], fromSide, b, outlineOf(id));
    pts[pts.length - 1] = ontoOutline(pts[pts.length - 1], toSide, b, outlineOf(id));
    out.set(e.id, simplify(pts.map((p) => ({ x: Math.round(p.x), y: Math.round(p.y) }))));
  }
  return new Map(view.edges.map((e) => [e.id, out.get(e.id)!]));
}

// keepStub moves the segment after a route's first one so that the first, which leaves an end with
// direction dir, runs at least len, and a further gap at a time while the moved segment would lie
// on a segment of another route in others, unless that would cross a box of solid or take the
// route off a point of via: libavoid's nudging can pull a stub's free point back toward the box,
// or past its end. A box holding the end is the end's own, which only the stub may leave.
export function keepStub(route: Point[], dir: Side, len: number, solid: Box[], others: Point[][] = [], gap = 10, via: Point[] = []): Point[] {
  if (route.length < 4) return route;
  const k = dir === 'left' || dir === 'right' ? 'x' : 'y';
  const o = k === 'x' ? 'y' : 'x';
  const s = step[dir][k];
  const [a, b, c, d] = route;
  if (a[o] !== b[o] || b[k] !== c[k] || c[o] !== d[o] || (b[k] - a[k]) * s >= len) return route;
  const [lo, hi] = [Math.min(b[o], c[o]), Math.max(b[o], c[o])];
  const onOther = (v: number) =>
    others.some((r) => r.slice(1).some((q, i) => Math.abs(q[k] - v) < gap / 2 && Math.abs(r[i][k] - v) < gap / 2 && Math.min(q[o], r[i][o]) < hi && Math.max(q[o], r[i][o]) > lo));
  let v = a[k] + s * len;
  for (let n = 0; n < 10 && onOther(v); n++) v += s * gap;
  const moved = route.map((p, i) => (i === 1 || i === 2 ? { ...p, [k]: v } : p));
  if (solid.some((l) => (!holds(l, a) && segmentHitsBox(a, moved[1], l)) || segmentHitsBox(moved[1], moved[2], l) || segmentHitsBox(moved[2], moved[3], l))) return route;
  return keepsVia(route, moved, via) ? simplify(moved, via) : route;
}

// joinLegs joins the legs of a route at the via points between them, so the route passes through
// each exactly. libavoid moves a leg's end when it nudges the leg's first or last segment. The
// first segment of each leg goes back onto its via; so does the last segment of the leg before,
// unless the route doubles back there, where nudging keeps the two apart, or that would take the
// route off the via before: that one stays and is joined to the via by a short segment.
export function joinLegs(legs: Point[][], via: Point[]): Point[] {
  const out = [...legs[0]];
  let placed = -1;
  via.forEach((v, i) => {
    const [start, second, ...rest] = legs[i + 1];
    const end = out[out.length - 1];
    const before = out[out.length - 2];
    const moved = end.x !== v.x || end.y !== v.y;
    const doublesBack = moved && (end.x - before.x) * (second.x - start.x) + (end.y - before.y) * (second.y - start.y) < 0;
    const slid = slide(before, end, v);
    if (doublesBack || (placed === out.length - 2 && !onSegment(before, out[placed - 1], slid))) out[out.length - 1] = extend(end, before, v);
    else {
      out[out.length - 2] = slid;
      out.pop();
    }
    placed = out.length;
    out.push(v, slide(second, start, v), ...rest);
  });
  return out;
}

// slide returns p, the far point of a segment from end, once the segment has moved across its
// direction to pass through v. A segment that is neither horizontal nor vertical is not nudged.
function slide(p: Point, end: Point, v: Point): Point {
  if (p.x === end.x) return { ...p, x: v.x };
  return p.y === end.y ? { ...p, y: v.y } : p;
}

// extend returns p, the far point of a segment from end, once the segment runs on to the line of v
// across it, so that p and v are in line.
function extend(p: Point, end: Point, v: Point): Point {
  if (p.x === end.x) return { ...p, y: v.y };
  return p.y === end.y ? { ...p, x: v.x } : p;
}

// join returns the points from an end with a direction to the first point of its route: the end,
// and a corner when libavoid's nudging has moved the route's stub point off the end's normal.
export function join(p: Pick<Plan, 'at' | 'dir'>, first: Point | undefined): Point[] {
  if (!first || p.dir === 'all') return [p.at];
  const vertical = p.dir === 'top' || p.dir === 'bottom';
  if (vertical ? first.x === p.at.x : first.y === p.at.y) return [p.at];
  return [p.at, vertical ? { x: p.at.x, y: first.y } : { x: first.x, y: p.at.y }];
}

// stubLength returns how far an end with marker runs straight from its side: far enough for the
// marker and a little line beyond it. An open arrow fits the shortest stub.
function stubLength(marker: string): number {
  const len = markerLength(marker);
  return len + 2 <= STUB ? STUB : len + 8;
}

// request returns the end of a route request; an end with a direction is routed from a free point
// its stub outside it.
function request(p: Plan, marker: string, obstacles: Box[]): RouteEnd {
  if (p.dir === 'all') return { at: p.at, obstacle: p.obstacle };
  const stub = clearStub(p.at, p.dir, p.stub ?? stubLength(marker), obstacles);
  return { at: { x: p.at.x + stub * step[p.dir].x, y: p.at.y + stub * step[p.dir].y } };
}

// clearStub returns stub, cut to end at the buffer of the first obstacle in its way, but no shorter
// than STUB: libavoid routes straight through an obstacle from a free point in its buffer, and a
// long stub reaches past the gap between a child and its container's label. A buffer the end is
// already in, like that of its own node, does not count.
export function clearStub(at: Point, dir: Side, stub: number, obstacles: Box[]): number {
  const k = dir === 'left' || dir === 'right' ? 'x' : 'y';
  const tip = { ...at, [k]: at[k] + step[dir][k] * stub };
  let clear = stub;
  for (const o of obstacles) {
    const b = { x: o.x - BUFFER, y: o.y - BUFFER, w: o.w + 2 * BUFFER, h: o.h + 2 * BUFFER };
    if (holds(b, at) || !segmentHitsBox(at, tip, b)) continue;
    const near = step[dir][k] > 0 ? b[k] : b[k] + (k === 'x' ? b.w : b.h);
    clear = Math.min(clear, Math.abs(near - at[k]));
  }
  return Math.max(STUB, clear);
}

function holds(b: Box, p: Point): boolean {
  return p.x >= b.x && p.x <= b.x + b.w && p.y >= b.y && p.y <= b.y + b.h;
}

// isCornerLoop reports whether an edge is a self-loop between two plain ends of a node, which is
// drawn around one of the node's corners instead of routed.
function isCornerLoop(e: ViewEdge): boolean {
  return e.from.node === e.to.node && !e.from.port && !e.to.port && !e.from.row && !e.to.row && !e.hint?.from_side && !e.hint?.to_side && !e.hint?.via?.length;
}

const CORNERS: [number, number][] = [[1, -1], [1, 1], [-1, 1], [-1, -1]];

// loopReach returns how far a self-loop reaches out from its node: two grid units, or more when
// its markers need it.
function loopReach(start: string, end: string, grid: number): number {
  return ceilTo(Math.max(2 * grid, markerLength(start) + 6, markerLength(end) + 6), grid);
}

// LOOP_ROOM is the room around a self-loop its end labels need.
const LOOP_ROOM = 30;
// FAR sorts a loop's end to the corner end of a side's spread ends.
const FAR = 1e9;
// ROW_SPREAD separates ends with different markers at one row end.
const ROW_SPREAD = 2 * MARKER_HALF + 2;

// cornerLoop returns a self-loop of a node with box b around its corner c (clockwise from the top
// right) at nesting level: it leaves a vertical side a from the corner and enters a horizontal one
// a2 from it, reaching further out at each level. It also returns the sides its ends are on.
export function cornerLoop(b: Box, c: number, level: number, reach: number, a = Math.min(b.h / 2, reach), a2 = Math.min(b.w / 2, reach)): [Point[], Side, Side] {
  const [sx, sy] = CORNERS[c];
  const d = reach * (1 + level);
  const cx = sx > 0 ? b.x + b.w : b.x;
  const cy = sy > 0 ? b.y + b.h : b.y;
  const pts = [
    { x: cx, y: cy - sy * a },
    { x: cx + sx * d, y: cy - sy * a },
    { x: cx + sx * d, y: cy + sy * d },
    { x: cx - sx * a2, y: cy + sy * d },
    { x: cx - sx * a2, y: cy },
  ];
  return [pts, sx > 0 ? 'right' : 'left', sy > 0 ? 'bottom' : 'top'];
}

// straighten moves two spread ends that face each other a little apart to their midpoint.
function straighten(a: Plan, b: Plan, boxes: Map<string, Box>, grid: number) {
  const vertical = (a.dir === 'bottom' && b.dir === 'top') || (a.dir === 'top' && b.dir === 'bottom');
  const horizontal = (a.dir === 'right' && b.dir === 'left') || (a.dir === 'left' && b.dir === 'right');
  if (!a.spread || !b.spread || !a.outlineOf || !b.outlineOf || !(vertical || horizontal)) return;
  const k = vertical ? 'x' : 'y';
  if (a.at[k] === b.at[k] || Math.abs(a.at[k] - b.at[k]) > 2 * grid) return;
  const mid = (a.at[k] + b.at[k]) / 2;
  const fits = (id: string) => {
    const box = boxes.get(id)!;
    return k === 'x' ? mid > box.x && mid < box.x + box.w : mid > box.y && mid < box.y + box.h;
  };
  if (fits(a.outlineOf) && fits(b.outlineOf)) {
    a.at = { ...a.at, [k]: mid };
    b.at = { ...b.at, [k]: mid };
  }
}

// awayFromBorders moves each inner segment of an orthogonal route that runs along a container's
// border, closer to it than gap and between two turns, to gap from it, unless that would cross a
// box of solid or take the route off a point of via. The first and last segments stay, so the ends
// do not move.
export function awayFromBorders(route: Point[], containers: Box[], solid: Box[], gap: number, via: Point[] = []): Point[] {
  const pts = route.map((p) => ({ ...p }));
  for (let i = 1; i + 2 < pts.length; i++) {
    const a = pts[i];
    const b = pts[i + 1];
    const k = a.x === b.x ? 'x' : a.y === b.y ? 'y' : undefined;
    if (!k) continue;
    const o = k === 'x' ? 'y' : 'x';
    if (pts[i - 1][o] !== a[o] || pts[i + 2][o] !== b[o]) continue;
    for (const c of containers) {
      const lo = k === 'x' ? c.y : c.x;
      const hi = k === 'x' ? c.y + c.h : c.x + c.w;
      if (Math.max(a[o], b[o]) <= lo || Math.min(a[o], b[o]) >= hi) continue;
      const borders: [number, number][] = k === 'x' ? [[c.x, -1], [c.x + c.w, 1]] : [[c.y, -1], [c.y + c.h, 1]];
      const near = borders.find(([v]) => Math.abs(a[k] - v) < gap);
      if (!near) continue;
      const [v, outward] = near;
      const moved = v + Math.sign(a[k] - v || outward) * gap;
      const trial = pts.map((p, j) => (j === i || j === i + 1 ? { ...p, [k]: moved } : p));
      const touched = [trial[i - 1], trial[i], trial[i + 1], trial[i + 2]];
      if (solid.some((l) => touched.slice(1).some((p, j) => segmentHitsBox(touched[j], p, l)))) continue;
      if (!keepsVia(pts, trial, via)) continue;
      pts[i] = trial[i];
      pts[i + 1] = trial[i + 1];
      break;
    }
  }
  return simplify(pts, via);
}

export interface PlacedLabel {
  text: TextBlock;
  box: Box;
}

// placeLabels places the labels of every edge (spec 7.5): first the start and end labels next to
// their ends, then the middle labels, in edge order, each avoiding leaf nodes, container labels,
// markers and the labels placed before it. markers holds the parts of each marker of each edge.
// Each edge's labels are returned middle, start, end.
export function placeLabels(g: EdgeGeometry, routes: Map<string, Point[]>, markers: Map<string, MarkerPart[][]>, text: TextMeasure): Map<string, PlacedLabel[]> {
  const markerBoxes = [...markers.values()].flat().filter((m) => m.length).map(markerBox);
  const avoid = [...g.view.nodes.filter((n) => !n.container).map((n) => g.boxes.get(n.id)!), ...g.headers.values(), ...markerBoxes];
  const all = g.view.edges.map((e) => routes.get(e.id)!);
  const label = (s: string, size: number) => {
    const t = textBlock(s, 0, { size, weight: 400 }, text);
    return { t, size: { w: ceilTo(t.w + 2 * LABEL_PAD, g.grid), h: t.h } };
  };
  const ends = new Map<string, PlacedLabel[]>();
  for (const e of g.view.edges) {
    const route = routes.get(e.id)!;
    const placed: PlacedLabel[] = [];
    for (const [s, r] of [[e.labels.start, [...route].reverse()], [e.labels.end, route]] as const) {
      if (!s || r.length < 2) continue;
      const { t, size } = label(s, e.style.font_size);
      const box = placeEndLabel(r, size, avoid, all.filter((o) => o !== route));
      avoid.push(box);
      placed.push({ text: t, box });
    }
    ends.set(e.id, placed);
  }
  const out = new Map<string, PlacedLabel[]>();
  for (const e of g.view.edges) {
    const placed: PlacedLabel[] = [];
    if (e.labels.mid) {
      const { t, size } = label(e.labels.mid, e.style.font_size);
      const others = g.view.edges.filter((o) => o.id !== e.id).map((o) => routes.get(o.id)!);
      const box = placeLabel(routes.get(e.id)!, size, e.hint?.label_pos, avoid, others);
      avoid.push(box);
      placed.push({ text: t, box });
    }
    out.set(e.id, [...placed, ...ends.get(e.id)!]);
  }
  return out;
}

function markerBox(parts: MarkerPart[]): Box {
  const pts = parts.flatMap((p) => ('line' in p ? p.line : 'polygon' in p ? p.polygon : [{ x: p.circle.x - p.r, y: p.circle.y - p.r }, { x: p.circle.x + p.r, y: p.circle.y + p.r }]));
  const x = Math.min(...pts.map((p) => p.x));
  const y = Math.min(...pts.map((p) => p.y));
  return { x, y, w: Math.max(...pts.map((p) => p.x)) - x, h: Math.max(...pts.map((p) => p.y)) - y };
}

// nearestSide returns the side of b nearest p, or, for a point outside b, the side it is beyond
// the most.
export function nearestSide(b: Box, p: Point): Side {
  const d: [Side, number][] = [['top', p.y - b.y], ['right', b.x + b.w - p.x], ['bottom', b.y + b.h - p.y], ['left', p.x - b.x]];
  return d.reduce((a, c) => (c[1] < a[1] ? c : a))[0];
}

// project returns the point of side of b nearest p, kept away from the corners.
function project(b: Box, side: Side, p: Point): Point {
  const mx = Math.min(16, b.w / 2);
  const my = Math.min(16, b.h / 2);
  const x = Math.min(Math.max(p.x, b.x + mx), b.x + b.w - mx);
  const y = Math.min(Math.max(p.y, b.y + my), b.y + b.h - my);
  switch (side) {
    case 'top':
      return { x, y: b.y };
    case 'bottom':
      return { x, y: b.y + b.h };
    case 'left':
      return { x: b.x, y };
    case 'right':
      return { x: b.x + b.w, y };
  }
}

// ontoOutline moves an end p on side of box b inward, along the side's normal, onto outline.
function ontoOutline(p: Point, side: Side, b: Box, poly: Point[]): Point {
  const c = center(b);
  const inner = side === 'top' || side === 'bottom' ? { x: p.x, y: c.y } : { x: c.x, y: p.y };
  if (!insidePolygon(inner, poly) || insidePolygon(p, poly)) return p;
  return clip([inner, p], poly, null)[0];
}

function orthogonal(pts: Point[]): boolean {
  return pts.slice(1).every((p, i) => p.x === pts[i].x || p.y === pts[i].y);
}

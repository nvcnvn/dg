import { ceilTo, center, cmp, type Box, type Point } from '../geometry/geometry';
import { outline } from '../geometry/shapes';
import { textBlock, type TextBlock, type TextMeasure } from '../geometry/text';
import type { End, ResolvedView, Side } from '../protocol/view';
import { clip, insidePolygon, onSide, placeLabel, portPositions, segmentHitsBox, sideFacing, simplify, type PortPos } from './route';

// BUFFER is the clearance the router keeps around every obstacle.
export const BUFFER = 10;
// STUB is how far outside the buffer a free end with a direction is routed from.
const STUB = BUFFER + 2;
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
// clipped to, whether it may be spread along the side it enters by, and the node whose outline
// an end on its bounding box is moved onto.
interface Plan {
  at: Point;
  dir: Dir;
  obstacle?: string;
  clipTo?: Point[];
  spread?: boolean;
  outlineOf?: string;
}

const step: Record<Side, Point> = { top: { x: 0, y: -1 }, bottom: { x: 0, y: 1 }, left: { x: -1, y: 0 }, right: { x: 1, y: 0 } };
const opposite: Record<Side, Side> = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' };

export interface EdgeGeometry {
  view: ResolvedView;
  boxes: Map<string, Box>;
  // headers holds the boxes of the containers' labels.
  headers: Map<string, Box>;
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
// their other ends, and routed again. An end with a direction (a port, a side hint, a spread end,
// an end at a container) is routed from a free point a stub outside it, then joined to it.
export async function routeEdges(g: EdgeGeometry, ports: Map<string, Map<string, PortPos>>, router: Router): Promise<Map<string, Point[]>> {
  const { view, boxes, grid } = g;
  const shapes = view.shapes ?? {};
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const outlineOf = (id: string) => outline(shapes[byId.get(id)!.style.shape] ? 'rect' : byId.get(id)!.style.shape, boxes.get(id)!);
  const at = (end: End): Point => (end.port && ports.get(end.node)!.get(end.port)) || center(boxes.get(end.node)!);

  const plan = (end: End, other: End, hint?: Side): Plan => {
    const n = byId.get(end.node)!;
    const b = boxes.get(n.id)!;
    const port = end.port ? ports.get(n.id)!.get(end.port) : undefined;
    if (port) return { at: port, dir: port.side };
    if (!n.container) {
      if (hint) return { at: onSide(b, hint, 0.5), dir: hint, outlineOf: n.id };
      return { at: center(b), dir: 'all', obstacle: n.id, clipTo: outlineOf(n.id), spread: true };
    }
    const p = at(other);
    const inside = p.x > b.x && p.x < b.x + b.w && p.y > b.y && p.y < b.y + b.h;
    const side = hint ?? (inside ? nearestSide(b, p) : sideFacing(b, p));
    return { at: project(b, side, p), dir: inside ? opposite[side] : side };
  };
  const plans = new Map(view.edges.map((e) => [e.id, [plan(e.from, e.to, e.hint?.from_side), plan(e.to, e.from, e.hint?.to_side)] as [Plan, Plan]]));

  // A label's key starts with a control character, which no node ID has. Its box is shortened by
  // the buffer at the bottom, so the buffer there covers the text and leaves room for an edge
  // between the label and the container's first row of children.
  const obstacles = new Map<string, Box>([
    ...view.nodes.filter((n) => !n.container).map((n): [string, Box] => [n.id, boxes.get(n.id)!]),
    ...[...g.headers].map(([id, b]): [string, Box] => ['\0' + id, { ...b, h: Math.max(1, b.h - BUFFER) }]),
  ]);

  const routeAll = async () => {
    const requests: RouteRequest[] = view.edges
      .filter((e) => e.style.routing !== 'straight')
      .map((e) => {
        const [from, to] = plans.get(e.id)!;
        const type = e.style.routing === 'polyline' ? 'polyline' : 'orthogonal';
        return { id: e.id, type, from: request(from), to: request(to), via: (e.hint?.via ?? []).map(([x, y]) => ({ x, y })) };
      });
    const routes = requests.length ? await router(obstacles, requests) : new Map<string, Point[]>();
    return new Map(
      view.edges.map((e) => {
        const [from, to] = plans.get(e.id)!;
        let raw = e.style.routing === 'straight' ? [from.at, to.at] : [...(routes.get(e.id) ?? [])];
        if (e.style.routing !== 'straight') {
          if (from.dir !== 'all') raw = [...join(from, raw[0]), ...raw];
          if (to.dir !== 'all') raw = [...raw, ...join(to, raw[raw.length - 1]).reverse()];
        }
        return [e.id, clip(raw, from.clipTo ?? null, to.clipTo ?? null)];
      }),
    );
  };

  const first = await routeAll();
  const sides = new Map<string, { edge: string; end: 0 | 1; key: number }[]>();
  for (const e of view.edges) {
    if (e.style.routing === 'straight') continue;
    const pts = first.get(e.id)!;
    [e.from, e.to].forEach((end, i) => {
      if (!plans.get(e.id)![i].spread || pts.length < 2) return;
      const side = nearestSide(boxes.get(end.node)!, i === 0 ? pts[0] : pts[pts.length - 1]);
      const o = at(i === 0 ? e.to : e.from);
      const k = end.node + '\n' + side;
      sides.set(k, [...(sides.get(k) ?? []), { edge: e.id, end: i as 0 | 1, key: side === 'top' || side === 'bottom' ? o.x : o.y }]);
    });
  }
  for (const [k, ends] of [...sides].sort((a, b) => cmp(a[0], b[0]))) {
    const [id, side] = k.split('\n') as [string, Side];
    ends.sort((a, b) => a.key - b.key || cmp(a.edge, b.edge) || a.end - b.end);
    ends.forEach((x, i) => {
      plans.get(x.edge)![x.end] = { at: onSide(boxes.get(id)!, side, (i + 1) / (ends.length + 1)), dir: side, outlineOf: id, spread: true };
    });
  }
  for (const [from, to] of plans.values()) straighten(from, to, boxes, grid);

  const second = sides.size ? await routeAll() : first;
  const containers = view.nodes.filter((n) => n.container).map((n) => boxes.get(n.id)!);
  const leaves = view.nodes.filter((n) => !n.container).map((n) => boxes.get(n.id)!);
  return new Map(
    view.edges.map((e) => {
      // libavoid draws a straight line where it finds no route; such an edge keeps its first route.
      let pts = e.style.routing === 'orthogonal' && !orthogonal(second.get(e.id)!) ? first.get(e.id)! : second.get(e.id)!;
      pts = [...pts];
      const [from, to] = plans.get(e.id)!;
      if (from.outlineOf && pts.length > 1) pts[0] = ontoOutline(pts[0], from.dir as Side, boxes.get(from.outlineOf)!, outlineOf(from.outlineOf));
      if (to.outlineOf && pts.length > 1) pts[pts.length - 1] = ontoOutline(pts[pts.length - 1], to.dir as Side, boxes.get(to.outlineOf)!, outlineOf(to.outlineOf));
      pts = simplify(pts.map((p) => ({ x: Math.round(p.x), y: Math.round(p.y) })));
      return [e.id, e.style.routing === 'orthogonal' ? awayFromBorders(pts, containers, leaves, grid) : pts];
    }),
  );
}

// join returns the points from an end with a direction to the first point of its route: the end,
// and a corner when libavoid's nudging has moved the route's stub point off the end's normal.
function join(p: Plan, first: Point | undefined): Point[] {
  if (!first || p.dir === 'all') return [p.at];
  const vertical = p.dir === 'top' || p.dir === 'bottom';
  if (vertical ? first.x === p.at.x : first.y === p.at.y) return [p.at];
  return [p.at, vertical ? { x: p.at.x, y: first.y } : { x: first.x, y: p.at.y }];
}

function request(p: Plan): RouteEnd {
  if (p.dir === 'all') return { at: p.at, obstacle: p.obstacle };
  return { at: { x: p.at.x + STUB * step[p.dir].x, y: p.at.y + STUB * step[p.dir].y } };
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
// border, closer to it than gap, to gap from it, unless that would cross a leaf box, so an edge
// is never drawn on top of a border. The first and last segments stay, so the ends do not move.
export function awayFromBorders(route: Point[], containers: Box[], leaves: Box[], gap: number): Point[] {
  const pts = route.map((p) => ({ ...p }));
  for (let i = 1; i + 2 < pts.length; i++) {
    const a = pts[i];
    const b = pts[i + 1];
    const k = a.x === b.x ? 'x' : a.y === b.y ? 'y' : undefined;
    if (!k) continue;
    const o = k === 'x' ? 'y' : 'x';
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
      if (leaves.some((l) => touched.slice(1).some((p, j) => segmentHitsBox(touched[j], p, l)))) continue;
      pts[i] = trial[i];
      pts[i + 1] = trial[i + 1];
      break;
    }
  }
  return simplify(pts);
}

// placeLabels places the middle labels in edge order (spec 7.5), each avoiding leaf nodes,
// container labels and the labels placed before it.
export function placeLabels(g: EdgeGeometry, routes: Map<string, Point[]>, text: TextMeasure): Map<string, { text: TextBlock; box: Box }> {
  const avoid = [...g.view.nodes.filter((n) => !n.container).map((n) => g.boxes.get(n.id)!), ...g.headers.values()];
  const out = new Map<string, { text: TextBlock; box: Box }>();
  for (const e of g.view.edges) {
    if (!e.labels.mid) continue;
    const t = textBlock(e.labels.mid, 0, { size: e.style.font_size, weight: 400 }, text);
    const size = { w: ceilTo(t.w + 2 * LABEL_PAD, g.grid), h: t.h };
    const others = g.view.edges.filter((o) => o.id !== e.id).map((o) => routes.get(o.id)!);
    const box = placeLabel(routes.get(e.id)!, size, e.hint?.label_pos, avoid, others);
    avoid.push(box);
    out.set(e.id, { text: t, box });
  }
  return out;
}

function nearestSide(b: Box, p: Point): Side {
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

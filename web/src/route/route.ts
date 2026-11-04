import { center, type Box, type Point, type Size } from '../geometry/geometry';
import type { CustomShape, Side, ViewNode } from '../protocol/view';
import { MARKER_HALF } from './markers';

export interface PortPos extends Point {
  side: Side;
}

// sideFacing returns the side of b that faces point p.
export function sideFacing(b: Box, p: Point): Side {
  const c = center(b);
  const dx = (p.x - c.x) / Math.max(b.w, 1);
  const dy = (p.y - c.y) / Math.max(b.h, 1);
  if (Math.abs(dx) >= Math.abs(dy)) return dx >= 0 ? 'right' : 'left';
  return dy >= 0 ? 'bottom' : 'top';
}

// rowSide returns the end of a row line of box b that faces the other endpoint, whose box is o (a
// point for a port): the side o lies on, or, when the two overlap horizontally, the side where
// their edges are closer, so both ends of an edge between two rows take the same side.
export function rowSide(b: Box, o: Box): 'left' | 'right' {
  if (o.x >= b.x + b.w) return 'right';
  if (o.x + o.w <= b.x) return 'left';
  return Math.abs(o.x - b.x) < Math.abs(o.x + o.w - b.x - b.w) ? 'left' : 'right';
}

// onSide returns the point at fraction f along side of b, from its top or left end.
export function onSide(b: Box, side: Side, f: number): PortPos {
  switch (side) {
    case 'top':
      return { x: b.x + f * b.w, y: b.y, side };
    case 'bottom':
      return { x: b.x + f * b.w, y: b.y + b.h, side };
    case 'left':
      return { x: b.x, y: b.y + f * b.h, side };
    case 'right':
      return { x: b.x + b.w, y: b.y + f * b.h, side };
  }
}

function nearestSide(fx: number, fy: number): Side {
  const d: [Side, number][] = [['top', fy], ['right', 1 - fx], ['bottom', 1 - fy], ['left', fx]];
  return d.reduce((a, b) => (b[1] < a[1] ? b : a))[0];
}

// portPositions places a node's ports (spec 7.5): a custom shape's anchor, else a pinned side
// and offset, else its side, ports on one side spread evenly in declaration order; an auto
// port faces the point toward gives for it. An auto port without edges has no position.
export function portPositions(n: ViewNode, b: Box, custom: CustomShape | undefined, toward: Map<string, Point>): Map<string, PortPos> {
  const out = new Map<string, PortPos>();
  const spread: Record<Side, string[]> = { top: [], right: [], bottom: [], left: [] };
  for (const p of n.ports ?? []) {
    const anchor = custom?.anchors?.[p.name];
    if (anchor) {
      out.set(p.name, { x: b.x + anchor[0] * b.w, y: b.y + anchor[1] * b.h, side: nearestSide(anchor[0], anchor[1]) });
      continue;
    }
    const pin = n.pin?.ports?.[p.name];
    if (pin?.offset !== undefined) {
      out.set(p.name, onSide(b, pin.side, pin.offset));
      continue;
    }
    const other = toward.get(p.name);
    const side = pin?.side ?? (p.side !== 'auto' ? p.side : other ? sideFacing(b, other) : undefined);
    if (side) spread[side].push(p.name);
  }
  for (const side of ['top', 'right', 'bottom', 'left'] as Side[]) {
    const names = spread[side];
    names.forEach((name, i) => out.set(name, onSide(b, side, (i + 1) / (names.length + 1))));
  }
  return out;
}

export function insidePolygon(p: Point, poly: Point[]): boolean {
  let inside = false;
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const a = poly[i];
    const b = poly[j];
    if (a.y > p.y !== b.y > p.y && p.x < ((b.x - a.x) * (p.y - a.y)) / (b.y - a.y) + a.x) inside = !inside;
  }
  return inside;
}

// exit returns where the segment from p, inside the convex polygon, to q leaves it.
function exit(p: Point, q: Point, poly: Point[]): Point {
  let best = 0;
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const a = poly[j];
    const b = poly[i];
    const d = (q.x - p.x) * (b.y - a.y) - (q.y - p.y) * (b.x - a.x);
    if (d === 0) continue;
    const t = ((a.x - p.x) * (b.y - a.y) - (a.y - p.y) * (b.x - a.x)) / d;
    const u = ((a.x - p.x) * (q.y - p.y) - (a.y - p.y) * (q.x - p.x)) / d;
    if (t >= 0 && t <= 1 && u >= 0 && u <= 1) best = Math.max(best, t);
  }
  return { x: p.x + best * (q.x - p.x), y: p.y + best * (q.y - p.y) };
}

function clipStart(pts: Point[], poly: Point[]): Point[] {
  if (pts.length < 2 || !insidePolygon(pts[0], poly)) return pts;
  let i = 0;
  while (i + 2 < pts.length && insidePolygon(pts[i + 1], poly)) i++;
  if (insidePolygon(pts[i + 1], poly)) return pts.slice(i);
  return [exit(pts[i], pts[i + 1], poly), ...pts.slice(i + 1)];
}

// clip trims a route that starts or ends inside a shape outline (an end attached to a node's
// center) so it starts and ends on the outline, rounds it to whole pixels and drops repeated
// and collinear points, keeping the route on every point of via.
export function clip(route: Point[], from: Point[] | null, to: Point[] | null, via: Point[] = []): Point[] {
  let pts = route;
  if (from) pts = clipStart(pts, from);
  if (to) pts = clipStart([...pts].reverse(), to).reverse();
  return simplify(pts.map(round), via);
}

function round(p: Point): Point {
  return { x: Math.round(p.x), y: Math.round(p.y) };
}

// simplify drops repeated points and the middle of three points in line, which also removes a
// back-and-forth along one line, unless that would take the route off a point of via.
export function simplify(pts: Point[], via: Point[] = []): Point[] {
  const out: Point[] = [];
  for (const p of pts) {
    const last = out[out.length - 1];
    if (last && last.x === p.x && last.y === p.y) continue;
    const prev = out[out.length - 2];
    if (prev && last && (prev.x - last.x) * (last.y - p.y) === (prev.y - last.y) * (last.x - p.x)) {
      const needed = via.some((v) => (onSegment(v, prev, last) || onSegment(v, last, p)) && !onSegment(v, prev, p));
      if (!needed) {
        out[out.length - 1] = p;
        continue;
      }
    }
    out.push(p);
  }
  return out;
}

function onSegment(p: Point, a: Point, b: Point): boolean {
  const inLine = (b.x - a.x) * (p.y - a.y) === (b.y - a.y) * (p.x - a.x);
  return inLine && p.x >= Math.min(a.x, b.x) && p.x <= Math.max(a.x, b.x) && p.y >= Math.min(a.y, b.y) && p.y <= Math.max(a.y, b.y);
}

// onRoute reports whether p lies on the route.
export function onRoute(route: Point[], p: Point): boolean {
  return route.slice(1).some((q, i) => onSegment(p, route[i], q));
}

// pointAlong returns the point at fraction t of the route's length, and whether the segment it
// lies on runs horizontally.
export function pointAlong(route: Point[], t: number): Point & { horizontal: boolean } {
  const lengths = route.slice(1).map((p, i) => Math.hypot(p.x - route[i].x, p.y - route[i].y));
  let remaining = t * lengths.reduce((a, b) => a + b, 0);
  for (let i = 0; i < lengths.length; i++) {
    if (remaining <= lengths[i] && lengths[i] > 0) {
      const f = remaining / lengths[i];
      const a = route[i];
      const b = route[i + 1];
      return { x: a.x + f * (b.x - a.x), y: a.y + f * (b.y - a.y), horizontal: Math.abs(b.x - a.x) >= Math.abs(b.y - a.y) };
    }
    remaining -= lengths[i];
  }
  return { ...route[route.length - 1], horizontal: true };
}

// segmentHitsBox reports whether the segment from a to b passes through box r.
export function segmentHitsBox(a: Point, b: Point, r: Box): boolean {
  let t0 = 0;
  let t1 = 1;
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  for (const [p, q] of [[-dx, a.x - r.x], [dx, r.x + r.w - a.x], [-dy, a.y - r.y], [dy, r.y + r.h - a.y]]) {
    if (p === 0) {
      if (q <= 0) return false;
      continue;
    }
    const t = q / p;
    if (p < 0) t0 = Math.max(t0, t);
    else t1 = Math.min(t1, t);
    if (t0 >= t1) return false;
  }
  return true;
}

const LABEL_TRIES = [0.5, 0.4, 0.6, 0.3, 0.7, 0.2, 0.8];
const LABEL_GAP = 4;

// overlapScore scores how much box b overlaps the boxes in avoid, plus how many route segments
// cross it.
export function overlapScore(b: Box, avoid: Box[], routes: Point[][]): number {
  return (
    avoid.reduce((sum, a) => sum + Math.max(0, Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x)) * Math.max(0, Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y)), 0) +
    routes.reduce((sum, r) => sum + r.slice(1).filter((p, i) => segmentHitsBox(r[i], p, b)).length, 0)
  );
}

// placeEndLabel returns the box of a start or end label of the given size next to the end of
// route: beside its last segment, clear of the widest marker, from LABEL_GAP outside the node
// outward, on either side of the segment, or moved up to two labels further out along it. It
// takes the first of these that overlaps no box in avoid, crosses no route and has no other route
// within LABEL_GAP of it, so it is not read as another edge's label; else the one that overlaps
// least.
export function placeEndLabel(route: Point[], size: Size, avoid: Box[], others: Point[][]): Box {
  const tip = route[route.length - 1];
  let i = route.length - 2;
  while (i > 0 && route[i].x === tip.x && route[i].y === tip.y) i--;
  const tail = route[Math.max(0, i)];
  const off = MARKER_HALF + 1;
  const horizontal = Math.abs(tip.x - tail.x) >= Math.abs(tip.y - tail.y);
  const out = horizontal ? Math.sign(tail.x - tip.x) || 1 : Math.sign(tail.y - tip.y) || 1;
  const at = (side: number, step: number): Box => {
    const along = LABEL_GAP + step * ((horizontal ? size.w : size.h) + LABEL_GAP);
    const across = side < 0 ? -off - (horizontal ? size.h : size.w) : off;
    const near = out > 0 ? along : -along - (horizontal ? size.w : size.h);
    return horizontal
      ? { x: Math.round(tip.x + near), y: Math.round(tip.y + across), ...size }
      : { x: Math.round(tip.x + across), y: Math.round(tip.y + near), ...size };
  };
  const score = (b: Box) =>
    overlapScore(b, avoid, [route, ...others]) + overlapScore({ x: b.x - LABEL_GAP, y: b.y - LABEL_GAP, w: b.w + 2 * LABEL_GAP, h: b.h + 2 * LABEL_GAP }, [], others);
  let best = at(-1, 0);
  let least = Infinity;
  for (const step of [0, 1, 2]) {
    for (const side of [-1, 1]) {
      const b = at(side, step);
      const o = score(b);
      if (o === 0) return b;
      if (o < least) [best, least] = [b, o];
    }
  }
  return best;
}

// placeLabel returns the box of a middle label of the given size: centered on the route at pos
// when the layout pins it; otherwise the first clear one of a few points around the middle,
// first on the route and then beside it (beside it first when the route is too short to hold
// the label), where clear means overlapping no box in avoid and
// crossing no other route; if none is clear, the one that overlaps least.
export function placeLabel(route: Point[], size: Size, pos: number | undefined, avoid: Box[], others: Point[][]): Box {
  const at = (t: number, side: number): Box => {
    const c = pointAlong(route, t);
    const dx = c.horizontal ? 0 : side * (size.w / 2 + LABEL_GAP);
    const dy = c.horizontal ? side * (size.h / 2 + LABEL_GAP) : 0;
    return { x: Math.round(c.x + dx - size.w / 2), y: Math.round(c.y + dy - size.h / 2), ...size };
  };
  if (pos !== undefined) return at(pos, 0);
  const length = route.slice(1).reduce((sum, p, i) => sum + Math.hypot(p.x - route[i].x, p.y - route[i].y), 0);
  const mid = pointAlong(route, 0.5);
  const short = length < (mid.horizontal ? size.w : size.h) + 2 * LABEL_GAP;
  let best = at(LABEL_TRIES[0], short ? 1 : 0);
  let least = Infinity;
  for (const side of short ? [1, -1, 0] : [0, -1, 1]) {
    for (const t of LABEL_TRIES) {
      const b = at(t, side);
      const o = overlapScore(b, avoid, others);
      if (o === 0) return b;
      if (o < least) {
        best = b;
        least = o;
      }
    }
  }
  return best;
}

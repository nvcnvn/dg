import type { Box, Point } from '../geometry/geometry';

export const ARROW_SIZE = 10;

function inside(p: Point, b: Box): boolean {
  return p.x >= b.x && p.x <= b.x + b.w && p.y >= b.y && p.y <= b.y + b.h;
}

// exit returns where the segment from p, inside b, to q leaves b.
function exit(p: Point, q: Point, b: Box): Point {
  const t = (from: number, to: number, lo: number, hi: number) =>
    to > from ? (hi - from) / (to - from) : to < from ? (lo - from) / (to - from) : Infinity;
  const s = Math.min(1, t(p.x, q.x, b.x, b.x + b.w), t(p.y, q.y, b.y, b.y + b.h));
  return { x: p.x + s * (q.x - p.x), y: p.y + s * (q.y - p.y) };
}

// clip trims a route that runs from inside the source box to inside the
// target box so it starts and ends on their borders, rounds it to whole
// pixels and drops repeated and collinear points.
export function clip(route: Point[], from: Box, to: Box): Point[] {
  if (route.length < 2) return route.map(round);
  let i = 0;
  while (i + 2 < route.length && inside(route[i + 1], from)) i++;
  let j = route.length - 1;
  while (j - 2 >= i && inside(route[j - 1], to)) j--;
  const pts = route.slice(i, j + 1);
  if (inside(pts[0], from) && !inside(pts[1], from)) pts[0] = exit(pts[0], pts[1], from);
  const n = pts.length - 1;
  if (inside(pts[n], to) && !inside(pts[n - 1], to)) pts[n] = exit(pts[n], pts[n - 1], to);
  return simplify(pts.map(round));
}

function round(p: Point): Point {
  return { x: Math.round(p.x), y: Math.round(p.y) };
}

function simplify(pts: Point[]): Point[] {
  const out: Point[] = [];
  for (const p of pts) {
    const last = out[out.length - 1];
    if (last && last.x === p.x && last.y === p.y) continue;
    const prev = out[out.length - 2];
    if (prev && last && (prev.x - last.x) * (last.y - p.y) === (prev.y - last.y) * (last.x - p.x)) {
      out[out.length - 1] = p;
      continue;
    }
    out.push(p);
  }
  return out;
}

// pointAlong returns the point at fraction t of the route's length.
export function pointAlong(route: Point[], t: number): Point {
  const lengths = route.slice(1).map((p, i) => Math.hypot(p.x - route[i].x, p.y - route[i].y));
  let remaining = t * lengths.reduce((a, b) => a + b, 0);
  for (let i = 0; i < lengths.length; i++) {
    if (remaining <= lengths[i] && lengths[i] > 0) {
      const f = remaining / lengths[i];
      return { x: route[i].x + f * (route[i + 1].x - route[i].x), y: route[i].y + f * (route[i + 1].y - route[i].y) };
    }
    remaining -= lengths[i];
  }
  return route[route.length - 1];
}

// openArrow returns the two barb ends of an open arrowhead at the route's end.
export function openArrow(route: Point[]): [Point, Point] | undefined {
  const tip = route[route.length - 1];
  const tail = route[route.length - 2];
  if (!tip || !tail) return undefined;
  const len = Math.hypot(tip.x - tail.x, tip.y - tail.y);
  if (len === 0) return undefined;
  const ux = (tip.x - tail.x) / len;
  const uy = (tip.y - tail.y) / len;
  const back = { x: tip.x - ux * ARROW_SIZE, y: tip.y - uy * ARROW_SIZE };
  const half = ARROW_SIZE / 2;
  return [
    { x: back.x - uy * half, y: back.y + ux * half },
    { x: back.x + uy * half, y: back.y - ux * half },
  ];
}

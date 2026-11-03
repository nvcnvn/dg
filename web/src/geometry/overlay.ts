import type { Box, Point } from './geometry';

// Hull is an overlay's outline: the convex hull of its members' boxes grown by padding, with
// round corners, as SVG path data; its bounding box; and the left end of its top edge, where
// its label goes.
export interface Hull {
  path: string;
  box: Box;
  top: Point;
}

// convexHull returns the convex hull of points, clockwise on screen (y down), without
// collinear points, starting from the leftmost, topmost one.
export function convexHull(points: Point[]): Point[] {
  const pts = [...points].sort((a, b) => a.x - b.x || a.y - b.y);
  if (pts.length < 3) return pts;
  const cross = (o: Point, a: Point, b: Point) => (a.x - o.x) * (b.y - o.y) - (a.y - o.y) * (b.x - o.x);
  const half = (list: Point[]) => {
    const out: Point[] = [];
    for (const p of list) {
      while (out.length >= 2 && cross(out[out.length - 2], out[out.length - 1], p) <= 0) out.pop();
      out.push(p);
    }
    return out.slice(0, -1);
  };
  return [...half(pts), ...half([...pts].reverse())];
}

// hull returns the outline around boxes at distance pad: the hull's edges moved out by pad,
// joined by arcs of radius pad around its corners. Coordinates are rounded to whole pixels.
export function hull(boxes: Box[], pad: number): Hull {
  const corners = boxes.flatMap((b) => [
    { x: b.x, y: b.y },
    { x: b.x + b.w, y: b.y },
    { x: b.x + b.w, y: b.y + b.h },
    { x: b.x, y: b.y + b.h },
  ]);
  const h = convexHull(corners);
  const xs = h.map((p) => p.x);
  const ys = h.map((p) => p.y);
  const box = { x: Math.min(...xs) - pad, y: Math.min(...ys) - pad, w: Math.max(...xs) - Math.min(...xs) + 2 * pad, h: Math.max(...ys) - Math.min(...ys) + 2 * pad };
  const top = { x: Math.min(...h.filter((p) => p.y === Math.min(...ys)).map((p) => p.x)), y: box.y };
  const r = (v: number) => String(Math.round(v) || 0);
  if (h.length < 3 || pad <= 0) {
    const poly = h.length < 3 ? [{ x: box.x, y: box.y }, { x: box.x + box.w, y: box.y }, { x: box.x + box.w, y: box.y + box.h }, { x: box.x, y: box.y + box.h }] : h;
    return { path: poly.map((p, i) => `${i ? 'L' : 'M'}${r(p.x)} ${r(p.y)}`).join(' ') + ' Z', box, top };
  }
  // The hull runs clockwise on screen, so each edge's outward normal is its direction turned a
  // quarter counterclockwise on screen: (dy, -dx).
  const normal = (a: Point, b: Point): Point => {
    const len = Math.hypot(b.x - a.x, b.y - a.y);
    return { x: (b.y - a.y) / len, y: -(b.x - a.x) / len };
  };
  const parts: string[] = [];
  for (let i = 0; i < h.length; i++) {
    const a = h[i];
    const b = h[(i + 1) % h.length];
    const c = h[(i + 2) % h.length];
    const n1 = normal(a, b);
    const n2 = normal(b, c);
    const start = { x: a.x + n1.x * pad, y: a.y + n1.y * pad };
    const end = { x: b.x + n1.x * pad, y: b.y + n1.y * pad };
    const next = { x: b.x + n2.x * pad, y: b.y + n2.y * pad };
    if (i === 0) parts.push(`M${r(start.x)} ${r(start.y)}`);
    parts.push(`L${r(end.x)} ${r(end.y)}`, `A${r(pad)} ${r(pad)} 0 0 1 ${r(next.x)} ${r(next.y)}`);
  }
  return { path: parts.join(' ') + ' Z', box, top };
}

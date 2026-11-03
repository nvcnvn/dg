import { cmp, containerSize, intersects, roundTo, type Box, type Layout, type NodeMeasure, type Point, type Size } from '../geometry/geometry';
import { arrangeFrame } from '../geometry/policy';
import type { ResolvedView } from '../protocol/view';

const MAX_RINGS = 1000;
const CLEARANCE = 4;

// placeSpiral places the unpinned nodes of a view with pinned nodes (spec 7.4). Frames are
// filled innermost first, so a container's size is known before it is placed; within a frame,
// each unpinned node in sorted ID order starts at the centroid of its placed neighbors in that
// frame, else at the center of the frame, and moves outward in a spiral on the grid until it
// keeps four grid units of clearance from every placed node of the frame. pins are positions
// relative to the frame's content origin; a negative one inside a container is drawn at 0 (spec
// 7.1), so it is placed there. A stack or grid frame places its children by its policy instead.
export function placeSpiral(view: ResolvedView, measures: Map<string, NodeMeasure>, pins: Map<string, Point>, grid: number): Layout {
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const kids = new Map<string, string[]>();
  for (const n of view.nodes) {
    const f = n.frame ?? '';
    kids.set(f, [...(kids.get(f) ?? []), n.id].sort(cmp));
  }
  const chain = (id: string): string[] => {
    const ids = [id];
    for (let f = byId.get(id)?.frame; f; f = byId.get(f)?.frame) ids.push(f);
    return ids;
  };
  const rel = new Map<string, Point>();
  const sizes = new Map<string, Size>();
  const tooSmall = new Set<string>();

  const neighborCentroid = (id: string, frame: string, placed: Set<string>): Point | undefined => {
    const points: Point[] = [];
    for (const e of view.edges) {
      for (const [a, b] of [[e.from.node, e.to.node], [e.to.node, e.from.node]]) {
        if (!chain(a).includes(id) || chain(b).includes(id)) continue;
        const rep = chain(b).find((c) => (byId.get(c)?.frame ?? '') === frame);
        if (rep && placed.has(rep)) {
          const p = rel.get(rep)!;
          const s = sizes.get(rep)!;
          points.push({ x: p.x + s.w / 2, y: p.y + s.h / 2 });
        }
      }
    }
    if (points.length === 0) return undefined;
    return { x: points.reduce((a, p) => a + p.x, 0) / points.length, y: points.reduce((a, p) => a + p.y, 0) / points.length };
  };

  const frameCenter = (frame: string, placed: Box[]): Point => {
    const f = byId.get(frame);
    if (f?.pin?.w !== undefined && f.pin.h !== undefined) {
      const o = measures.get(frame)!.origin;
      const pad = f.style.padding;
      return { x: (f.pin.w - o.x - pad) / 2, y: (f.pin.h - o.y - pad) / 2 };
    }
    if (placed.length === 0) return { x: 0, y: 0 };
    const x0 = Math.min(...placed.map((b) => b.x));
    const y0 = Math.min(...placed.map((b) => b.y));
    const x1 = Math.max(...placed.map((b) => b.x + b.w));
    const y1 = Math.max(...placed.map((b) => b.y + b.h));
    return { x: (x0 + x1) / 2, y: (y0 + y1) / 2 };
  };

  const placeFrame = (frame: string) => {
    const ids = kids.get(frame) ?? [];
    for (const id of ids) {
      const n = byId.get(id)!;
      if (!n.container) {
        sizes.set(id, measures.get(id)!.size);
        continue;
      }
      placeFrame(id);
      const children = (kids.get(id) ?? []).map((c) => ({ ...rel.get(c)!, ...sizes.get(c)! }));
      const { size, tooSmall: small } = containerSize(n, measures.get(id)!, children, grid);
      sizes.set(id, size);
      if (small) tooSmall.add(id);
    }
    if (byId.get(frame)?.policy) {
      arrangeFrame(byId, measures, frame, rel, sizes);
      return;
    }
    const placed: Box[] = [];
    const done = new Set<string>();
    for (const id of ids) {
      const pin = pins.get(id);
      if (!pin) continue;
      const p = frame ? { x: Math.max(0, pin.x), y: Math.max(0, pin.y) } : pin;
      rel.set(id, p);
      placed.push({ ...p, ...sizes.get(id)! });
      done.add(id);
    }
    for (const id of ids) {
      if (done.has(id)) continue;
      const size = sizes.get(id)!;
      const p = spiral(neighborCentroid(id, frame, done) ?? frameCenter(frame, placed), size, placed, frame !== '', grid);
      rel.set(id, p);
      placed.push({ ...p, ...size });
      done.add(id);
    }
  };
  placeFrame('');
  return { rel, sizes, tooSmall };
}

// spiral returns the grid position nearest start, in ring order, where a box of the given size
// keeps the clearance from every placed box; inside a container coordinates are not negative.
export function spiral(start: Point, size: Size, placed: Box[], inContainer: boolean, grid: number): Point {
  const bx = roundTo(start.x - size.w / 2, grid);
  const by = roundTo(start.y - size.h / 2, grid);
  const clear = CLEARANCE * grid;
  for (let r = 0; r <= MAX_RINGS; r++) {
    for (const [i, j] of ring(r)) {
      const x = bx + i * grid;
      const y = by + j * grid;
      if (inContainer && (x < 0 || y < 0)) continue;
      const box = { x: x - clear, y: y - clear, w: size.w + 2 * clear, h: size.h + 2 * clear };
      if (!placed.some((b) => intersects(box, b))) return { x, y };
    }
  }
  return inContainer ? { x: Math.max(0, bx), y: Math.max(0, by) } : { x: bx, y: by };
}

const rings: [number, number][][] = [];

// ring returns the grid offsets at Chebyshev distance r, nearest first.
function ring(r: number): [number, number][] {
  if (!rings[r]) {
    const cells: [number, number][] = [];
    for (let j = -r; j <= r; j++) {
      for (let i = -r; i <= r; i++) {
        if (Math.max(Math.abs(i), Math.abs(j)) === r) cells.push([i, j]);
      }
    }
    rings[r] = cells.sort((a, b) => a[0] ** 2 + a[1] ** 2 - (b[0] ** 2 + b[1] ** 2) || a[1] - b[1] || a[0] - b[0]);
  }
  return rings[r];
}

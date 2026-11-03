import type { Point } from '../geometry/geometry';

// MarkerPart is one stroke of an arrow marker: an open line, a closed polygon or a circle, filled
// with the edge's stroke color or with the background.
export type MarkerPart = { line: Point[] } | { polygon: Point[]; filled: boolean } | { circle: Point; r: number; filled: boolean };

// A marker is drawn in a frame whose origin is the route's end and whose x axis points back along
// the route: [d, o] is d along the route from its end and o to its side.
type Part = { line: [number, number][] } | { polygon: [number, number][]; filled: boolean } | { circle: [number, number]; r: number; filled: boolean };

const bar = (d: number): Part => ({ line: [[d, 7], [d, -7]] });
const ring = (d: number): Part => ({ circle: [d, 0], r: 5, filled: false });
const foot: Part[] = [{ line: [[0, 7], [12, 0], [0, -7]] }];

const MARKERS: Record<string, { length: number; parts: Part[] }> = {
  none: { length: 0, parts: [] },
  arrow: { length: 10, parts: [{ line: [[10, 5], [0, 0], [10, -5]] }] },
  triangle: { length: 12, parts: [{ polygon: [[0, 0], [12, 6], [12, -6]], filled: true }] },
  'triangle-open': { length: 12, parts: [{ polygon: [[0, 0], [12, 6], [12, -6]], filled: false }] },
  diamond: { length: 16, parts: [{ polygon: [[0, 0], [8, 5], [16, 0], [8, -5]], filled: true }] },
  'diamond-open': { length: 16, parts: [{ polygon: [[0, 0], [8, 5], [16, 0], [8, -5]], filled: false }] },
  circle: { length: 8, parts: [{ circle: [4, 0], r: 4, filled: true }] },
  'circle-open': { length: 8, parts: [{ circle: [4, 0], r: 4, filled: false }] },
  'crow-one': { length: 14, parts: [bar(8), bar(14)] },
  'crow-zero-one': { length: 22, parts: [bar(8), ring(17)] },
  'crow-many': { length: 12, parts: foot },
  'crow-one-many': { length: 16, parts: [...foot, bar(16)] },
  'crow-zero-many': { length: 24, parts: [...foot, ring(19)] },
};

// MARKER_HALF is half the widest marker's width.
export const MARKER_HALF = 7;

// markerLength returns how far a marker reaches back along the route from its end.
export function markerLength(name: string): number {
  return MARKERS[name]?.length ?? 0;
}

// marker returns the parts of the named marker at the end of route, oriented along its last
// segment.
export function marker(name: string, route: Point[]): MarkerPart[] {
  const parts = MARKERS[name]?.parts ?? [];
  const tip = route[route.length - 1];
  let i = route.length - 2;
  while (i >= 0 && route[i].x === tip?.x && route[i].y === tip?.y) i--;
  if (!parts.length || i < 0) return [];
  const tail = route[i];
  const len = Math.hypot(tip.x - tail.x, tip.y - tail.y);
  const ux = (tip.x - tail.x) / len;
  const uy = (tip.y - tail.y) / len;
  const at = ([d, o]: [number, number]): Point => ({ x: tip.x - ux * d - uy * o, y: tip.y - uy * d + ux * o });
  return parts.map((p) => {
    if ('line' in p) return { line: p.line.map(at) };
    if ('polygon' in p) return { polygon: p.polygon.map(at), filled: p.filled };
    return { circle: at(p.circle), r: p.r, filled: p.filled };
  });
}

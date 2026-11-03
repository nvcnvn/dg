import { describe, expect, it } from 'vitest';
import { avoidRouter } from '../canvas/avoid';
import { edge, node, text } from '../fixtures';
import type { Box, Point } from '../geometry/geometry';
import type { ResolvedView } from '../protocol/view';
import { awayFromBorders, placeLabels, planPorts, routeEdges, type Router } from './edges';

const wasm = decodeURIComponent(new URL('../../node_modules/libavoid-js/dist/libavoid.wasm', import.meta.url).pathname);
const avoid = avoidRouter(wasm);

async function routes(view: ResolvedView, boxes: Record<string, Box>, router: Router = avoid) {
  const g = { view, boxes: new Map(Object.entries(boxes)), headers: new Map<string, Box>(), grid: 10 };
  return routeEdges(g, planPorts(g), router);
}

const box = (x: number, y: number): Box => ({ x, y, w: 100, h: 40 });

describe('routeEdges', () => {
  it('spreads ends entering one side and keeps routes orthogonal', async () => {
    const view: ResolvedView = { view: 'v', grid: 10, nodes: [node('a'), node('b'), node('t')], edges: [edge('at', 'a', 't'), edge('bt', 'b', 't')] };
    const r = await routes(view, { a: box(100, 0), b: box(250, 0), t: { x: 100, y: 300, w: 300, h: 40 } });
    const ends = ['at', 'bt'].map((id) => r.get(id)!.at(-1)!);
    expect(ends.map((p) => p.y)).toEqual([300, 300]);
    expect(ends[0].x).toBeLessThan(ends[1].x);
    for (const pts of r.values()) expect(pts.slice(1).every((p, i) => p.x === pts[i].x || p.y === pts[i].y)).toBe(true);
  });

  it('leaves a port perpendicular to its side', async () => {
    const view: ResolvedView = {
      view: 'v',
      grid: 10,
      nodes: [node('a', { ports: [{ name: 'p', side: 'bottom' }] }), node('b')],
      edges: [{ ...edge('ab', 'a', 'b'), from: { node: 'a', port: 'p' } }],
    };
    const [p0, p1] = (await routes(view, { a: box(0, 0), b: box(300, 0) })).get('ab')!;
    expect(p0).toEqual({ x: 50, y: 40 });
    expect(p1.x).toBe(50);
    expect(p1.y).toBeGreaterThan(40);
  });

  it('honours a side hint on a polyline edge', async () => {
    const e = { ...edge('ab', 'a', 'b'), hint: { from_side: 'top' as const }, style: { ...edge('x', 'a', 'b').style, routing: 'polyline' as const } };
    const view: ResolvedView = { view: 'v', grid: 10, nodes: [node('a'), node('b')], edges: [e] };
    const [p0, p1] = (await routes(view, { a: box(0, 100), b: box(300, 100) })).get('ab')!;
    expect(p0).toEqual({ x: 50, y: 100 });
    expect(p1.x).toBe(50);
    expect(p1.y).toBeLessThan(100);
  });

  it('runs an edge from a container inward to its child', async () => {
    const view: ResolvedView = { view: 'v', grid: 10, nodes: [node('g', { container: true }), node('c', { frame: 'g' })], edges: [edge('gc', 'g', 'c')] };
    const pts = (await routes(view, { g: { x: 0, y: 0, w: 300, h: 200 }, c: box(100, 100) })).get('gc')!;
    expect(pts[0].y).toBe(200);
    expect(pts.at(-1)).toEqual({ x: 150, y: 140 });
  });

  it('keeps the first route when the second has no route', async () => {
    let calls = 0;
    const flaky: Router = async (o, reqs) => {
      calls++;
      const r = await avoid(o, reqs);
      if (calls === 2) for (const q of reqs) r.set(q.id, [q.from.at, q.to.at]);
      return r;
    };
    const view: ResolvedView = { view: 'v', grid: 10, nodes: [node('a'), node('b'), node('t')], edges: [edge('at', 'a', 't'), edge('bt', 'b', 't')] };
    const r = await routes(view, { a: box(0, 0), b: box(300, 0), t: box(150, 200) }, flaky);
    expect(calls).toBe(2);
    for (const pts of r.values()) expect(pts.slice(1).every((p, i) => p.x === pts[i].x || p.y === pts[i].y)).toBe(true);
  });
});

describe('awayFromBorders', () => {
  const container = { x: 100, y: 0, w: 200, h: 300 };
  const route: Point[] = [{ x: 0, y: 50 }, { x: 104, y: 50 }, { x: 104, y: 250 }, { x: 200, y: 250 }];
  it('moves an inner segment off a border', () => {
    expect(awayFromBorders(route, [container], [], 10)).toEqual([{ x: 0, y: 50 }, { x: 110, y: 50 }, { x: 110, y: 250 }, { x: 200, y: 250 }]);
  });
  it('does not move it onto a leaf', () => {
    expect(awayFromBorders(route, [container], [{ x: 106, y: 100, w: 50, h: 50 }], 10)).toEqual(route);
  });
});

describe('placeLabels', () => {
  it('puts the label beside a route too short to hold it', () => {
    const view: ResolvedView = { view: 'v', grid: 10, nodes: [], edges: [{ ...edge('e', 'a', 'b'), labels: { mid: 'a long label' } }] };
    const route = [{ x: 0, y: 0 }, { x: 0, y: 20 }];
    const { box: b } = placeLabels({ view, boxes: new Map(), headers: new Map(), grid: 10 }, new Map([['e', route]]), text).get('e')!;
    expect(b.x > 0 || b.x + b.w < 0).toBe(true);
  });
});

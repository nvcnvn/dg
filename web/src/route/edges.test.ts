import { describe, expect, it } from 'vitest';
import { avoidRouter } from '../canvas/avoid';
import { edge, node, text } from '../fixtures';
import type { Box, Point } from '../geometry/geometry';
import type { ResolvedView } from '../protocol/view';
import { awayFromBorders, cornerLoop, keepStub, placeLabels, planPorts, routeEdges, type Router } from './edges';

const wasm = decodeURIComponent(new URL('../../node_modules/libavoid-js/dist/libavoid.wasm', import.meta.url).pathname);
const avoid = avoidRouter(wasm);

async function routes(view: ResolvedView, boxes: Record<string, Box>, router: Router = avoid, rows: Record<string, Record<string, number>> = {}) {
  const g = { view, boxes: new Map(Object.entries(boxes)), headers: new Map<string, Box>(), rows: new Map(Object.entries(rows).map(([k, v]) => [k, new Map(Object.entries(v))])), grid: 10 };
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

describe('row ends', () => {
  const view = (from: string, to: string): ResolvedView => ({
    view: 'v',
    grid: 10,
    nodes: [node('t'), node('u')],
    edges: [{ ...edge('e', from, to), from: { node: from, row: 'id' }, to: { node: to, row: 'id' } }],
  });
  const rows = { t: { id: 30 }, u: { id: 130 } };
  it('attach at the end of the row facing the other endpoint', async () => {
    const pts = (await routes(view('t', 'u'), { t: box(0, 0), u: box(300, 100) }, avoid, rows)).get('e')!;
    expect(pts[0]).toEqual({ x: 100, y: 30 });
    expect(pts.at(-1)).toEqual({ x: 300, y: 130 });
    const back = (await routes(view('u', 't'), { t: box(0, 0), u: box(300, 100) }, avoid, rows)).get('e')!;
    expect(back[0]).toEqual({ x: 300, y: 130 });
    expect(back.at(-1)).toEqual({ x: 100, y: 30 });
  });
  it('share a row end with one marker and spread ends with different markers', async () => {
    const crow = (end: string) => ({ ...edge('x', 'a', 'b').style, arrow_start: 'crow-many', arrow_end: end });
    const v: ResolvedView = {
      view: 'v',
      grid: 10,
      nodes: [node('t'), node('u'), node('w')],
      edges: [
        { ...edge('e1', 'u', 't'), from: { node: 'u', row: 'id' }, to: { node: 't', row: 'id' }, style: crow('crow-one') },
        { ...edge('e2', 'w', 't'), from: { node: 'w', row: 'id' }, to: { node: 't', row: 'id' }, style: crow('crow-zero-one') },
        { ...edge('e3', 'w', 't'), from: { node: 'w', row: 'x' }, to: { node: 't', row: 'id' }, style: crow('crow-one') },
      ],
    };
    const r = await routes(v, { t: box(0, 0), u: box(300, 0), w: box(300, 200) }, avoid, { t: { id: 30 }, u: { id: 30 }, w: { id: 230, x: 250 } });
    const ends = ['e1', 'e2', 'e3'].map((id) => r.get(id)!.at(-1)!);
    expect(ends.map((p) => p.x)).toEqual([100, 100, 100]);
    // crow-one and crow-zero-one 16 apart around the row's line, the two crow-one ends together.
    expect(ends.map((p) => p.y)).toEqual([22, 38, 22]);
    // Each runs straight a grid unit past the longest marker at that side (crow-zero-one, 22).
    for (const id of ['e1', 'e2', 'e3']) {
      const pts = r.get(id)!;
      const [a, b] = [pts.at(-1)!, pts.at(-2)!];
      expect(b.y).toBe(a.y);
      expect(b.x - a.x).toBeGreaterThanOrEqual(32);
    }
  });
  it('end at the node itself when it is not drawn as a table', async () => {
    const pts = (await routes(view('t', 'u'), { t: box(0, 0), u: box(300, 100) })).get('e')!;
    const onBorder = (p: Point, b: Box) => (p.x === b.x || p.x === b.x + b.w || p.y === b.y || p.y === b.y + b.h) && p.x >= b.x && p.x <= b.x + b.w && p.y >= b.y && p.y <= b.y + b.h;
    expect(onBorder(pts[0], box(0, 0))).toBe(true);
    expect(onBorder(pts.at(-1)!, box(300, 100))).toBe(true);
  });
  it('take the same side when the tables overlap horizontally', async () => {
    const pts = (await routes(view('t', 'u'), { t: box(0, 0), u: { x: 20, y: 100, w: 100, h: 40 } }, avoid, rows)).get('e')!;
    expect(pts[0]).toEqual({ x: 100, y: 30 });
    expect(pts.at(-1)).toEqual({ x: 120, y: 130 });
  });
});

describe('self-loops', () => {
  it('route around the four corners, then nest further out', async () => {
    const view: ResolvedView = { view: 'v', grid: 10, nodes: [node('a')], edges: ['l1', 'l2', 'l3', 'l4', 'l5'].map((id) => edge(id, 'a', 'a')) };
    const r = await routes(view, { a: box(100, 100) });
    expect(r.get('l1')).toEqual([{ x: 200, y: 120 }, { x: 220, y: 120 }, { x: 220, y: 80 }, { x: 180, y: 80 }, { x: 180, y: 100 }]);
    const corner = (id: string) => [r.get(id)![2].x > 150, r.get(id)![2].y > 120].join();
    expect(new Set(['l1', 'l2', 'l3', 'l4'].map(corner)).size).toBe(4);
    expect(Math.abs(r.get('l5')![1].x - r.get('l5')![0].x)).toBe(40);
  });
  it('take the free corner', async () => {
    const view: ResolvedView = { view: 'v', grid: 10, nodes: [node('a'), node('b')], edges: [edge('l', 'a', 'a')] };
    const r = await routes(view, { a: box(100, 100), b: box(230, 40) });
    expect(r.get('l')![0]).toEqual({ x: 200, y: 120 });
    expect(r.get('l')!.at(-1)).toEqual({ x: 180, y: 140 });
  });
  it('leave a slot to a spread edge on the same side', async () => {
    const view: ResolvedView = { view: 'v', grid: 10, nodes: [node('a'), node('b'), node('c')], edges: [edge('ab', 'a', 'b'), edge('ca', 'c', 'a'), edge('l', 'a', 'a')] };
    const r = await routes(view, { a: box(100, 100), b: box(400, 100), c: box(-200, 100) });
    const loop = r.get('l')!;
    const ab = r.get('ab')!;
    expect([ab[0].x, loop[0].x]).toEqual([200, 200]);
    expect(loop[0].y).toBeLessThan(ab[0].y - 5);
  });
  it('reach far enough for long markers', () => {
    const [pts] = cornerLoop(box(0, 0), 0, 0, 30);
    expect(pts[1].x - pts[0].x).toBe(30);
  });
});

describe('keepStub', () => {
  it('moves the segment after a short first one out to the stub', () => {
    const route = [{ x: 130, y: 260 }, { x: 140, y: 260 }, { x: 140, y: 76 }, { x: 200, y: 76 }];
    expect(keepStub(route, 'right', 20, [])).toEqual([{ x: 130, y: 260 }, { x: 150, y: 260 }, { x: 150, y: 76 }, { x: 200, y: 76 }]);
  });
  it('leaves a long enough first segment and does not move onto a leaf', () => {
    const route = [{ x: 130, y: 260 }, { x: 140, y: 260 }, { x: 140, y: 76 }, { x: 200, y: 76 }];
    expect(keepStub(route, 'right', 10, [])).toEqual(route);
    expect(keepStub(route, 'right', 20, [{ x: 145, y: 100, w: 20, h: 20 }])).toEqual(route);
  });
  it('steps past another route already there', () => {
    const route = [{ x: 130, y: 260 }, { x: 140, y: 260 }, { x: 140, y: 76 }, { x: 200, y: 76 }];
    const other = [{ x: 130, y: 220 }, { x: 150, y: 220 }, { x: 150, y: 50 }];
    expect(keepStub(route, 'right', 20, [], [other], 10)[1]).toEqual({ x: 160, y: 260 });
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
    const [{ box: b }] = placeLabels({ view, boxes: new Map(), headers: new Map(), rows: new Map(), grid: 10 }, new Map([['e', route]]), new Map(), text).get('e')!;
    expect(b.x > 0 || b.x + b.w < 0).toBe(true);
  });

  it('puts start and end labels next to their ends, outside the nodes, and not on each other', () => {
    const view: ResolvedView = {
      view: 'v',
      grid: 10,
      nodes: [node('a'), node('b')],
      edges: [
        { ...edge('e1', 'a', 'b'), labels: { mid: 'm', start: '1', end: '0..*' } },
        { ...edge('e2', 'a', 'b'), labels: { start: '1', end: '1..*' } },
      ],
    };
    const boxes = new Map([['a', box(0, 0)], ['b', box(300, 0)]]);
    const routes = new Map([['e1', [{ x: 100, y: 15 }, { x: 300, y: 15 }]], ['e2', [{ x: 100, y: 25 }, { x: 300, y: 25 }]]]);
    const placed = placeLabels({ view, boxes, headers: new Map(), rows: new Map(), grid: 10 }, routes, new Map(), text);
    const [, s1, e1] = placed.get('e1')!.map((l) => l.box);
    expect(s1.x).toBeGreaterThan(100);
    expect(s1.x).toBeLessThan(120);
    expect(e1.x + e1.w).toBeLessThan(300);
    expect(e1.x + e1.w).toBeGreaterThan(280);
    const all = [...placed.values()].flat().map((l) => l.box);
    for (const [i, p] of all.entries()) for (const q of all.slice(i + 1)) expect(p.x < q.x + q.w && q.x < p.x + p.w && p.y < q.y + q.h && q.y < p.y + p.h).toBe(false);
  });
});

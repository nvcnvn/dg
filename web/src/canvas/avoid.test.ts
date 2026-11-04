import { describe, expect, it } from 'vitest';
import { libavoidWasm } from '../fixtures';
import type { RouteRequest } from '../route/edges';
import { avoidRouter } from './avoid';

const route = avoidRouter(libavoidWasm);

const boxes = new Map([
  ['a', { x: 0, y: 0, w: 100, h: 40 }],
  ['b', { x: 300, y: 0, w: 100, h: 40 }],
  ['wall', { x: 150, y: -100, w: 50, h: 240 }],
]);
const centers = (via: { x: number; y: number }[] = [], type: RouteRequest['type'] = 'orthogonal'): RouteRequest[] => [
  { id: 'e', type, from: { at: { x: 50, y: 20 }, obstacle: 'a' }, to: { at: { x: 350, y: 20 }, obstacle: 'b' }, via },
];

describe('avoidRouter', () => {
  it('routes around an obstacle from center to center', async () => {
    const r = (await route(boxes, centers())).get('e')!;
    expect(r[0]).toEqual({ x: 50, y: 20 });
    expect(r.some((p) => p.y < -100 || p.y > 140)).toBe(true);
  });

  it('starts a free end at its point', async () => {
    const req: RouteRequest[] = [{ id: 'e', type: 'orthogonal', from: { at: { x: 50, y: 200 } }, to: { at: { x: 50, y: 20 }, obstacle: 'a' }, via: [] }];
    expect((await route(boxes, req)).get('e')![0]).toEqual({ x: 50, y: 200 });
  });

  it('passes through via points', async () => {
    expect((await route(boxes, centers([{ x: 175, y: 300 }]))).get('e')).toContainEqual({ x: 175, y: 300 });
  });

  it('hits via points where the route doubles back, without a diagonal', async () => {
    const obstacles = new Map([
      ['a', { x: 0, y: 200, w: 100, h: 40 }],
      ['b', { x: 400, y: 0, w: 100, h: 40 }],
    ]);
    const via = [{ x: 300, y: 300 }, { x: 300, y: 100 }];
    const req: RouteRequest[] = [{ id: 'e', type: 'orthogonal', from: { at: { x: 112, y: 220 } }, to: { at: { x: 450, y: 52 } }, via }];
    const r = (await route(obstacles, req)).get('e')!;
    expect(r).toEqual(expect.arrayContaining(via));
    expect(r.slice(1).every((p, i) => p.x === r[i].x || p.y === r[i].y)).toBe(true);
  });

  it('routes a via point given twice', async () => {
    const via = [{ x: 175, y: 300 }, { x: 175, y: 300 }];
    expect((await route(boxes, centers(via))).get('e')).toContainEqual(via[0]);
  });

  it('gives the same routes twice', async () => {
    expect([...(await route(boxes, centers([], 'polyline')))]).toEqual([...(await route(boxes, centers([], 'polyline')))]);
  });
});

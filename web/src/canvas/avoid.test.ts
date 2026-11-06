import { describe, expect, it, vi } from 'vitest';
import { libavoidWasm } from '../fixtures';
import type { RouteRequest } from '../route/edges';
import { avoidRouter, liveRouter } from './avoid';

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

  // libavoid aborts on an orthogonal leg from a pin to a free point the pin sees along a center
  // line; a is at the origin, 100 by 40.
  describe('a free point on a center line of the pin a leg leaves', () => {
    const a = { x: 0, y: 0, w: 100, h: 40 };
    const b = { x: 400, y: 0, w: 100, h: 40 };
    const pin = (id: string, x: number, y: number) => ({ at: { x, y }, obstacle: id });
    const free = (x: number, y: number) => ({ at: { x, y } });
    it.each<[string, Map<string, typeof a>, RouteRequest]>([
      ['a via on the edge of the buffer', new Map([['a', a], ['b', b]]), { id: 'e', type: 'orthogonal', from: pin('a', 50, 20), to: pin('b', 450, 20), via: [{ x: 110, y: 20 }] }],
      ['a via in the buffer', new Map([['a', a], ['b', b]]), { id: 'e', type: 'orthogonal', from: pin('a', 50, 20), to: pin('b', 450, 20), via: [{ x: 105, y: 20 }] }],
      ['a via in the box', new Map([['a', a], ['b', b]]), { id: 'e', type: 'orthogonal', from: pin('a', 50, 20), to: pin('b', 450, 20), via: [{ x: 50, y: 5 }] }],
      ['a via above the box', new Map([['a', a], ['b', b]]), { id: 'e', type: 'orthogonal', from: pin('a', 50, 20), to: pin('b', 450, 20), via: [{ x: 50, y: -10 }] }],
      ['a stub point in the buffer', new Map([['a', a], ['b', b]]), { id: 'e', type: 'orthogonal', from: pin('a', 50, 20), to: free(105, 20), via: [] }],
      ['a point in a buffer overlapping that of the pin', new Map([['a', a], ['n', { x: 115, y: 0, w: 100, h: 40 }]]), { id: 'e', type: 'orthogonal', from: pin('a', 50, 20), to: free(150, 20), via: [] }],
      ['a point past a buffer, between neighbors that take the line off the side of the box', new Map([['a', { x: 150, y: 260, w: 100, h: 40 }], ['above', { x: 130, y: 190, w: 120, h: 60 }], ['below', { x: 70, y: 300, w: 60, h: 40 }]]), { id: 'e', type: 'orthogonal', from: pin('a', 200, 280), to: free(138, 280), via: [] }],
    ])('routes %s', async (_, obstacles, e) => {
      const r = (await route(obstacles, [e])).get('e')!;
      expect(r[0]).toEqual(e.from.at);
      expect(r).toContainEqual(e.via[0] ?? e.to.at);
    });
  });

  it('routes while a drag moves a center line over a via', async () => {
    const obstacles = new Map([
      ['a', { x: 0, y: 0, w: 100, h: 40 }],
      ['b', { x: 400, y: 0, w: 100, h: 40 }],
    ]);
    const request = (dy: number): RouteRequest => ({ id: 'e', type: 'orthogonal', from: { at: { x: 50, y: 20 + dy }, obstacle: 'a' }, to: { at: { x: 450, y: 20 }, obstacle: 'b' }, via: [{ x: 110, y: 40 }] });
    await route(obstacles, [request(0)]);
    const live = liveRouter(obstacles, new Set(['a']), [request(0)])!;
    for (let dy = 0; dy <= 40; dy += 10) expect(live.route([request(dy)], { x: 0, y: dy }).get('e')).toContainEqual({ x: 110, y: 40 });
    live.destroy();
  });

  it('routes while a drag moves a free end, which a leg from a pin ends at', async () => {
    const obstacles = new Map([
      ['a', { x: 0, y: 0, w: 100, h: 40 }],
      ['b', { x: 400, y: 0, w: 100, h: 40 }],
    ]);
    const request = (dx: number): RouteRequest => ({ id: 'e', type: 'orthogonal', from: { at: { x: 450, y: 20 }, obstacle: 'b' }, to: { at: { x: 200 + dx, y: 20 } }, via: [] });
    await route(obstacles, [request(0)]);
    const live = liveRouter(obstacles, new Set(), [request(0)])!;
    for (const dx of [0, 50, 100]) {
      const r = live.route([request(dx)], { x: 0, y: 0 }).get('e')!;
      expect(r[0]).toEqual({ x: 450, y: 20 });
      expect(r[r.length - 1]).toEqual({ x: 200 + dx, y: 20 });
    }
    live.destroy();
  });

  describe('when libavoid aborts', () => {
    const bad: RouteRequest[] = [{ id: 'e', type: 'orthogonal', from: { at: { x: 50, y: 20 } }, to: { at: { x: 350, y: 20 } }, via: [{ x: NaN, y: NaN }] }];
    it('names the edge and via, and routes again afterwards', async () => {
      await expect(route(boxes, bad)).rejects.toThrow('e via [NaN,NaN]');
      expect((await route(boxes, centers())).get('e')![0]).toEqual({ x: 50, y: 20 });
    });
    it('draws the rest of a drag as lines, and routes again afterwards', async () => {
      await route(boxes, centers());
      const live = liveRouter(boxes, new Set(), bad)!;
      const error = vi.spyOn(console, 'error').mockImplementation(() => {});
      expect(live.route(bad, { x: 0, y: 0 })).toEqual(new Map());
      expect(live.route(bad, { x: 1, y: 0 })).toEqual(new Map());
      expect(error).toHaveBeenCalledOnce();
      error.mockRestore();
      live.destroy();
      expect((await route(boxes, centers())).get('e')![0]).toEqual({ x: 50, y: 20 });
    });
    it('leaves a live router of the old module harmless, and the new module loaded', async () => {
      await route(boxes, centers());
      const stale = liveRouter(boxes, new Set(), centers())!;
      await expect(route(boxes, bad)).rejects.toThrow();
      await route(boxes, centers());
      const error = vi.spyOn(console, 'error').mockImplementation(() => {});
      expect(stale.route(centers(), { x: 0, y: 0 })).toEqual(new Map());
      stale.destroy();
      expect(error).not.toHaveBeenCalled();
      error.mockRestore();
      const fresh = liveRouter(boxes, new Set(), centers())!;
      expect(fresh.route(centers(), { x: 0, y: 0 }).get('e')![0]).toEqual({ x: 50, y: 20 });
      fresh.destroy();
    });
    it('gives no live router while the module loads again', async () => {
      await expect(route(boxes, bad)).rejects.toThrow();
      const loading = route(boxes, centers());
      expect(liveRouter(boxes, new Set(), centers())).toBeUndefined();
      await loading;
    });
    it('reloads for a route that was waiting when another aborted', async () => {
      await expect(route(boxes, bad)).rejects.toThrow();
      const [first, second] = await Promise.allSettled([route(boxes, bad), route(boxes, centers())]);
      expect(first.status).toBe('rejected');
      expect(second.status).toBe('fulfilled');
    });
  });
});

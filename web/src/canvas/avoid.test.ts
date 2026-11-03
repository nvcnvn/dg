import { describe, expect, it } from 'vitest';
import { routeAvoid } from './avoid';

const wasm = decodeURIComponent(new URL('../../node_modules/libavoid-js/dist/libavoid.wasm', import.meta.url).pathname);

describe('routeAvoid', () => {
  it('routes around an obstacle', async () => {
    const boxes = new Map([
      ['a', { x: 0, y: 0, w: 100, h: 40 }],
      ['b', { x: 300, y: 0, w: 100, h: 40 }],
      ['wall', { x: 150, y: -100, w: 50, h: 240 }],
    ]);
    const ids = ['a', 'b', 'wall'];
    const routes = await routeAvoid(boxes, ids, new Set(ids), [{ id: 'e', from: 'a', to: 'b' }], wasm);
    const route = routes.get('e')!;
    expect(route.length).toBeGreaterThan(2);
    expect(route.some((p) => p.y < -100 || p.y > 140)).toBe(true);
  });
});

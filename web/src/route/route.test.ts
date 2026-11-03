import { describe, expect, it } from 'vitest';
import { ARROW_SIZE, clip, openArrow, pointAlong } from './route';

const a = { x: 0, y: 0, w: 100, h: 40 };
const b = { x: 200, y: 100, w: 100, h: 40 };

describe('clip', () => {
  it.each([
    ['straight', [{ x: 50, y: 20 }, { x: 250, y: 20 }], { ...b, y: 0 }, [{ x: 100, y: 20 }, { x: 200, y: 20 }]],
    [
      'elbow with a vertex inside the source',
      [{ x: 50, y: 20 }, { x: 60, y: 20 }, { x: 150.4, y: 20 }, { x: 150.4, y: 120 }, { x: 250, y: 120 }],
      b,
      [{ x: 100, y: 20 }, { x: 150, y: 20 }, { x: 150, y: 120 }, { x: 200, y: 120 }],
    ],
  ])('%s', (_name, route, to, want) => {
    expect(clip(route, a, to)).toEqual(want);
  });
});

describe('pointAlong', () => {
  it('finds the midpoint by length', () => {
    expect(pointAlong([{ x: 0, y: 0 }, { x: 100, y: 0 }, { x: 100, y: 100 }], 0.5)).toEqual({ x: 100, y: 0 });
    expect(pointAlong([{ x: 0, y: 0 }, { x: 0, y: 10 }], 0.5)).toEqual({ x: 0, y: 5 });
  });
});

describe('openArrow', () => {
  it('draws barbs behind the tip', () => {
    expect(openArrow([{ x: 0, y: 0 }, { x: 100, y: 0 }])).toEqual([
      { x: 100 - ARROW_SIZE, y: ARROW_SIZE / 2 },
      { x: 100 - ARROW_SIZE, y: -ARROW_SIZE / 2 },
    ]);
    expect(openArrow([{ x: 1, y: 1 }])).toBeUndefined();
  });
});

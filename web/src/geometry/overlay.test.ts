import { describe, expect, it } from 'vitest';
import { convexHull, hull } from './overlay';

describe('overlay hull', () => {
  it('runs clockwise on screen from the top left, without collinear points', () => {
    const pts = [
      { x: 0, y: 0 },
      { x: 5, y: 0 },
      { x: 10, y: 0 },
      { x: 10, y: 10 },
      { x: 0, y: 10 },
      { x: 4, y: 4 },
    ];
    expect(convexHull(pts)).toEqual([
      { x: 0, y: 0 },
      { x: 10, y: 0 },
      { x: 10, y: 10 },
      { x: 0, y: 10 },
    ]);
  });

  it('draws one box as a rectangle with round corners grown by the padding', () => {
    const h = hull([{ x: 0, y: 0, w: 100, h: 40 }], 10);
    expect(h.box).toEqual({ x: -10, y: -10, w: 120, h: 60 });
    expect(h.path).toBe('M0 -10 L100 -10 A10 10 0 0 1 110 0 L110 40 A10 10 0 0 1 100 50 L0 50 A10 10 0 0 1 -10 40 L-10 0 A10 10 0 0 1 0 -10 Z');
  });

  it('spans boxes apart from each other', () => {
    const h = hull([{ x: 100, y: 50, w: 10, h: 10 }, { x: 0, y: 0, w: 10, h: 10 }], 5);
    expect(h.box).toEqual({ x: -5, y: -5, w: 120, h: 70 });
    expect(h.top).toEqual({ x: 0, y: -5 });
  });
});

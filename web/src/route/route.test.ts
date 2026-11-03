import { describe, expect, it } from 'vitest';
import { node } from '../fixtures';
import { outline } from '../geometry/shapes';
import { clip, placeEndLabel, placeLabel, portPositions, rowSide, segmentHitsBox, sideFacing } from './route';

describe('clip', () => {
  const a = { x: 0, y: 0, w: 100, h: 40 };
  const b = { x: 300, y: 100, w: 100, h: 40 };
  it('trims a route from center to center to the borders', () => {
    const route = [{ x: 50, y: 20 }, { x: 200, y: 20 }, { x: 200, y: 120 }, { x: 350, y: 120 }];
    expect(clip(route, outline('rect', a), outline('rect', b))).toEqual([{ x: 100, y: 20 }, { x: 200, y: 20 }, { x: 200, y: 120 }, { x: 300, y: 120 }]);
  });
  it('trims to an ellipse outline', () => {
    const route = [{ x: 50, y: 20 }, { x: 200, y: 20 }];
    expect(clip(route, outline('ellipse', a), null)[0]).toEqual({ x: 100, y: 20 });
    expect(clip([{ x: 50, y: 20 }, { x: 50, y: 200 }], outline('diamond', a), null)[0]).toEqual({ x: 50, y: 40 });
  });
  it('leaves ends that are not inside a shape', () => {
    const route = [{ x: 100, y: 20 }, { x: 300, y: 20 }];
    expect(clip(route, null, null)).toEqual(route);
  });
});

describe('portPositions', () => {
  const b = { x: 0, y: 0, w: 100, h: 60 };
  it('spreads ports on a side in declaration order', () => {
    const n = node('n', { ports: [{ name: 'a', side: 'top' }, { name: 'b', side: 'top' }, { name: 'c', side: 'left' }] });
    const p = portPositions(n, b, undefined, new Map());
    expect(p.get('a')).toMatchObject({ y: 0, side: 'top' });
    expect(p.get('a')!.x).toBeCloseTo(100 / 3);
    expect(p.get('b')!.x).toBeCloseTo(200 / 3);
    expect(p.get('c')).toEqual({ x: 0, y: 30, side: 'left' });
  });
  it('prefers an anchor, then a pinned offset', () => {
    const n = node('n', { ports: [{ name: 'a', side: 'top' }, { name: 'b', side: 'top' }], pin: { x: 0, y: 0, ports: { b: { side: 'bottom', offset: 0.25 } } } });
    const custom = { svg: '', text_box: [0, 0, 1, 1] as [number, number, number, number], stretch: 'uniform' as const, anchors: { a: [1, 0.5] as [number, number] } };
    const p = portPositions(n, b, custom, new Map());
    expect(p.get('a')).toEqual({ x: 100, y: 30, side: 'right' });
    expect(p.get('b')).toEqual({ x: 25, y: 60, side: 'bottom' });
  });
  it('turns an auto port toward its other end and leaves an unused one out', () => {
    const n = node('n', { ports: [{ name: 'a', side: 'auto' }, { name: 'b', side: 'auto' }] });
    const p = portPositions(n, b, undefined, new Map([['a', { x: 50, y: 500 }]]));
    expect(p.get('a')?.side).toBe('bottom');
    expect(p.has('b')).toBe(false);
  });
});

describe('sideFacing', () => {
  it.each([
    [{ x: 500, y: 30 }, 'right'],
    [{ x: -500, y: 30 }, 'left'],
    [{ x: 50, y: -500 }, 'top'],
    [{ x: 50, y: 500 }, 'bottom'],
  ])('%j is %s', (p, side) => {
    expect(sideFacing({ x: 0, y: 0, w: 100, h: 60 }, p)).toBe(side);
  });
});

describe('placeLabel', () => {
  const route = [{ x: 0, y: 0 }, { x: 400, y: 0 }];
  it('centers on the middle when it is clear', () => {
    expect(placeLabel(route, { w: 40, h: 20 }, undefined, [], [])).toEqual({ x: 180, y: -10, w: 40, h: 20 });
  });
  it('moves along the route away from a box', () => {
    const b = placeLabel(route, { w: 40, h: 20 }, undefined, [{ x: 150, y: -50, w: 100, h: 100 }], []);
    expect(b.x).toBe(100);
  });
  it('keeps a pinned position', () => {
    expect(placeLabel(route, { w: 40, h: 20 }, 0.25, [{ x: 0, y: -50, w: 400, h: 100 }], []).x).toBe(80);
  });
});

describe('segmentHitsBox', () => {
  const r = { x: 0, y: 0, w: 10, h: 10 };
  it.each([
    [{ x: -5, y: 5 }, { x: 15, y: 5 }, true],
    [{ x: -5, y: 15 }, { x: 15, y: 15 }, false],
    [{ x: 5, y: -5 }, { x: 5, y: -1 }, false],
  ])('%j to %j', (a, b, hit) => {
    expect(segmentHitsBox(a, b, r)).toBe(hit);
  });
});

describe('rowSide', () => {
  const b = { x: 0, y: 0, w: 100, h: 60 };
  it.each([
    ['right of it', { x: 300, y: 0, w: 100, h: 40 }, 'right'],
    ['left of it', { x: -300, y: 0, w: 100, h: 40 }, 'left'],
    ['overlapping, closer on the left', { x: 40, y: 200, w: 160, h: 40 }, 'left'],
    ['overlapping, closer on the right', { x: -60, y: 200, w: 150, h: 40 }, 'right'],
    ['a port point inside, nearer the left', { x: 30, y: 200, w: 0, h: 0 }, 'left'],
    ['the same box', b, 'right'],
  ])('%s', (_, o, side) => {
    expect(rowSide(b, o)).toBe(side);
  });
});

describe('placeEndLabel', () => {
  const route = [{ x: 0, y: 100 }, { x: 0, y: 0 }];
  it('goes beside the end, outside the node', () => {
    expect(placeEndLabel(route, { w: 20, h: 17 }, [], [])).toEqual({ x: -28, y: 4, w: 20, h: 17 });
  });
  it('keeps away from another route, moving further out if it must', () => {
    const others = [[{ x: -20, y: 0 }, { x: -20, y: 100 }], [{ x: 30, y: 0 }, { x: 30, y: 24 }]];
    expect(placeEndLabel(route, { w: 20, h: 17 }, [], others)).toEqual({ x: 8, y: 46, w: 20, h: 17 });
  });
});

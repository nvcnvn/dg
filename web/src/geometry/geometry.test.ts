import { describe, expect, it } from 'vitest';
import type { ResolvedView, ViewNode } from '../protocol/view';
import { CONTENT_ORIGIN, MIN_H, MIN_W, fitContainer, layoutBoxes, leafSize, type TextMeasure } from './geometry';

const text: TextMeasure = { width: (s) => s.length * 8, ascent: 14, descent: 4 };
const style = { shape: 'rect', fill: '#FFF', stroke: '#000' } as const;
const node = (id: string, frame?: string, container?: boolean): ViewNode => ({ id, kind: 'k', label: id, frame, container, style });

describe('leafSize', () => {
  it.each([
    ['', { w: MIN_W, h: MIN_H }],
    ['a label that is quite long', { w: 230, h: MIN_H }],
  ])('sizes %j', (label, want) => {
    expect(leafSize(label, text, 10)).toEqual(want);
  });
});

describe('fitContainer', () => {
  it('reaches the furthest child plus padding, on the grid', () => {
    expect(fitContainer({ w: 100, h: 80 }, [{ x: 0, y: 0, w: 100, h: 40 }, { x: 150, y: 75, w: 100, h: 40 }], 10)).toEqual({
      w: 20 + 250 + 20,
      h: 40 + 115 + 20 + 5,
    });
  });
  it('keeps the minimum size when empty', () => {
    expect(fitContainer({ w: 120, h: 80 }, [], 10)).toEqual({ w: 120, h: 80 });
  });
});

describe('layoutBoxes', () => {
  it('places children relative to their frame content origin', () => {
    const view: ResolvedView = { view: 'main', grid: 10, nodes: [node('a', 'g'), node('g', undefined, true), node('h', 'g', true), node('i', 'h')], edges: [] };
    const leaf = { w: 100, h: 40 };
    const rel = new Map([
      ['g', { x: 30, y: 40 }],
      ['a', { x: 0, y: 0 }],
      ['h', { x: 0, y: 60 }],
      ['i', { x: -10, y: 0 }],
    ]);
    const boxes = layoutBoxes(view, new Map([['a', leaf], ['i', leaf]]), new Map(), rel, 10);
    const g = boxes.get('g')!;
    const h = boxes.get('h')!;
    expect(boxes.get('a')).toEqual({ x: 30 + CONTENT_ORIGIN.x, y: 40 + CONTENT_ORIGIN.y, ...leaf });
    expect(boxes.get('i')).toEqual({ x: h.x + CONTENT_ORIGIN.x, y: h.y + CONTENT_ORIGIN.y, ...leaf });
    expect(h.w).toBe(CONTENT_ORIGIN.x + 100 + 20);
    expect(g.h).toBe(CONTENT_ORIGIN.y + 60 + h.h + 20);
  });
});

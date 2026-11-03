import { describe, expect, it } from 'vitest';
import type { ViewDecoration } from '../protocol/view';
import { pathBounds, placeDecorations } from './decoration';
import type { NodeMeasure } from './geometry';
import type { TextMeasure } from './text';

const text: TextMeasure = { width: (s, f) => s.length * f.size * 0.5, ascent: 0.8, descent: 0.2 };

const deco = (d: Partial<ViewDecoration>): ViewDecoration => ({
  id: 'd',
  type: 'rect',
  x: 0,
  y: 0,
  z: 1,
  fill: 'none',
  stroke: '#000000',
  stroke_width: 1,
  stroke_dash: [],
  opacity: 1,
  text_color: '#000000',
  font_size: 10,
  ...d,
});

describe('decorations', () => {
  const boxes = new Map([['n', { x: 100, y: 100, w: 40, h: 20 }], ['g', { x: 0, y: 200, w: 300, h: 300 }]]);
  const measures = new Map([['g', { origin: { x: 10, y: 30 } } as NodeMeasure]]);
  it('places by the canvas, a frame, or the center of an attached node', () => {
    const [canvas, framed, attached, arrow] = placeDecorations(
      [
        deco({ x: 5, y: 6, w: 10, h: 10 }),
        deco({ in: 'g', x: 5, y: 6, w: 10, h: 10 }),
        deco({ type: 'text', attach: 'n', x: 0, y: -20, text: 'abcd' }),
        deco({ type: 'arrow', attach: 'n', x: 0, y: 0, points: [[0, 0], [30, -10]] }),
      ],
      boxes,
      measures,
      text,
    );
    expect(canvas.box).toEqual({ x: 5, y: 6, w: 10, h: 10 });
    expect(framed.box).toEqual({ x: 15, y: 236, w: 10, h: 10 });
    expect(attached.box).toEqual({ x: 110, y: 83, w: 20, h: 14 });
    expect(arrow.points).toEqual([{ x: 120, y: 110 }, { x: 150, y: 100 }]);
  });

  it('bounds path data with relative commands and arcs', () => {
    expect(pathBounds('M10 10 l20 0 v10 h-5 Z')).toEqual({ x: 10, y: 10, w: 20, h: 10 });
    expect(pathBounds('M0 0 C 0 -10 10 -10 10 0')).toEqual({ x: 0, y: -10, w: 10, h: 10 });
    expect(pathBounds('M0 0 A5 5 0 0 1 10 0')).toEqual({ x: -5, y: -5, w: 20, h: 10 });
  });
});

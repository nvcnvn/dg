import { describe, expect, it } from 'vitest';
import { node, text } from '../fixtures';
import type { ResolvedView } from '../protocol/view';
import { absoluteBoxes, containerSize, contentLayout, fitContainers, measureNode } from './geometry';
import { innerBox, outerSize, svgSize } from './shapes';
import { lineHeight, textBlock } from './text';

const font = { size: 14, weight: 400 };

describe('textBlock', () => {
  it.each([
    ['one line', 0, ['one line']],
    ['a b c', 24, ['a b', 'c']],
    ['first\nsecond', 0, ['first', 'second']],
    ['abcdefgh', 32, ['abcd', 'efgh']],
    ['', 100, ['']],
    ['🇻🇳🇻🇳', 32, ['🇻🇳', '🇻🇳']],
    ['👩‍💻x', 44, ['👩‍💻', 'x']],
  ])('wraps %j at %d', (s, max, lines) => {
    expect(textBlock(s, max, font, text).lines).toEqual(lines);
  });
  it('is as tall as its lines', () => {
    expect(textBlock('a\nb', 0, font, text).h).toBe(2 * lineHeight(14));
    expect(textBlock('', 0, font, text).h).toBe(0);
  });
});

describe('measureNode', () => {
  it('keeps the minimum size for a short label', () => {
    expect(measureNode(node('a'), text, 10).size).toEqual({ w: 100, h: 40 });
  });
  it('wraps the label at max_w', () => {
    const m = measureNode(node('n', { label: 'word '.repeat(20).trim() }), text, 10);
    expect(m.text!.lines.length).toBeGreaterThan(1);
    expect(m.size.w).toBeLessThanOrEqual(240);
  });
  it('makes room for an icon above the label', () => {
    const plain = measureNode(node('a'), text, 10);
    const icon = measureNode(node('a', {}, { icon: '<svg/>', min_h: 0 }), text, 10);
    expect(icon.size.h).toBeGreaterThan(plain.size.h);
  });
  it('grows the box by the shape around the content', () => {
    const rect = measureNode(node('a', { label: 'a long label here' }), text, 10).size;
    const ellipse = measureNode(node('a', { label: 'a long label here' }, { shape: 'ellipse' }), text, 10).size;
    expect(ellipse.w).toBeGreaterThan(rect.w);
  });
  it('draws no label on a point', () => {
    expect(measureNode(node('a', {}, { shape: 'point', min_w: 20, min_h: 20 }), text, 10)).toMatchObject({ text: null, size: { w: 20, h: 20 } });
  });
  it('takes a pinned size', () => {
    expect(measureNode(node('a', { pin: { x: 0, y: 0, w: 333 } }), text, 10).size).toEqual({ w: 333, h: 40 });
  });
  it('keeps the aspect ratio of a uniform custom shape', () => {
    const custom = { svg: '<svg viewBox="0 0 200 100"/>', text_box: [0, 0, 1, 1] as [number, number, number, number], stretch: 'uniform' as const };
    expect(measureNode(node('a', {}, { min_w: 0, min_h: 60 }), text, 10, custom).size).toEqual({ w: 120, h: 60 });
    expect(measureNode(node('a', {}, { min_w: 0, min_h: 65 }), text, 10, custom).size).toEqual({ w: 130, h: 65 });
    expect(measureNode(node('a', {}, { min_w: 0, min_h: 0, font_weight: 600 }), text, 10).text!.font.weight).toBe(700);
  });
  it('puts a container content origin under its label', () => {
    const m = measureNode(node('g', { container: true }, { padding: 20 }), text, 10);
    expect(m.origin).toEqual({ x: 20, y: 10 + lineHeight(14) + 20 });
  });
});

describe('containerSize', () => {
  const g = node('g', { container: true }, { padding: 20 });
  const m = measureNode(g, text, 10);
  it('reaches the furthest child plus padding, on the grid', () => {
    const { size, tooSmall } = containerSize(g, m, [{ x: 0, y: 0, w: 100, h: 40 }, { x: 150, y: 75, w: 100, h: 40 }], 10);
    expect(size).toEqual({ w: 20 + 250 + 20, h: 50 + 115 + 20 + 5 });
    expect(tooSmall).toBe(false);
  });
  it('reports a pinned size too small for the children', () => {
    const pinned = { ...g, pin: { x: 0, y: 0, w: 100, h: 100 } };
    expect(containerSize(pinned, m, [{ x: 0, y: 0, w: 300, h: 40 }], 10)).toEqual({ size: { w: 100, h: 100 }, tooSmall: true });
  });
});

describe('absoluteBoxes', () => {
  it('places children relative to their frame content origin, clamping negatives', () => {
    const view: ResolvedView = {
      view: 'main',
      grid: 10,
      nodes: [node('a', { frame: 'g' }), node('g', { container: true }, { padding: 20 }), node('h', { frame: 'g', container: true }, { padding: 20 }), node('i', { frame: 'h' })],
      edges: [],
    };
    const measures = new Map(view.nodes.map((n) => [n.id, measureNode(n, text, 10)]));
    const rel = new Map([
      ['g', { x: 30, y: 40 }],
      ['a', { x: 0, y: 0 }],
      ['h', { x: 0, y: 60 }],
      ['i', { x: -10, y: 0 }],
    ]);
    const layout = fitContainers(view, measures, rel, 10);
    const boxes = absoluteBoxes(view, measures, layout);
    const o = measures.get('g')!.origin;
    const h = boxes.get('h')!;
    expect(boxes.get('a')).toEqual({ x: 30 + o.x, y: 40 + o.y, w: 100, h: 40 });
    expect(boxes.get('i')).toEqual({ x: h.x + o.x, y: h.y + o.y, w: 100, h: 40 });
  });
});

describe('contentLayout', () => {
  it('centers a leaf label and puts a container label at its top-left', () => {
    const leaf = node('a');
    const lc = contentLayout(leaf, measureNode(leaf, text, 10), { x: 0, y: 0, w: 100, h: 40 });
    expect(lc).toMatchObject({ align: 'center', textBox: { x: 10, w: 80, h: lineHeight(14) } });
    const g = node('g', { container: true }, { padding: 20 });
    expect(contentLayout(g, measureNode(g, text, 10), { x: 0, y: 0, w: 300, h: 200 })).toMatchObject({ align: 'left', textBox: { x: 20, y: 10 } });
  });
});

describe('shapes', () => {
  it.each(['rect', 'rounded', 'stadium', 'ellipse', 'diamond', 'cylinder', 'hexagon', 'parallelogram', 'document', 'person', 'cloud', 'text', 'image', 'table'])(
    '%s holds its content',
    (shape) => {
      const c = { w: 120, h: 40 };
      const o = outerSize(shape, c);
      const inner = innerBox(shape, { x: 0, y: 0, ...o });
      expect(inner.w).toBeGreaterThanOrEqual(c.w - 1e-9);
      expect(inner.h).toBeGreaterThanOrEqual(c.h - 1e-9);
    },
  );
  it.each([
    ['<svg viewBox="0 0 64 32" width="10" height="10"/>', { w: 64, h: 32 }],
    ['<svg width="20" height="30"/>', { w: 20, h: 30 }],
    ['<svg/>', { w: 100, h: 100 }],
  ])('svgSize %s', (svg, want) => {
    expect(svgSize(svg)).toEqual(want);
  });
});

describe('tables', () => {
  const t = node('orders', { compartments: [{ name: '', rows: [{ name: 'id', type: 'uuid', key: 'PK' }, { name: 'total', type: 'money' }] }] }, { shape: 'table', padding: 8 });
  it('sizes a table to its header and its key, name and type columns', () => {
    const m = measureNode(t, text, 10);
    // header 8 + 20 + 8; one compartment of 2 rows of 24 between 4 and 4.
    expect(m.table!.lines).toEqual([36]);
    expect(m.table!.rows.map((r) => r.y)).toEqual([52, 76]);
    // key "PK" 16 + gap 14, name "total" 40, gap 14, type "money" 40, padding 8 on both sides.
    expect(m.table!.nameX).toBe(38);
    expect(m.size).toEqual({ w: 140, h: 100 });
  });
  it('draws a class compartment without rows as an empty band', () => {
    const c = node('c', { compartments: [{ name: 'fields', rows: [{ name: 'x' }] }, { name: 'methods', rows: [] }] }, { shape: 'table', padding: 8 });
    const m = measureNode(c, text, 10).table!;
    expect(m.lines).toEqual([36, 68]);
    expect(m.size.h).toBe(76);
  });
  it('draws a table without rows as a rectangle', () => {
    expect(measureNode(node('t', {}, { shape: 'table' }), text, 10).table).toBeUndefined();
  });
});

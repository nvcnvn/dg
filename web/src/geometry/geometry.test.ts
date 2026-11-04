import { describe, expect, it } from 'vitest';
import { node, text } from '../fixtures';
import type { ResolvedView } from '../protocol/view';
import { absoluteBoxes, BADGE, containerSize, contentLayout, drillBadge, GAP, measureNode, fitZoom, MIN_ZOOM, overflows, union } from './geometry';
import { innerBox, outerSize, svgSize } from './shapes';
import { labelBlock, lineHeight, textBlock } from './text';

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

describe('labelBlock', () => {
  const sub = { font: { size: 10, weight: 700 }, color: '#666666' };
  it('is a text block without a sub style or a line break', () => {
    expect(labelBlock('a\nb', 0, font, undefined, text)).toEqual(textBlock('a\nb', 0, font, text));
    expect(labelBlock('a b', 0, font, sub, text)).toEqual(textBlock('a b', 0, font, text));
  });
  it('wraps and sizes what follows the first line break in the sub style', () => {
    const t = labelBlock('title\nsub title\nmore', 48, font, sub, text);
    expect(t.lines[0]).toBe('title');
    expect(t.sub).toEqual({ ...sub, from: 1 });
    expect(t.lines.slice(1)).toEqual(textBlock('sub title\nmore', 48, sub.font, text).lines);
    expect(t.h).toBe(lineHeight(14) + (t.lines.length - 1) * lineHeight(10));
  });
  it('sizes a node by it', () => {
    const style = { min_w: 0, min_h: 0, padding: 0, sub_font_size: 10, sub_text_color: '#666666' };
    const m = measureNode(node('a', { label: 'a\nb' }, style), text, 1);
    expect(m.text).toMatchObject({ h: lineHeight(14) + lineHeight(10), sub: { from: 1, color: '#666666', font: { size: 10, weight: 400 } } });
  });
});

describe('collapsed containers and drill-down badges', () => {
  it('sizes a collapsed container as a leaf with its badge under the label', () => {
    const n = node('c', { collapsed: true, label: 'x'.repeat(20) }, { min_h: 0, min_w: 0 });
    const m = measureNode(n, text, 10);
    expect(m.size).toEqual({ w: 180, h: 60 });
    const c = contentLayout(n, m, { x: 0, y: 0, ...m.size });
    expect(c.badge).toEqual({ x: 84, y: c.textBox.y + lineHeight(14) + GAP, w: BADGE, h: BADGE });
    expect(c.badge!.y + BADGE).toBeLessThanOrEqual(m.size.h - 10);
  });
  it('puts the drill-down badge in the top right corner of the content area, or of a container', () => {
    expect(drillBadge(node('n'), { x: 0, y: 0, w: 100, h: 40 })).toEqual({ x: 85, y: 3, w: BADGE, h: BADGE });
    expect(drillBadge(node('n', {}, { shape: 'cylinder' }), { x: 0, y: 0, w: 100, h: 60 }).y).toBe(17);
    expect(drillBadge(node('g', { container: true }), { x: 10, y: 10, w: 100, h: 60 })).toEqual({ x: 95, y: 13, w: BADGE, h: BADGE });
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
  it.each([
    ['past max_w', { w: 600 }, { max_w: 240 }, 584],
    ['short of max_w', { w: 100 }, { max_w: 240 }, 84],
    ['with no max_w', { w: 100 }, { max_w: 0 }, 84],
    ['in the text area of its shape', { w: 200 }, { max_w: 0, shape: 'diamond' as const }, 84],
  ])('wraps at a pinned width %s', (_, pin, style, width) => {
    const label = 'a long label that wraps on a line of any width up to about here and beyond';
    const t = measureNode(node('a', { label, pin: { x: 0, y: 0, ...pin } }, { padding: 8, ...style }), text, 10).text!;
    expect(t).toEqual(textBlock(label, width, font, text));
  });
  it.each([
    ['leaf width', {}, { w: 100 }, 'w', {}, 220],
    ['leaf width along the stack', {}, { w: 100, h: 60 }, 'h', {}, 80],
    ['container width', { container: true }, { w: 100 }, 'w', {}, 220],
    ['left label height', { container: true }, { h: 100 }, 'h', { label_side: 'left' }, 220],
    ['left label height along the stack', { container: true }, { h: 100 }, 'w', { label_side: 'left' }, 80],
  ] as const)('wraps by a pin only off the stretched axis: %s', (_, over, pin, axis, style, width) => {
    const label = 'a long label that wraps on a line of any width up to about here and beyond';
    const n = node('a', { label, pin: { x: 0, y: 0, ...pin }, ...over }, style);
    const m = measureNode(n, text, 10, undefined, axis);
    expect(m.text).toEqual(textBlock(label, width, font, text));
  });
  it('keeps a pin on the stretched axis as the measured size', () => {
    expect(measureNode(node('a', { pin: { x: 0, y: 0, w: 120 } }), text, 10, undefined, 'w').size.w).toBe(120);
  });
  it('wraps a container label at its pinned size', () => {
    const label = 'a label that needs more than one line here';
    const pin = { x: 0, y: 0, w: 100, h: 200 };
    const lines = (style: object) => measureNode(node('g', { container: true, label, pin }, { padding: 10, max_w: 0, ...style }), text, 10).text!;
    expect(lines({})).toEqual(textBlock(label, 80, font, text));
    expect(lines({ label_side: 'left' })).toEqual(textBlock(label, 180, font, text));
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

describe('containerSize of a pinned container', () => {
  const g = node('g', { container: true, label: '' }, { padding: 20, min_w: 100, min_h: 100 });
  const m = measureNode(g, text, 10);
  const at = (w: number, h: number) => ({ ...g, pin: { x: 0, y: 0, w, h } });
  it.each([
    ['empty, under min_w', at(40, 300), [], false],
    ['holding its child', at(140, 80), [{ x: 0, y: 0, w: 100, h: 40 }], false],
    ['too narrow for its child', at(130, 80), [{ x: 0, y: 0, w: 100, h: 40 }], true],
  ])('%s', (_, pinned, children, tooSmall) => {
    expect(containerSize(pinned, m, children, 10).tooSmall).toBe(tooSmall);
  });
});

describe('overflows', () => {
  const label = 'a label long enough to need several lines at a narrow width';
  const at = (b: { w: number; h: number }, pin?: object, style = {}) => {
    const n = node('a', { label, ...(pin ? { pin: { x: 0, y: 0, ...pin } } : {}) }, style);
    return overflows(n, measureNode(n, text, 10), { x: 0, y: 0, ...b });
  };
  it('is false for a node without a pinned size', () => {
    expect(at({ w: 10, h: 10 })).toBe(false);
  });
  it('is false when the wrapped label fits', () => {
    expect(at({ w: 200, h: 200 }, { w: 200, h: 200 })).toBe(false);
  });
  it('is true when the wrapped label is taller than the box', () => {
    expect(at({ w: 100, h: 20 }, { w: 100, h: 20 })).toBe(true);
  });
  it('is false for a pin the box does not have', () => {
    expect(at({ w: 240, h: 20 }, { w: 100 })).toBe(false);
    expect(at({ w: 100, h: 300 }, { w: 90, h: 20 })).toBe(false);
  });
  it('is true when the box has one of two pins and does not hold the label', () => {
    expect(at({ w: 240, h: 20 }, { w: 100, h: 20 })).toBe(true);
  });
  it('is true when a container is shorter than its label', () => {
    const n = node('g', { container: true, label, pin: { x: 0, y: 0, w: 100, h: 20 } });
    expect(overflows(n, measureNode(n, text, 10), { x: 0, y: 0, w: 100, h: 20 })).toBe(true);
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
    const sizes = new Map([...measures].map(([id, m]) => [id, m.size]));
    const layout = { rel, sizes, tooSmall: new Set<string>() };
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

describe('fitZoom', () => {
  it.each([
    ['small: 100%', { w: 100, h: 100 }, { w: 800, h: 600 }, 1],
    ['too wide', { w: 1600, h: 100 }, { w: 800, h: 600 }, 0.5],
    ['too tall', { w: 100, h: 1200 }, { w: 800, h: 600 }, 0.5],
    ['nothing drawn', { w: 0, h: 0 }, { w: 800, h: 600 }, 1],
    ['larger than the lowest zoom shows', { w: 100000, h: 100 }, { w: 800, h: 600 }, MIN_ZOOM],
  ])('%s', (_, box, room, want) => {
    expect(fitZoom(box, room)).toBe(want);
  });
});

describe('union', () => {
  it('is the box around boxes', () => {
    expect(union([{ x: 10, y: 20, w: 5, h: 5 }, { x: -10, y: 30, w: 1, h: 1 }])).toEqual({ x: -10, y: 20, w: 25, h: 11 });
  });
});

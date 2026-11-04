import ELK from 'elkjs/lib/elk.bundled.js';
import { describe, expect, it } from 'vitest';
import { avoidRouter } from './canvas/avoid';
import { edge, libavoidWasm, node, text } from './fixtures';
import { textBlock } from './geometry/text';
import type { CustomShape, Policy, ResolvedView, ViewEdge, ViewNode } from './protocol/view';
import { draw } from './render';

const noElk = { layout: () => Promise.reject(new Error('elk is not used when a node is pinned')) };

// bigView is n nodes in n/20 containers, one pinned so the spiral placer runs, and 2n edges
// between random nodes from a fixed seed.
function bigView(n: number): ResolvedView {
  let seed = 1;
  const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
  const groups = Math.max(1, Math.floor(n / 20));
  const id = (i: number) => 'n' + String(i).padStart(3, '0');
  const nodes: ViewNode[] = [];
  for (let g = 0; g < groups; g++) nodes.push(node('g' + g, { container: true, pin: g === 0 ? { x: 0, y: 0 } : undefined }, { padding: 20 }));
  for (let i = 0; i < n; i++) nodes.push(node(id(i), { frame: 'g' + (i % groups) }));
  const edges: ViewEdge[] = [];
  for (let j = 0; j < 2 * n; j++) {
    const a = Math.floor(rnd() * n);
    const b = (Math.floor(rnd() * n) + (a === 0 ? 1 : 0)) % n;
    edges.push(edge('e' + String(j).padStart(3, '0'), id(a), id(b === a ? (b + 1) % n : b)));
  }
  return { view: 'big', grid: 10, nodes, edges };
}

// The drag budget of FR-8 is M10's; this bounds a whole draw so routing cannot blow up again.
// Nearly all of it is libavoid's own routing of this dense graph: about 3.3 seconds on an M-series
// Mac, where a pin or nudging option that makes libavoid slow costs minutes. The bound leaves room
// for shared CI runners, which take from 4 to 10 seconds on the same code.
it('draws 200 nodes and 400 edges within 20 seconds', async () => {
  const view = bigView(200);
  const start = performance.now();
  const d = await draw(view, text, 'Inter', noElk, avoidRouter(libavoidWasm));
  const ms = performance.now() - start;
  console.log(`draw 200/400: ${Math.round(ms)} ms`);
  expect(d.edges).toHaveLength(400);
  expect(ms).toBeLessThan(20000);
}, 120000);

const drawn = (nodes: ViewNode[], over: Partial<ResolvedView> = {}, grid = 10) => draw({ view: 'v', grid, nodes, edges: [], ...over }, text, 'Inter', new ELK(), avoidRouter(libavoidWasm));

describe('a pinned size a stretched stack overrides', () => {
  const label = 'A rather long label that has to wrap across several lines when pinned';
  const font = { size: 14, weight: 400 };
  const stack = (over: Partial<Policy>): Policy => ({ layout: 'stack', direction: 'vertical', gap: 10, columns: 2, align: 'start', order: ['a', 'wide'], ...over });
  const horizontal = stack({ direction: 'horizontal', stretch: true });
  const vertical = stack({ stretch: true });
  const inStack = (policy: Policy | undefined, a: Partial<ViewNode>, style = {}) => [
    node('s', { container: true, policy, pin: { x: 0, y: 0 } }),
    node('a', { frame: 's', label, ...a }, style),
    node('wide', { frame: 's', label: 'x'.repeat(100) }),
  ];
  const left = { label_side: 'left' as const };
  it.each<[string, Policy | undefined, Partial<ViewNode>, object, number]>([
    ['a leaf across a vertical stack', vertical, { pin: { x: 0, y: 0, w: 100 } }, {}, 220],
    ['a container across a vertical stack', vertical, { container: true, pin: { x: 0, y: 0, w: 100 } }, {}, 220],
    ['a left label across a horizontal stack', horizontal, { container: true, pin: { x: 0, y: 0, h: 100 } }, left, 80],
    ['the height of a leaf across a horizontal stack, in its text area', horizontal, { pin: { x: 0, y: 0, w: 200, h: 100 } }, { shape: 'hexagon' }, 130],
    ['a leaf along a horizontal stack', horizontal, { pin: { x: 0, y: 0, w: 100 } }, {}, 80],
    ['a container along a horizontal stack', horizontal, { container: true, pin: { x: 0, y: 0, w: 100 } }, {}, 80],
    ['a left label along a vertical stack', vertical, { container: true, pin: { x: 0, y: 0, h: 100 } }, left, 80],
    ['a leaf in a stack that does not stretch', stack({}), { pin: { x: 0, y: 0, w: 100 } }, {}, 80],
    ['a leaf in a grid', stack({ layout: 'grid', stretch: true }), { pin: { x: 0, y: 0, w: 100 } }, {}, 80],
    ['a leaf in a free container', undefined, { pin: { x: 0, y: 0, w: 100 } }, {}, 80],
  ])('wraps %s', async (_, policy, a, style, width) => {
    const d = await drawn(inStack(policy, a, style));
    expect(d.nodes.find((n) => n.id === 'a')!.text).toEqual(textBlock(label, width, font, text));
  });
  const child = (id: string, a: Partial<ViewNode>) => node(id, { frame: 's', label, ...a }, { padding: 10 });
  const lanes = (s: Partial<ViewNode>, ...kids: ViewNode[]) => drawn([node('s', { container: true, policy: { ...vertical, order: kids.map((k) => k.id) }, pin: { x: 0, y: 0 }, ...s }, { padding: 10, min_w: 0 }), ...kids]);
  const width = (w: number) => ({ pin: { x: 0, y: 0, w } });
  it.each<[string, Partial<ViewNode>, number, number]>([
    ['narrower than the stack', width(100), 130, 110],
    ['wider than the stack', width(300), 130, 110],
  ])('wraps a child pinned %s at the inside of a pinned stack', async (_, a, w, wrap) => {
    const d = await lanes(width(150), child('a', a));
    const n = d.nodes.find((x) => x.id === 'a')!;
    expect(n.box.w).toBe(w);
    expect(n.text).toEqual(textBlock(label, wrap, font, text));
    expect(n.overflows).toBe(false);
  });
  it('wraps a child without a pin at the inside of a pinned stack', async () => {
    const n = (await lanes(width(150), child('a', {}))).nodes.find((x) => x.id === 'a')!;
    expect(n.box.w).toBe(130);
    expect(n.text).toEqual(textBlock(label, 110, font, text));
  });
  it('wraps children all pinned to one width at that width', async () => {
    const d = await lanes({}, child('a', width(100)), child('b', width(100)));
    for (const n of d.nodes.filter((x) => x.id !== 's')) {
      expect(n.box.w).toBe(100);
      expect(n.text).toEqual(textBlock(label, 80, font, text));
      expect(n.overflows).toBe(false);
    }
  });
  it('wraps lanes pinned to one width with no max_w at that width', async () => {
    const lane = (id: string) => node(id, { frame: 's', label: 'x '.repeat(100).trim(), ...width(200) }, { padding: 10, max_w: 0 });
    const d = await lanes({}, lane('a'), lane('b'));
    const a = d.nodes.find((x) => x.id === 'a')!;
    expect(a.box.w).toBe(200);
    expect(a.text!.w).toBeLessThanOrEqual(180);
  });
  it('keeps wrapping children without a pin in a stack without one at max_w', async () => {
    const d = await lanes({}, child('a', {}), child('b', width(400)));
    expect(d.nodes.find((x) => x.id === 'a')!.text).toEqual(textBlock(label, 220, font, text));
    expect(d.nodes.find((x) => x.id === 'b')!.text).toEqual(textBlock(label, 380, font, text));
  });
  it('wraps a child of a lane at the room left by the label strip of a wider lane', async () => {
    const lane = { label_side: 'left' as const, padding: 10, min_w: 0 };
    const d = await drawn([
      node('s', { container: true, policy: { ...vertical, order: ['wide', 't'] }, pin: { x: 0, y: 0, w: 300 } }, { padding: 0, min_w: 0 }),
      node('wide', { frame: 's', container: true, label: 'two\nlines' }, lane),
      node('t', { frame: 's', container: true, label: 'x', policy: { ...vertical, order: ['a'] } }, lane),
      child('a', { frame: 't' }),
    ]);
    const a = d.nodes.find((n) => n.id === 'a')!;
    expect(a.text).toEqual(textBlock(label, a.box.w - 20, font, text));
  });
  it('wraps a child in the stack of the of of a view the same way', async () => {
    const nodes = [node('a', { label, pin: { x: 0, y: 0, w: 100 } }), node('wide', { label: 'x'.repeat(100) })];
    const d = await drawn(nodes, { policy: { ...vertical, container: 's' } });
    const a = d.nodes.find((n) => n.id === 'a')!;
    expect(a.text).toEqual(textBlock(label, 220, font, text));
    expect(a.overflows).toBe(false);
  });
  it('draws the child as tall as its label wrapped that way', async () => {
    const d = await drawn(inStack(vertical, { pin: { x: 0, y: 0, w: 100 } }));
    expect(d.nodes.find((n) => n.id === 'a')!.box).toMatchObject({ w: 240, h: 80 });
  });
  it.each<[string, Policy, Partial<ViewNode>, object, boolean]>([
    ['a width the stack overrides', vertical, { pin: { x: 0, y: 0, w: 100 } }, {}, false],
    ['a container width the stack overrides', vertical, { container: true, label: 'word '.repeat(9), pin: { x: 0, y: 0, w: 100 } }, {}, false],
    ['a left label height the stack overrides', horizontal, { container: true, label: 'word '.repeat(9), pin: { x: 0, y: 0, h: 60 } }, left, false],
    ['a height along the stack too short for the label', vertical, { pin: { x: 0, y: 0, w: 100, h: 20 } }, {}, true],
    ['a width along the stack too narrow for the label', horizontal, { pin: { x: 0, y: 0, w: 10, h: 300 } }, {}, true],
  ])('warns only when the label overflows: %s', async (_, policy, a, style, want) => {
    const d = await drawn(inStack(policy, a, style));
    expect(d.nodes.find((n) => n.id === 'a')!.overflows).toBe(want);
  });
  it('warns about a label that overflows where the width is known only after placement', async () => {
    const free = node('g', { frame: 's', container: true, label: '' }, { padding: 0, min_w: 0 });
    const d = await drawn([node('s', { container: true, policy: { ...vertical, order: ['a', 'g'] }, pin: { x: 0, y: 0 } }, { min_w: 0, padding: 0 }), node('a', { frame: 's', label, pin: { x: 0, y: 0, w: 100 } }), free, node('i', { frame: 'g', label: 'x', pin: { x: 0, y: 0, w: 100 } })]);
    const a = d.nodes.find((n) => n.id === 'a')!;
    expect(a.box.w).toBe(100);
    expect(a.text).toEqual(textBlock(label, 220, font, text));
    expect(a.overflows).toBe(true);
  });
});

describe('bounds', () => {
  const pin = (w: number, h: number) => ({ pin: { x: 100, y: 100, w, h } });
  const rows = [
    { name: 'id', type: 'uuid', key: 'PK' },
    { name: 'total', type: 'money' },
  ];
  const tall = Array.from({ length: 8 }, (_, i) => ({ name: 'row' + i }));
  const table = { shape: 'table', padding: 8 };
  const none: CustomShape = { svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 60"/>', text_box: [0, 0, 1, 1], stretch: 'none' };
  const icon = { icon: '<svg/>', padding: 0, min_w: 0, min_h: 0 };
  const lane = { label_side: 'left' as const, padding: 10, min_w: 0, min_h: 0 };

  // Each case gives the left, top, right and bottom of what the node draws, at a grid of 1. A
  // table has no top: nothing in it draws above its box.
  it.each<[string, ViewNode, [number, number, number, number]]>([
    ['a label past all four sides', node('n', { label: 'ab', ...pin(20, 10) }, { font_size: 70, padding: 0 }), [90, 7, 130, 203]],
    ['an icon past all four sides', node('n', { label: '', ...pin(20, 10) }, { ...icon, font_size: 40 }), [70, 65, 150, 145]],
    ['an icon above a label', node('n', { label: 'x', ...pin(20, 10) }, { ...icon, font_size: 70 }), [40, -16, 180, 226]],
    ['rows below the box', node('t', { label: 'orders', compartments: [{ name: '', rows: tall }], ...pin(300, 30) }, table), [100, 100, 400, 330]],
    ['the name and type columns of a table with no label', node('t', { label: '', compartments: [{ name: '', rows }], ...pin(20, 10) }, table), [72, 100, 178, 150]],
    ['a separator line below the last row', node('t', { label: '', compartments: [{ name: '', rows: [{ name: 'x' }] }, { name: 'm', rows: [] }], ...pin(100, 10) }, table), [100, 100, 200, 132]],
    ['a left label past the top, bottom and right', node('g', { container: true, label: 'a', ...pin(20, 20) }, { ...lane, font_size: 70 }), [100, 90, 208, 130]],
    ['a left label with an icon', node('g', { container: true, label: 'a', ...pin(20, 20) }, { ...lane, icon: '<svg/>', font_size: 70 }), [100, 40, 352, 180]],
    ['a left icon with no label', node('g', { container: true, label: '', ...pin(20, 20) }, { ...lane, icon: '<svg/>', font_size: 40 }), [100, 70, 190, 150]],
    ['a collapse badge', node('c', { collapsed: true, label: '', ...pin(20, 10) }, { padding: 0 }), [100, 99, 120, 111]],
    ['a custom picture at its natural size', node('n', { label: '', ...pin(20, 10) }, { shape: 'none' }), [60, 75, 160, 135]],
  ])('hold %s', async (_, n, [left, top, right, bottom]) => {
    const d = await drawn([n], { shapes: { none } }, 1);
    expect(d.bounds).toEqual({ x: left - 20, y: top - 20, w: right - left + 40, h: bottom - top + 40 });
  });

  const at = { pin: { x: 100, y: 100 } };
  it.each<[string, ViewNode]>([
    ['a label', node('n', at)],
    ['an icon and a label', node('n', at, { icon: '<svg/>' })],
    ['a table', node('t', { label: 'orders', compartments: [{ name: '', rows }], ...at }, table)],
    ['a left label and an icon', node('g', { container: true, ...at }, { label_side: 'left', icon: '<svg/>' })],
    ['a collapse badge', node('c', { collapsed: true, ...at })],
    ['a custom picture', node('n', { label: '', ...at }, { shape: 'none' })],
  ])('are the box of a node that holds %s', async (_, n) => {
    const d = await drawn([n], { shapes: { none } }, 1);
    const b = d.nodes[0].box;
    expect(d.bounds).toEqual({ x: b.x - 20, y: b.y - 20, w: b.w + 40, h: b.h + 40 });
  });
});

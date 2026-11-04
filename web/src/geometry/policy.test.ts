import ELK from 'elkjs/lib/elk.bundled.js';
import { describe, expect, it } from 'vitest';
import { edge, node, text } from '../fixtures';
import { elkLayout } from '../place/elk';
import { placeSpiral } from '../place/spiral';
import type { Policy, ResolvedView, ViewNode } from '../protocol/view';
import { contentLayout, measureNode } from './geometry';
import { alignHeaders, arrange, stretchedAxes, stretchedSizes, withBlock } from './policy';

const stack = (over: Partial<Policy> = {}): Policy => ({ layout: 'stack', direction: 'vertical', gap: 10, columns: 3, align: 'start', order: [], ...over });

describe('arrange', () => {
  const sizes = [
    { w: 100, h: 40 },
    { w: 60, h: 20 },
    { w: 80, h: 30 },
  ];
  it('stacks children along its direction, gap apart, aligned across it', () => {
    expect(arrange(stack(), sizes, 100).rel).toEqual([
      { x: 0, y: 0 },
      { x: 0, y: 50 },
      { x: 0, y: 80 },
    ]);
    expect(arrange(stack({ direction: 'horizontal', align: 'end' }), sizes, 40).rel).toEqual([
      { x: 0, y: 0 },
      { x: 110, y: 20 },
      { x: 180, y: 10 },
    ]);
  });
  it('stretches children across the stack, keeping their size along it', () => {
    expect(arrange(stack({ stretch: true }), sizes, 120).sizes).toEqual([
      { w: 120, h: 40 },
      { w: 120, h: 20 },
      { w: 120, h: 30 },
    ]);
  });
  it('fills a grid row by row, columns as wide as their widest child, rows as tall as their tallest', () => {
    const grid = stack({ layout: 'grid', columns: 2, align: 'center' });
    expect(arrange(grid, sizes, 0).rel).toEqual([
      { x: 0, y: 0 },
      { x: 110, y: 10 },
      { x: 10, y: 50 },
    ]);
  });
});

describe('label along the left edge', () => {
  const lane = node('lane', { container: true, label: 'Sales' }, { padding: 20, label_side: 'left' });
  it('puts the content origin past the label and turns the label around the middle of the left strip', () => {
    const m = measureNode(lane, text, 10);
    expect(m.origin).toEqual({ x: 10 + 20 + 20, y: 20 });
    const c = contentLayout(lane, m, { x: 0, y: 0, w: 300, h: 100 });
    expect(c.rotate).toEqual({ x: 20, y: 50 });
    expect(c.divider).toBe(40);
    expect(c.textBox).toMatchObject({ x: 20 - 20, w: 40 });
  });
});

// A lane set of two lanes with steps flowing across them, and a grid of two leaves.
const view: ResolvedView = {
  view: 'v',
  grid: 10,
  nodes: [
    node('set', { container: true, policy: stack({ gap: 0, stretch: true, order: ['b', 'a'] }) }, { padding: 0 }),
    node('a', { container: true, frame: 'set' }, { padding: 20, label_side: 'left' }),
    node('b', { container: true, frame: 'set' }, { padding: 20, label_side: 'left' }),
    node('a1', { frame: 'a' }),
    node('a2', { frame: 'a' }),
    node('b1', { frame: 'b' }),
    node('g', { container: true, policy: stack({ layout: 'grid', columns: 2, order: ['g1', 'g2'] }) }),
    node('g1', { frame: 'g' }),
    node('g2', { frame: 'g' }, { min_w: 140 }),
  ],
  edges: [edge('e1', 'a1', 'b1'), edge('e2', 'b1', 'a2'), edge('e3', 'a2', 'g1')],
};
const measures = new Map(view.nodes.map((n) => [n.id, measureNode(n, text, 10)]));

describe('elkLayout', () => {
  it('stacks lanes in order, stretched to one width, with the steps flowing across them', async () => {
    const l = await elkLayout(new ELK(), view, measures);
    expect(l.rel.get('b')).toEqual({ x: 0, y: 0 });
    expect(l.rel.get('a')).toEqual({ x: 0, y: l.sizes.get('b')!.h });
    expect(l.sizes.get('a')!.w).toBe(l.sizes.get('b')!.w);
    expect(l.sizes.get('set')!.w).toBe(l.sizes.get('a')!.w);
    const x = (id: string) => l.rel.get(id)!.x;
    expect(x('a1')).toBe(0);
    expect(x('a1')).toBeLessThan(x('b1'));
    expect(x('b1')).toBeLessThan(x('a2'));
    expect(l.rel.get('a2')!.y).toBe(0);
    expect(l.rel.get('g1')).toEqual({ x: 0, y: 0 });
    expect(l.rel.get('g2')).toEqual({ x: 110, y: 0 });
  });
  it('is deterministic', async () => {
    const first = await elkLayout(new ELK(), view, measures);
    const second = await elkLayout(new ELK(), view, measures);
    expect([...first.rel]).toEqual([...second.rel]);
    expect([...first.sizes]).toEqual([...second.sizes]);
  });
  it('lays out the children of the of of a view as one box among the other nodes', async () => {
    const inside: ResolvedView = {
      ...view,
      policy: stack({ container: 'set', order: ['b', 'a'] }),
      nodes: [...view.nodes.filter((n) => n.frame === 'set' || n.frame === 'a' || n.frame === 'b').map((n) => (n.frame === 'set' ? { ...n, frame: undefined } : n)), node('other')],
      edges: [...view.edges.slice(0, 2), edge('e4', 'a2', 'other')],
    };
    const placed = withBlock(inside);
    const ms = new Map(placed.nodes.map((n) => [n.id, measureNode(n, text, 10)]));
    const l = await elkLayout(new ELK(), placed, ms);
    expect(l.rel.get('b')).toEqual({ x: 0, y: 0 });
    expect(l.rel.get('a')!.y).toBe(l.sizes.get('b')!.h + 10);
    const block = { ...l.rel.get('set')!, ...l.sizes.get('set')! };
    const other = { ...l.rel.get('other')!, ...l.sizes.get('other')! };
    expect(other.y >= block.y + block.h || other.x >= block.x + block.w).toBe(true);
  });
  it('arranges a stretched inner stack again in its new width', async () => {
    const nested: ResolvedView = {
      view: 'n',
      grid: 10,
      nodes: [
        node('outer', { container: true, policy: stack({ gap: 0, stretch: true, order: ['wide', 'inner'] }) }, { padding: 0 }),
        node('wide', { frame: 'outer' }, { min_w: 300 }),
        node('inner', { container: true, frame: 'outer', policy: stack({ align: 'center', order: ['i1'] }) }, { padding: 0 }),
        node('i1', { frame: 'inner' }),
      ],
      edges: [],
    };
    const ms = new Map(nested.nodes.map((n) => [n.id, measureNode(n, text, 10)]));
    const l = await elkLayout(new ELK(), nested, ms);
    expect(l.sizes.get('inner')!.w).toBe(300);
    expect(l.rel.get('i1')!.x).toBe(100);
  });
});

describe('stretchedAxes', () => {
  const axes = (p: Policy) => stretchedAxes({ view: 'v', grid: 10, edges: [], nodes: [node('s', { container: true, policy: p }), node('a', { frame: 's' })] });
  it.each<[string, Partial<Policy>, [string, { axis: string }][]]>([
    ['the width across a vertical stack', { stretch: true }, [['a', { axis: 'w' }]]],
    ['the height across a horizontal stack', { stretch: true, direction: 'horizontal' }, [['a', { axis: 'h' }]]],
    ['nothing in a stack that does not stretch', {}, []],
    ['nothing in a grid', { stretch: true, layout: 'grid' }, []],
  ])('gives %s', (_, p, want) => {
    expect([...axes(stack({ order: ['a'], ...p }))]).toEqual(want);
  });
  it('sees the children of the of of a view through its block', () => {
    const inside: ResolvedView = { view: 'v', grid: 10, edges: [], nodes: [node('a')], policy: stack({ container: 'of', stretch: true, order: ['a'] }) };
    expect(stretchedAxes(inside)).toEqual(new Map());
    expect([...stretchedAxes(withBlock(inside))]).toEqual([['a', { axis: 'w' }]]);
  });
});

describe('stretchedSizes', () => {
  const pin = (w: number) => ({ pin: { x: 0, y: 0, w } });
  const vertical = (order: string[]) => stack({ stretch: true, order });
  const box = (id: string, frame: string | undefined, order: string[], over = {}) => node(id, { container: true, frame, policy: vertical(order), ...over }, { padding: 10, min_w: 0 });
  const sizes = (nodes: ViewNode[]) => {
    const v: ResolvedView = { view: 'v', grid: 10, edges: [], nodes };
    return Object.fromEntries([...stretchedSizes(v, new Map(nodes.map((n) => [n.id, measureNode(n, text, 10)])), 10)].map(([id, s]) => [id, s.size]));
  };
  const leaf = (id: string, frame: string, over = {}) => node(id, { frame, label: 'x', ...over });
  it.each<[string, ViewNode[], Record<string, number | undefined>]>([
    ['the inside of a pinned stack to every child', [box('s', undefined, ['a', 'b'], pin(150)), leaf('a', 's', pin(300)), leaf('b', 's')], { a: 130, b: 130 }],
    ['the widest pinned child to the pinned children only', [box('s', undefined, ['a', 'b']), leaf('a', 's', pin(100)), leaf('b', 's', pin(300))], { a: 300, b: 300 }],
    ['the widest measured child to a pinned child', [box('s', undefined, ['a', 'b']), leaf('a', 's', pin(50)), node('b', { frame: 's', label: 'y'.repeat(20) })], { a: 180, b: undefined }],
    ['nothing to children without a pin in a stack without one', [box('s', undefined, ['a', 'b']), leaf('a', 's'), leaf('b', 's')], { a: undefined, b: undefined }],
    ['nothing to a pinned child beside a child placement sizes', [box('s', undefined, ['a', 'g']), leaf('a', 's', pin(100)), node('g', { frame: 's', container: true }), leaf('i', 'g')], { a: undefined, g: undefined }],
    ['an outer pin through an inner stack, outermost first', [box('s', undefined, ['t'], pin(200)), box('t', 's', ['a', 'b']), leaf('a', 't', pin(50)), leaf('b', 't')], { t: 180, a: 160, b: 160 }],
    ['the widest of an inner stack, innermost first', [box('s', undefined, ['t', 'c']), box('t', 's', ['a']), leaf('a', 't', pin(300)), leaf('c', 's', pin(100))], { t: undefined, a: 300, c: 320 }],
    ['the widest of an outer stack to a pinned child of an inner one', [box('s', undefined, ['t', 'c']), box('t', 's', ['a']), leaf('a', 't', pin(100)), leaf('c', 's', pin(300))], { t: undefined, a: 280, c: 300 }],
    ['nothing to a child without a pin of an inner stack without one', [box('s', undefined, ['t', 'c']), box('t', 's', ['a', 'b']), leaf('a', 't', pin(100)), leaf('b', 't'), leaf('c', 's', pin(300))], { t: undefined, a: 280, b: undefined, c: 300 }],
    ['nothing inside an inner stack when placement sizes the outer one', [box('s', undefined, ['g', 'k']), node('g', { frame: 's', container: true }), leaf('i', 'g'), box('k', 's', ['c']), leaf('c', 'k', pin(60))], { g: undefined, k: undefined, c: undefined }],
    ['nothing inside a pinned inner stack when placement sizes the outer one', [box('s', undefined, ['g', 'k']), node('g', { frame: 's', container: true }), leaf('i', 'g'), box('k', 's', ['c'], pin(300)), leaf('c', 'k', pin(60))], { g: undefined, k: undefined, c: undefined }],
    ['an outer stack through a pinned inner one, whose children all take it', [box('s', undefined, ['t', 'c']), box('t', 's', ['a'], pin(100)), leaf('a', 't'), leaf('c', 's', pin(300))], { t: 300, a: 280, c: 300 }],
  ])('gives %s', (_, nodes, want) => {
    expect(sizes(nodes)).toEqual(want);
  });
  it('gives the height across a horizontal stack, past a left label', () => {
    const lanes = node('s', { container: true, policy: stack({ stretch: true, direction: 'horizontal', order: ['a'] }), pin: { x: 0, y: 0, h: 200 } }, { padding: 10 });
    expect(sizes([lanes, node('a', { frame: 's', container: true, pin: { x: 0, y: 0, h: 80 } }, { label_side: 'left' })]).a).toBe(200 - measureNode(lanes, text, 10).origin.y - 10);
  });
});

describe('alignHeaders', () => {
  it('gives the lanes of a stretched stack the widest label strip', () => {
    const ms = new Map(view.nodes.map((n) => [n.id, measureNode(n, text, 10)]));
    const lanes: ResolvedView = { ...view, nodes: view.nodes.map((n) => (n.id === 'a' ? { ...n, label: 'one\ntwo\nthree' } : n)) };
    ms.set('a', measureNode(lanes.nodes.find((n) => n.id === 'a')!, text, 10));
    alignHeaders(lanes, ms);
    expect(ms.get('b')!.origin.x).toBe(ms.get('a')!.origin.x);
    expect(ms.get('a')!.origin.x).toBe(10 + 3 * 20 + 20);
  });
});

describe('placeSpiral', () => {
  it('places the children of a stack by its policy, whatever their pins', () => {
    const pinned = { ...view, nodes: view.nodes.map((n) => (n.id === 'set' ? { ...n, pin: { x: 0, y: 0, w: 600 } } : n.id === 'a' ? { ...n, pin: { x: 0, y: 0, h: 200 } } : n)) };
    const ms = new Map(pinned.nodes.map((n) => [n.id, measureNode(n, text, 10)]));
    const l = placeSpiral(pinned, ms, new Map([['set', { x: 0, y: 0 }]]), 10);
    expect(l.rel.get('a')).toEqual({ x: 0, y: l.sizes.get('b')!.h });
    expect(l.sizes.get('a')).toEqual({ w: 600, h: 200 });
    expect(l.sizes.get('b')!.w).toBe(600);
  });
});

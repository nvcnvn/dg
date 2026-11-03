import ELK from 'elkjs/lib/elk.bundled.js';
import { describe, expect, it } from 'vitest';
import { edge, node, text } from '../fixtures';
import { measureNode } from '../geometry/geometry';
import type { EdgeStyle, ResolvedView } from '../protocol/view';
import { backEdges, elkGraph, elkLayout } from './elk';

const view: ResolvedView = {
  view: 'main',
  grid: 10,
  nodes: [node('a', { frame: 'g' }), node('b', { frame: 'g' }), node('c'), node('g', { container: true }, { padding: 20 })],
  edges: [edge('ab', 'a', 'b'), edge('ca', 'c', 'a'), edge('ga', 'g', 'a')],
};
const measures = new Map([...view.nodes, node('t'), node('u')].map((n) => [n.id, measureNode(n, text, 10)]));

describe('elkGraph', () => {
  it('nests children and puts each edge in the innermost common container', () => {
    const g = elkGraph(view, measures);
    expect(g.children!.map((c) => c.id)).toEqual(['n:c', 'n:g']);
    expect(g.edges!.map((e) => e.id)).toEqual(['e:ca']);
    const group = g.children![1];
    expect(group.children!.map((c) => c.id)).toEqual(['n:a', 'n:b']);
    expect(group.edges!.map((e) => e.id)).toEqual(['e:ab']);
  });
});

describe('elkGraph edges', () => {
  const nodes = [node('t'), node('u')];
  const styled = (id: string, over: Partial<EdgeStyle>) => ({ ...edge(id, 't', 'u'), style: { ...edge(id, 't', 'u').style, ...over } });
  it('lays out a view with row edges left to right', () => {
    const rows: ResolvedView = { view: 'v', grid: 10, nodes, edges: [{ ...edge('r', 't', 'u'), from: { node: 't', row: 'id' } }] };
    expect(elkGraph(rows, measures).layoutOptions!['elk.direction']).toBe('RIGHT');
    expect(elkGraph({ ...rows, edges: [edge('r', 't', 'u')] }, measures).layoutOptions!['elk.direction']).toBe('DOWN');
  });
  it('takes the direction the view sets', () => {
    const rows: ResolvedView = { view: 'v', grid: 10, nodes, edges: [{ ...edge('r', 't', 'u'), from: { node: 't', row: 'id' } }], direction: 'up' };
    expect(elkGraph(rows, measures).layoutOptions!['elk.direction']).toBe('UP');
    expect(elkGraph({ ...rows, edges: [], direction: 'left' }, measures).layoutOptions!['elk.direction']).toBe('LEFT');
  });
  it('gives an edge from the end with an open triangle', () => {
    const edges = [styled('a', { arrow_end: 'triangle-open' }), styled('b', { arrow_start: 'triangle-open', arrow_end: 'none' }), styled('c', { arrow_start: 'triangle-open', arrow_end: 'triangle-open' })];
    const ends = edges.map((e) => elkGraph({ view: 'v', grid: 10, nodes, edges: [e] }, measures).edges!.map((g) => [g.sources[0], g.targets[0]])[0]);
    expect(ends).toEqual([['n:u', 'n:t'], ['n:t', 'n:u'], ['n:t', 'n:u']]);
  });
  it('keeps self-loops and places start and end labels at the tail and head', () => {
    const g = elkGraph({ view: 'v', grid: 10, nodes, edges: [edge('l', 't', 't')] }, measures, new Map([['l', { start: { w: 20, h: 17 }, end: { w: 30, h: 17 } }]]));
    expect(g.edges!.map((e) => [e.sources[0], e.targets[0]])).toEqual([['n:t', 'n:t']]);
    expect(g.edges![0].labels!.map((l) => l.layoutOptions!['elk.edgeLabels.placement'])).toEqual(['TAIL', 'HEAD']);
  });
});

describe('backEdges', () => {
  it('reverses the edges that loop back to where a flow from its sources has been', () => {
    const edges = [
      { id: 'a', a: 'start', b: 'check' },
      { id: 'b', a: 'check', b: 'fix' },
      { id: 'c', a: 'fix', b: 'check' },
      { id: 'd', a: 'check', b: 'done' },
    ];
    expect([...backEdges(edges)]).toEqual(['c']);
  });
  it('leaves a cycle no source leads to', () => {
    const edges = [
      { id: 'a', a: 'x', b: 'y' },
      { id: 'b', a: 'y', b: 'x' },
    ];
    expect([...backEdges(edges)]).toEqual([]);
  });
});

describe('elkLayout', () => {
  it('is deterministic and grid-aligned', async () => {
    const elk = new ELK();
    const first = (await elkLayout(elk, view, measures)).rel;
    const second = (await elkLayout(elk, view, measures)).rel;
    expect([...first]).toEqual([...second]);
    for (const p of first.values()) {
      expect(p.x % 10).toBe(0);
      expect(p.y % 10).toBe(0);
    }
  });
});

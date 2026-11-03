import ELK from 'elkjs/lib/elk.bundled.js';
import { describe, expect, it } from 'vitest';
import type { ResolvedView } from '../protocol/view';
import { elkGraph, placeWithElk } from './elk';

const style = { shape: 'rect', fill: '#FFF', stroke: '#000' } as const;
const edgeStyle = { stroke: '#000', arrow_end: 'arrow' };
const view: ResolvedView = {
  view: 'main',
  grid: 10,
  nodes: [
    { id: 'a', kind: 'k', label: 'a', frame: 'g', style },
    { id: 'b', kind: 'k', label: 'b', frame: 'g', style },
    { id: 'c', kind: 'k', label: 'c', style },
    { id: 'g', kind: 'k', label: 'g', container: true, style },
  ],
  edges: [
    { id: 'ab', kind: 'default', from: { node: 'a' }, to: { node: 'b' }, labels: {}, style: edgeStyle },
    { id: 'ca', kind: 'default', from: { node: 'c' }, to: { node: 'a' }, labels: {}, style: edgeStyle },
  ],
};
const sizes = new Map(['a', 'b', 'c', 'g'].map((id) => [id, { w: 100, h: 40 }]));

describe('elkGraph', () => {
  it('nests children and puts each edge in the innermost common container', () => {
    const g = elkGraph(view, sizes);
    expect(g.children!.map((c) => c.id)).toEqual(['n:c', 'n:g']);
    expect(g.edges!.map((e) => e.id)).toEqual(['e:ca']);
    const group = g.children![1];
    expect(group.children!.map((c) => c.id)).toEqual(['n:a', 'n:b']);
    expect(group.edges!.map((e) => e.id)).toEqual(['e:ab']);
  });
});

describe('placeWithElk', () => {
  it('is deterministic and on the grid', async () => {
    const first = await placeWithElk(new ELK(), view, sizes);
    const second = await placeWithElk(new ELK(), view, sizes);
    expect([...first]).toEqual([...second]);
    expect([...first.keys()].sort()).toEqual(['a', 'b', 'c', 'g']);
    for (const p of first.values()) {
      expect(p.x % 10).toBe(0);
      expect(p.y % 10).toBe(0);
    }
  });
});

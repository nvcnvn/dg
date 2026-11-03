import ELK from 'elkjs/lib/elk.bundled.js';
import { describe, expect, it } from 'vitest';
import { edge, node, text } from '../fixtures';
import { measureNode } from '../geometry/geometry';
import type { ResolvedView } from '../protocol/view';
import { elkGraph, placeWithElk } from './elk';

const view: ResolvedView = {
  view: 'main',
  grid: 10,
  nodes: [node('a', { frame: 'g' }), node('b', { frame: 'g' }), node('c'), node('g', { container: true }, { padding: 20 })],
  edges: [edge('ab', 'a', 'b'), edge('ca', 'c', 'a'), edge('ga', 'g', 'a')],
};
const measures = new Map(view.nodes.map((n) => [n.id, measureNode(n, text, 10)]));

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

describe('placeWithElk', () => {
  it('is deterministic and grid-aligned', async () => {
    const elk = new ELK();
    const first = await placeWithElk(elk, view, measures);
    const second = await placeWithElk(elk, view, measures);
    expect([...first]).toEqual([...second]);
    for (const p of first.values()) {
      expect(p.x % 10).toBe(0);
      expect(p.y % 10).toBe(0);
    }
  });
});

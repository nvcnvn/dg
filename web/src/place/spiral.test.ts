import { describe, expect, it } from 'vitest';
import { edge, node, text } from '../fixtures';
import { intersects, measureNode } from '../geometry/geometry';
import type { ResolvedView } from '../protocol/view';
import { placeSpiral } from './spiral';

function layout(view: ResolvedView) {
  const measures = new Map(view.nodes.map((n) => [n.id, measureNode(n, text, 10)]));
  const pins = new Map(view.nodes.filter((n) => n.pin).map((n) => [n.id, { x: n.pin!.x, y: n.pin!.y }]));
  return placeSpiral(view, measures, pins, 10);
}

describe('placeSpiral', () => {
  const view: ResolvedView = {
    view: 'main',
    grid: 10,
    nodes: [node('a', { pin: { x: 0, y: 0 } }), node('b'), node('c'), node('d', { pin: { x: 400, y: 0 } })],
    edges: [edge('ab', 'a', 'b'), edge('db', 'd', 'b')],
  };

  it('keeps pinned nodes and places the others with four grid units of clearance', () => {
    const l = layout(view);
    expect(l.rel.get('a')).toEqual({ x: 0, y: 0 });
    expect(l.rel.get('d')).toEqual({ x: 400, y: 0 });
    const boxes = [...l.rel].map(([id, p]) => ({ id, ...p, ...l.sizes.get(id)! }));
    for (const a of boxes) {
      for (const b of boxes) {
        if (a.id < b.id) expect(intersects({ x: a.x - 40, y: a.y - 40, w: a.w + 80, h: a.h + 80 }, b)).toBe(false);
      }
    }
  });

  it('starts at the centroid of placed neighbors', () => {
    const b = layout(view).rel.get('b')!;
    expect(b.x).toBe(200);
  });

  it('is deterministic', () => {
    expect([...layout(view).rel]).toEqual([...layout(view).rel]);
  });

  it('keeps coordinates inside a container non-negative', () => {
    const v: ResolvedView = {
      view: 'main',
      grid: 10,
      nodes: [node('g', { container: true, pin: { x: 0, y: 0 } }), node('x', { frame: 'g', pin: { x: 0, y: 0 } }), node('y', { frame: 'g' })],
      edges: [],
    };
    const p = layout(v).rel.get('y')!;
    expect(p.x).toBeGreaterThanOrEqual(0);
    expect(p.y).toBeGreaterThanOrEqual(0);
  });
});

it('keeps clear of a pin with negative coordinates inside a container', () => {
  const v: ResolvedView = {
    view: 'main',
    grid: 10,
    nodes: [node('box', { container: true, pin: { x: 0, y: 0 } }), node('a', { frame: 'box', pin: { x: -200, y: -200 } }), node('b', { frame: 'box' })],
    edges: [],
  };
  const l = layout(v);
  expect(l.rel.get('a')).toEqual({ x: 0, y: 0 });
  expect(intersects({ ...l.rel.get('a')!, ...l.sizes.get('a')! }, { ...l.rel.get('b')!, ...l.sizes.get('b')! })).toBe(false);
});

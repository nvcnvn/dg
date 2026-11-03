import { describe, expect, it } from 'vitest';
import type { DrawnNode } from '../canvas/markup';
import { editScene as fixture, node } from '../fixtures';
import type { Box } from '../geometry/geometry';
import type { Policy, ResolvedView, ViewNode } from '../protocol/view';
import { liveEdges, liveRequest, liveRoute } from '../route/live';
import {
  alignOps,
  batch,
  clampDelta,
  distributeOps,
  edgeAt,
  grown,
  hintOp,
  inside,
  labelPos,
  landing,
  movable,
  moved,
  moveOps,
  portPin,
  reorderOps,
  resizedBox,
  resizeOps,
  scene,
  snapDelta,
  unpinOps,
  withPins,
  withVia,
  type Op,
} from './edit';

function idOf(o: Op): string {
  return ('id' in o && o.id) || '';
}

describe('selection', () => {
  it('moves top-level nodes not placed by a policy, with what they hold', () => {
    const s = fixture();
    expect(movable(s, ['c', 'g', 's1', 'a'])).toEqual(['a', 'g']);
    expect([...moved(s, ['g', 's'])].sort()).toEqual(['c', 'g', 's', 's1', 's2', 's3']);
  });
  it('selects the nodes inside a box and the edge near a point', () => {
    const s = fixture();
    expect(inside(s.drawing, { x: -5, y: -5, w: 310, h: 150 })).toEqual(['a', 'b']);
    expect(edgeAt(s.drawing, { x: 153, y: 70 }, 6)).toBe('ab');
    expect(edgeAt(s.drawing, { x: 170, y: 70 }, 6)).toBeUndefined();
  });
});

describe('move', () => {
  it('keeps a node inside its frame', () => {
    const s = fixture();
    expect(clampDelta(s, ['c'], -50, -50)).toEqual({ x: -10, y: -10 });
    expect(clampDelta(s, ['c'], 500, 500)).toEqual({ x: 500, y: 500 });
    expect(clampDelta(s, ['a'], -500, -500)).toEqual({ x: -500, y: -500 });
    // In a container with a pinned size the node stops at its inside: its padding of 10 from
    // its right and bottom sides.
    expect(clampDelta(fixture({ gPin: { w: 200, h: 120 } }), ['c'], 500, 500)).toEqual({ x: 60, y: 20 });
    expect(snapDelta(s, ['a', 'c'], 14, -16)).toEqual({ x: 10, y: -10 });
  });
  it('grows the containers around a moved node unless their size is pinned', () => {
    const s = fixture();
    expect(grown(s, new Map([['c', { x: 200, y: 300, w: 100, h: 40 }]]))).toEqual(new Map([['g', { x: 0, y: 200, w: 310, h: 150 }]]));
    expect(grown(fixture({ gPin: { w: 200 } }), new Map([['c', { x: 200, y: 300, w: 100, h: 40 }]]))).toEqual(new Map([['g', { x: 0, y: 200, w: 200, h: 150 }]]));
  });
  it('pushes the children of a stack after a child that grew', () => {
    const stack: Policy = { layout: 'stack', direction: 'vertical', gap: 0, columns: 1, align: 'start', order: ['l1', 'l2'] };
    const nodes: [ViewNode, Box, Partial<DrawnNode>][] = [
      [node('set', { container: true, policy: stack, pin: { x: 0, y: 0 } }), { x: 0, y: 0, w: 300, h: 220 }, { container: true, origin: { x: 10, y: 10 } }],
      [node('l1', { container: true, frame: 'set' }), { x: 10, y: 10, w: 280, h: 100 }, { container: true, frame: 'set', orderedIn: 'set', origin: { x: 20, y: 20 } }],
      [node('l2', { container: true, frame: 'set' }), { x: 10, y: 110, w: 280, h: 100 }, { container: true, frame: 'set', orderedIn: 'set', origin: { x: 20, y: 120 } }],
      [node('x', { frame: 'l1', pin: { x: 0, y: 0 } }), { x: 20, y: 20, w: 100, h: 40 }, { frame: 'l1', rel: { x: 0, y: 0 } }],
      [node('y', { frame: 'l2', pin: { x: 0, y: 0 } }), { x: 20, y: 120, w: 100, h: 40 }, { frame: 'l2', rel: { x: 0, y: 0 } }],
    ];
    const view: ResolvedView = { view: 'main', grid: 10, nodes: nodes.map(([n]) => n), edges: [] };
    const drawing = { ...fixture().drawing, edges: [], nodes: nodes.map(([n, box, extra]): DrawnNode => ({ ...fixture().drawing.nodes[0], id: n.id, box, textBox: box, ...extra })) };
    const s = scene(view, drawing);
    expect(grown(s, new Map([['x', { x: 20, y: 100, w: 100, h: 40 }]]))).toEqual(
      new Map([
        ['l1', { x: 10, y: 10, w: 280, h: 140 }],
        ['l2', { x: 10, y: 150, w: 280, h: 100 }],
        ['y', { x: 20, y: 160, w: 100, h: 40 }],
        ['set', { x: 0, y: 0, w: 300, h: 260 }],
      ]),
    );
  });
  it('pins every node at the first drag, and moves only once pinned', () => {
    const moves = moveOps(fixture({ pinned: false }), ['a'], 10, 20);
    expect(moves).toEqual([{ op: 'move', view: 'main', id: 'a', x: 10, y: 20 }]);
    const ops = withPins(fixture({ pinned: false }), moves, new Set(['a']));
    expect(ops.map(idOf)).toEqual(['b', 'c', 'g', 's', 'a']);
    expect(ops[1]).toEqual({ op: 'move', view: 'main', id: 'c', x: 10, y: 10 });
    expect(withPins(fixture(), moves, new Set(['a']))).toEqual(moves);
    expect(batch(moves)).toEqual(moves[0]);
    expect(batch([])).toBeUndefined();
    expect(batch(ops)).toEqual({ op: 'batch', ops });
  });
});

describe('arrange', () => {
  it('aligns and distributes, keeping nodes in their frames', () => {
    const s = fixture();
    expect(alignOps(s, ['a', 'b', 'c'], 'right')).toEqual([
      { op: 'move', view: 'main', id: 'a', x: 200, y: 0 },
      { op: 'move', view: 'main', id: 'c', x: 180, y: 10 },
    ]);
    expect(alignOps(s, ['a', 'c'], 'top')).toEqual([{ op: 'move', view: 'main', id: 'c', x: 10, y: 0 }]);
    expect(alignOps(s, ['a'], 'top')).toEqual([]);
    expect(distributeOps(s, ['a', 'b', 'c'], 'y')).toEqual([{ op: 'move', view: 'main', id: 'b', x: 200, y: 130 }]);
  });
  it('pins the nodes it lines up in a view with nothing pinned', () => {
    const ops = alignOps(fixture({ pinned: false }), ['a', 'b'], 'left');
    expect(ops.map(idOf)).toEqual(['c', 'g', 's', 'a', 'b']);
  });
});

describe('resize', () => {
  it('moves a corner on the grid, keeping a uniform shape and the content origin', () => {
    const b = { x: 30, y: 250, w: 100, h: 40 };
    expect(resizedBox(b, 'se', 23, 7, 10, false)).toEqual({ x: 30, y: 250, w: 120, h: 50 });
    expect(resizedBox(b, 'nw', 20, 10, 10, false)).toEqual({ x: 50, y: 260, w: 80, h: 30 });
    expect(resizedBox(b, 'nw', -50, -50, 10, false, { x: 20, y: 240 })).toEqual({ x: 20, y: 240, w: 110, h: 50 });
    expect(resizedBox(b, 'se', 100, 0, 10, true)).toEqual({ x: 30, y: 250, w: 200, h: 80 });
    // A ratio of 1.5 stays 1.5 on a grid of 10: 150 by 100 or 180 by 120, never 160 by 110.
    expect(resizedBox({ x: 0, y: 0, w: 120, h: 80 }, 'se', 40, 0, 10, true)).toEqual({ x: 0, y: 0, w: 150, h: 100 });
    expect(resizedBox({ x: 0, y: 0, w: 120, h: 80 }, 'se', 0, 40, 10, true)).toEqual({ x: 0, y: 0, w: 180, h: 120 });
    expect(resizedBox(b, 'se', -500, -500, 10, false)).toEqual({ x: 30, y: 250, w: 10, h: 10 });
  });
  it('sends a resize, with a move when the top left moved', () => {
    const s = fixture();
    expect(resizeOps(s, 'a', { x: 0, y: 0, w: 120, h: 60 })).toEqual([{ op: 'resize', view: 'main', id: 'a', w: 120, h: 60 }]);
    expect(resizeOps(s, 'a', { x: -20, y: 0, w: 120, h: 40 })).toEqual([
      { op: 'move', view: 'main', id: 'a', x: -20, y: 0 },
      { op: 'resize', view: 'main', id: 'a', w: 120, h: 40 },
    ]);
    expect(resizeOps(s, 's1', { x: 410, y: 30, w: 140, h: 40 })).toEqual([{ op: 'resize', view: 'main', id: 's1', w: 140, h: 40 }]);
  });
});

describe('reorder', () => {
  it('puts a stack child before the first sibling past the pointer', () => {
    const s = fixture();
    expect(reorderOps(s, 's1', { x: 450, y: 160 })).toEqual([{ op: 'reorder', view: 'main', container: 's', order: ['s2', 's3', 's1'] }]);
    expect(reorderOps(s, 's3', { x: 450, y: 60 })).toEqual([{ op: 'reorder', view: 'main', container: 's', order: ['s1', 's3', 's2'] }]);
    expect(reorderOps(s, 's3', { x: 450, y: 40 })).toEqual([{ op: 'reorder', view: 'main', container: 's', order: ['s3', 's1', 's2'] }]);
    expect(reorderOps(s, 's1', { x: 450, y: 50 })).toEqual([]);
    expect(reorderOps(s, 'a', { x: 0, y: 0 })).toEqual([]);
    expect(landing(s, 's1', { x: 450, y: 160 })?.mark).toEqual({ line: [{ x: 410, y: 175 }, { x: 530, y: 175 }] });
    expect(landing(s, 's3', { x: 450, y: 60 })?.mark).toEqual({ line: [{ x: 410, y: 75 }, { x: 530, y: 75 }] });
    expect(landing(s, 's2', { x: 450, y: 0 })?.mark).toEqual({ line: [{ x: 410, y: 25 }, { x: 530, y: 25 }] });
  });
});

describe('unpin', () => {
  it('unpins the nodes their entries pin', () => {
    expect(unpinOps(fixture(), ['s1', 's3', 'a']).map(idOf)).toEqual(['a', 's3']);
    expect(unpinOps(fixture({ pinned: false }), ['a'])).toEqual([]);
  });
});

describe('hints', () => {
  it('pins a port on the side nearest the pointer', () => {
    expect(portPin({ x: 0, y: 0, w: 100, h: 40 }, { x: 103, y: 30 })).toEqual({ side: 'right', offset: 0.75 });
    expect(portPin({ x: 0, y: 0, w: 100, h: 40 }, { x: -20, y: -9 })).toEqual({ side: 'left', offset: 0 });
    expect(portPin({ x: 0, y: 0, w: 100, h: 40 }, { x: 150, y: 5 })).toEqual({ side: 'right', offset: 0.13 });
  });
  it('keeps the rest of a hint', () => {
    const s = fixture();
    const ac = s.view.edges[1];
    expect(hintOp(s, ac, { from_side: 'bottom' })).toEqual({ op: 'set_edge_hint', view: 'main', id: 'ac', hint: { to_side: 'top', from_side: 'bottom' } });
    expect(hintOp(s, ac, { to_side: undefined, via: [] })).toEqual({ op: 'set_edge_hint', view: 'main', id: 'ac', hint: {} });
  });
  it('places a label and a via point along the route', () => {
    const route = [{ x: 0, y: 0 }, { x: 100, y: 0 }, { x: 100, y: 100 }];
    expect(labelPos(route, { x: 110, y: 50 })).toBe(0.75);
    expect(withVia(route, [[0, 0], [100, 100]], { x: 104, y: 33 }, 10)).toEqual([[0, 0], [100, 30], [100, 100]]);
    expect(withVia(route, [], { x: 52, y: 1 }, 10)).toEqual([[50, 0]]);
  });
});

describe('live routes', () => {
  it('routes again only the edges with one end moving, keeping hinted ends on their side', () => {
    const s = fixture();
    const edges = liveEdges(s.view, new Map(s.drawing.nodes.map((n) => [n.id, n.box])), new Map(s.drawing.edges.map((e) => [e.id, e.points])), moved(s, ['g']));
    expect(edges.map((e) => e.id)).toEqual(['ac']);
    const ac = edges[0];
    expect(ac.from).toEqual({ node: 'a', at: { x: 50, y: 20 }, dir: 'all', moves: false });
    expect(ac.to).toEqual({ node: 'c', at: { x: 50, y: 250 }, dir: 'top', moves: true });
    const d = { x: 100, y: 0 };
    expect(liveRequest(ac, d)).toEqual({ id: 'ac', type: 'orthogonal', from: { at: { x: 50, y: 20 }, obstacle: 'a' }, to: { at: { x: 150, y: 238 } }, via: [] });
    const box = (id: string) => (id === 'a' ? s.nodes.get('a')!.box : { ...s.nodes.get(id)!.box, x: s.nodes.get(id)!.box.x + 100 });
    expect(liveRoute(ac, [{ x: 50, y: 20 }, { x: 50, y: 140 }, { x: 150, y: 140 }, { x: 150, y: 238 }], d, box)).toEqual([
      { x: 50, y: 40 },
      { x: 50, y: 140 },
      { x: 150, y: 140 },
      { x: 150, y: 250 },
    ]);
    expect(liveRoute(ac, undefined, d, box)).toEqual([{ x: 59, y: 40 }, { x: 150, y: 250 }]);
  });
});

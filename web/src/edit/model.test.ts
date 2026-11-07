import { describe, expect, it } from 'vitest';
import slugs from '../../../testdata/ids/slug.json';
import { editScene } from '../fixtures';
import type { ModelInfo } from '../protocol/client';
import type { Op } from '../protocol/ops';
import type { Scene } from './edit';
import { clearOf, copy, createOps, decorationOp, deleteOps, dropTarget, endAt, hideOps, newID, nodeID, nodeSelector, pasteOps, reparentOps, selectors, slug } from './model';

const model: ModelInfo = {
  kinds: [{ name: 'node' }, { name: 'group', container: true }],
  edge_kinds: ['default', 'async'],
  tags: ['hot'],
  nodes: [
    { id: 'a', kind: 'node', label: 'Alpha', tags: ['hot'], ports: [{ name: 'out', side: 'right' }] },
    { id: 'b', kind: 'node', label: 'b' },
    { id: 'c', kind: 'node', label: 'Gamma', parent: 'g', props: { n: 2 } },
    { id: 'g', kind: 'group', label: 'Group' },
    { id: 'ro', kind: 'node', label: 'ro', read_only: true },
    { id: 's', kind: 'stack', label: 's' },
    { id: 's1', kind: 'node', label: 's1', parent: 's' },
    { id: 's2', kind: 'node', label: 's2', parent: 's' },
    { id: 's3', kind: 'node', label: 's3', parent: 's' },
  ],
  edges: [
    { id: 'ab', kind: 'async', from: { node: 'a', port: 'out' }, to: { node: 'b' }, labels: { mid: 'x' } },
    { id: 'ac', kind: 'default', from: { node: 'a' }, to: { node: 'c' }, labels: {} },
  ],
};

const fixture = (pinned = true): Scene => editScene({ pinned, model });

describe('IDs', () => {
  it('generates IDs as spec 6.2 says', () => {
    for (const t of slugs) expect(slug(t.in), t.in).toBe(t.slug);
    expect(slug('x'.repeat(70))).toHaveLength(64);
    expect(newID('9lives', new Set())).toBe('n-9lives');
    expect(newID('', new Set())).toBe('n');
    expect(newID('all', new Set())).toBe('all-2');
    expect(nodeID('service', 'API', new Set(['api', 'api-2']))).toBe('api-3');
    expect(nodeID('service', '', new Set())).toBe('service');
  });
});

describe('palette', () => {
  it('pins a node dropped on the canvas or in a free container, in its frame', () => {
    expect(createOps(fixture(), 'node', { x: 260, y: 300 })).toEqual({ op: 'create_node', kind: 'node', view: 'main', x: 200, y: 280 });
    expect(createOps(fixture(), 'node', { x: 100, y: 260 })).toEqual({ op: 'create_node', kind: 'node', view: 'main', parent: 'g', x: 20, y: 0 });
  });
  it('puts a node dropped in a stack in its order', () => {
    expect(createOps(fixture(), 'node', { x: 450, y: 90 })).toEqual({ op: 'create_node', kind: 'node', view: 'main', parent: 's', before: 's2' });
    expect(createOps(fixture(), 'node', { x: 450, y: 190 })).toEqual({ op: 'create_node', kind: 'node', view: 'main', parent: 's' });
  });
  it('puts a node dropped at the side of a stack child into the stack', () => {
    const s = fixture();
    const lane = s.nodes.get('s2')!;
    lane.container = true;
    lane.origin = { x: 420, y: 90 };
    expect(createOps(s, 'node', { x: 450, y: 84 })).toEqual({ op: 'create_node', kind: 'node', view: 'main', parent: 's', before: 's2' });
    expect(createOps(s, 'node', { x: 450, y: 100 })).toEqual({ op: 'create_node', kind: 'node', view: 'main', parent: 's2', x: 0, y: 0 });
  });
  it('pins every node with the first one in a view with nothing pinned', () => {
    const op = createOps(fixture(false), 'node', { x: 260, y: 300 });
    expect(op?.op).toBe('batch');
    expect(op?.op === 'batch' && op.ops.map((o) => o.op)).toEqual(['move', 'move', 'move', 'move', 'move', 'create_node']);
  });
  it('finds the innermost container under a point', () => {
    expect(dropTarget(fixture(), { x: 50, y: 260 })).toBe('g');
    expect(dropTarget(fixture(), { x: 50, y: 260 }, new Set(['g']))).toBeUndefined();
    expect(dropTarget(fixture(), { x: 300, y: 300 })).toBeUndefined();
  });
});

describe('copy and paste', () => {
  it('copies the drawn nodes inside the selection and the edges among them', () => {
    const clip = copy(fixture(), ['a', 'g'])!;
    expect(clip.nodes.map((n) => [n.node.id, n.frame])).toEqual([
      ['a', undefined],
      ['g', undefined],
      ['c', 'g'],
    ]);
    expect(clip.edges.map((e) => e.id)).toEqual(['ac']);
  });
  it('generates the ID of a copy with no slug in its label from its kind without the prefix', () => {
    const s = editScene({ pinned: true, model: { ...model, kinds: [...model.kinds, { name: 'aws-dynamodb', pack: 'aws', short: 'dynamodb' }, { name: 'aws-own' }] } });
    const clip = copy(s, ['a', 'b'])!;
    clip.nodes[0].node = { ...clip.nodes[0].node, kind: 'aws-dynamodb', label: '' };
    clip.nodes[1].node = { ...clip.nodes[1].node, kind: 'aws-own', label: '' };
    expect((pasteOps(s, clip, { x: 0, y: 500 }) as { ops: Op[] }).ops.filter((o) => o.op === 'create_node').map((o) => o.id)).toEqual(['dynamodb', 'aws-own']);
  });

  it('creates the copies with new IDs at the paste point, then the edges, as one batch', () => {
    const s = fixture();
    const op = pasteOps(s, copy(s, ['a', 'b'])!, { x: 0, y: 500 });
    expect(op).toEqual({
      op: 'batch',
      ops: [
        { op: 'create_node', kind: 'node', id: 'alpha', label: 'Alpha', view: 'main', x: 0, y: 500 },
        { op: 'set_tags', id: 'alpha', tags: ['hot'] },
        { op: 'add_port', id: 'alpha', name: 'out', side: 'right' },
        { op: 'create_node', kind: 'node', id: 'b-2', label: 'b', view: 'main', x: 200, y: 600 },
        { op: 'connect', from: { node: 'alpha', port: 'out' }, to: { node: 'b-2' }, kind: 'async', labels: { mid: 'x' }, view: 'main' },
      ],
    });
  });
  it('keeps pinned sizes and ports', () => {
    const s = fixture();
    s.view.nodes.find((n) => n.id === 'b')!.pin = { x: 200, y: 100, w: 100, ports: { out: { side: 'right', offset: 0.25 } } };
    const op = pasteOps(s, copy(s, ['b'])!, { x: 0, y: 500 });
    expect(op?.op === 'batch' && op.ops.slice(1)).toEqual([
      { op: 'resize', view: 'main', id: 'b-2', w: 100, h: 40 },
      { op: 'pin_port', view: 'main', id: 'b-2', port: 'out', side: 'right', offset: 0.25 },
    ]);
  });
  it('keeps the children of a copied container in the copy', () => {
    const s = fixture();
    const op = pasteOps(s, copy(s, ['g'])!, { x: 0, y: 500 });
    expect(op?.op === 'batch' && op.ops).toEqual([
      { op: 'create_node', kind: 'group', id: 'group', label: 'Group', view: 'main', x: 0, y: 500 },
      { op: 'create_node', kind: 'node', id: 'gamma', label: 'Gamma', view: 'main', parent: 'group', x: 10, y: 10 },
      { op: 'set_props', id: 'gamma', props: { n: 2 } },
    ]);
  });
});

describe('re-parent', () => {
  it('sets the parent and moves the node into the container under the pointer', () => {
    expect(reparentOps(fixture(), ['b'], { x: -150, y: 160 }, { x: 100, y: 280 })).toEqual({
      target: 'g',
      ops: [
        { op: 'set_parent', id: 'b', parent: 'g' },
        { op: 'move', view: 'main', id: 'b', x: 30, y: 20 },
      ],
    });
  });
  it('keeps the node inside a container whose size is pinned', () => {
    const s = editScene({ model, gPin: { w: 200, h: 120 } });
    expect(reparentOps(s, ['b'], { x: -50, y: 260 }, { x: 180, y: 300 }).ops).toEqual([
      { op: 'set_parent', id: 'b', parent: 'g' },
      { op: 'move', view: 'main', id: 'b', x: 70, y: 30 },
    ]);
  });
  it('clears the parent on the canvas', () => {
    expect(reparentOps(fixture(), ['c'], { x: 300, y: 0 }, { x: 380, y: 270 }).ops).toEqual([
      { op: 'set_parent', id: 'c' },
      { op: 'move', view: 'main', id: 'c', x: 330, y: 250 },
    ]);
  });
  it('puts a node dropped in a stack in its order', () => {
    expect(reparentOps(fixture(), ['a'], { x: 410, y: 60 }, { x: 450, y: 90 }).ops).toEqual([
      { op: 'set_parent', id: 'a', parent: 's' },
      { op: 'reorder', view: 'main', container: 's', order: ['s1', 'a', 's2', 's3'] },
      { op: 'unpin', view: 'main', id: 'a' },
    ]);
  });
  it('sends nothing when the parent stays, and refuses a read-only node', () => {
    expect(reparentOps(fixture(), ['c'], { x: 10, y: 0 }, { x: 60, y: 270 })).toEqual({ target: 'g' });
    const s = fixture();
    s.model.nodes.find((n) => n.id === 'b')!.read_only = true;
    expect(reparentOps(s, ['b'], { x: -150, y: 160 }, { x: 100, y: 280 }).refused).toMatch(/read-only/);
    s.model.nodes.find((n) => n.id === 'b')!.read_only = false;
  });
});

describe('connect', () => {
  it('ends at a port, a row or a node', () => {
    const s = fixture();
    s.nodes.get('a')!.ports.push({ name: 'out', side: 'right', x: 100, y: 20 });
    expect(endAt(s, { x: 102, y: 21 }, 6)).toEqual({ node: 'a', port: 'out' });
    expect(endAt(s, { x: 50, y: 260 }, 6)).toEqual({ node: 'c' });
    expect(endAt(s, { x: 10, y: 210 }, 6)).toEqual({ node: 'g' });
    expect(endAt(s, { x: 300, y: 300 }, 6)).toBeUndefined();
    s.nodes.get('b')!.table = { lines: [130], rows: [{ name: 'id', y: 120 }, { name: 'name', y: 136 }], keyX: 0, nameX: 0, typeX: 0, font: { size: 14, weight: 400 }, keyFont: { size: 14, weight: 700 } };
    expect(endAt(s, { x: 250, y: 134 }, 6)).toEqual({ node: 'b', row: 'name' });
    s.nodes.get('a')!.ports.pop();
    delete s.nodes.get('b')!.table;
  });
});

describe('context menu', () => {
  it('deletes and hides nodes in one batch', () => {
    expect(deleteOps(['b', 'a'])).toEqual({ op: 'batch', ops: [{ op: 'delete_node', id: 'a' }, { op: 'delete_node', id: 'b' }] });
    expect(deleteOps(['g'], true)).toEqual({ op: 'delete_node', id: 'g', recursive: true });
    expect(hideOps(fixture(), ['a'])).toEqual({ op: 'hide_node', view: 'main', id: 'a' });
  });
  it('splits the selectors of the new-view dialog', () => {
    expect(selectors('all, node.a  node["a b, c"],kind.x')).toEqual(['all', 'node.a', 'node["a b, c"]', 'kind.x']);
    expect(selectors('  ')).toEqual([]);
    expect(['api', 'kube/dns', '9x'].map(nodeSelector)).toEqual(['node.api', 'node["kube/dns"]', 'node["9x"]']);
  });
  it('adds decorations at a point or attached to a node', () => {
    expect(decorationOp(fixture(), 'text', { x: 13, y: 27 }, undefined, 'Hi')).toEqual({ op: 'add_decoration', view: 'main', decoration: { type: 'text', x: 10, y: 30, text: 'Hi' } });
    expect(decorationOp(fixture(), 'rect', { x: 0, y: 0 }, 'a')).toEqual({ op: 'add_decoration', view: 'main', decoration: { type: 'rect', attach: 'a', dx: 0, dy: -50, w: 120, h: 60 } });
  });
});

describe('clearOf', () => {
  const box = { x: 100, y: 100, w: 120, h: 40 };
  const at = (boxes: (typeof box)[]) => ({ view: { grid: 10 }, drawing: { nodes: boxes.map((b, i) => ({ id: `n${i}`, box: b, container: false })) } }) as unknown as Scene;
  it.each([
    ['an empty place stays', [], { x: 500, y: 500 }, { x: 500, y: 500 }],
    ['beside a node', [box], { x: 160, y: 120 }, { x: 300, y: 120 }],
    ['past the next node too', [box, { ...box, x: 240 }], { x: 160, y: 120 }, { x: 440, y: 120 }],
  ])('%s', (_, boxes, p, want) => {
    expect(clearOf(at(boxes), p)).toEqual(want);
  });
});

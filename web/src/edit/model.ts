import { cmp, roundTo, type Box, type Point } from '../geometry/geometry';
import type { ModelEdge, ModelNode } from '../protocol/client';
import { RESERVED, type Op } from '../protocol/ops';
import type { End, Pin, Policy, ViewDecoration } from '../protocol/view';
import { ancestors, batch, moved, withPins, type Scene } from './edit';

// Model edits (spec FR-11): the operations the palette, paste, connect, re-parent and the
// context menu send. Each gesture is one operation or one batch, so it is one undo step.

// slug lowercases s and keeps a-z and 0-9, every other run of characters becoming one dash,
// trimmed of dashes and cut to 64 characters (spec 6.2, internal/ops slug).
export function slug(s: string): string {
  let out = '';
  let dash = false;
  for (const ch of s) {
    // Go lowers a character to one character: İ to i, where JavaScript adds a combining dot.
    const c = [...ch.toLowerCase()][0];
    if ((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
      if (dash && out) out += '-';
      dash = false;
      out += c;
    } else dash = true;
  }
  return out.slice(0, 64).replace(/-$/, '');
}

// newID returns an ID for base that taken does not hold and is not reserved: base, or base with
// the first free suffix -2, -3, …; a base that does not start with a letter is prefixed with n-.
export function newID(base: string, taken: Set<string>): string {
  if (!/^[a-z]/.test(base)) base = ('n-' + base).replace(/-$/, '');
  let id = base;
  for (let i = 2; taken.has(id) || RESERVED.includes(id); i++) id = `${base}-${i}`;
  return id;
}

// nodeID is the ID of a new node of kind labeled label (spec 6.2).
export function nodeID(kind: string, label: string, taken: Set<string>): string {
  return newID(slug(label) || slug(kind), taken);
}

// policyOf returns the policy of stack or grid container c in the view: an expanded one, or
// the view's of when its policy places the canvas.
export function policyOf(s: Scene, c: string): Policy | undefined {
  if (c === s.view.policy?.container) return s.view.policy;
  return s.view.nodes.find((n) => n.id === c)?.policy;
}

// slot returns where in order, which a stack or grid places, a node dropped at p goes: before
// the first sibling past p along a stack, or in the place of the sibling nearest p in a grid.
export function slot(s: Scene, policy: Policy, order: string[], p: Point): number {
  const box = (o: string) => s.nodes.get(o)!.box;
  const mid = (o: string) => ({ x: box(o).x + box(o).w / 2, y: box(o).y + box(o).h / 2 });
  if (!order.length) return 0;
  if (policy.layout === 'grid') {
    const d = order.map((o) => Math.hypot(mid(o).x - p.x, mid(o).y - p.y));
    return d.indexOf(Math.min(...d));
  }
  const k = policy.direction === 'vertical' ? 'y' : 'x';
  return order.filter((o) => mid(o)[k] < p[k]).length;
}

// dropTarget returns the innermost expanded container drawn under p, other than the nodes of
// skip, or undefined for the canvas.
export function dropTarget(s: Scene, p: Point, skip: Set<string> = new Set()): string | undefined {
  const under = s.drawing.nodes.filter((n) => n.container && !skip.has(n.id) && within(n.box, p));
  under.sort((a, b) => ancestors(s, b.id).length - ancestors(s, a.id).length || cmp(a.id, b.id));
  return under[0]?.id;
}

// Placement is where a new node goes: its parent, and its position in the parent's frame, or,
// when a stack or grid container places it, the sibling it goes before (none: last).
export interface Placement {
  parent?: string;
  x?: number;
  y?: number;
  before?: string;
}

// placement returns where a node whose top left corner is at p goes when dropped into target,
// a container drawn expanded or the canvas (undefined). On the canvas of a view with of, the
// node goes inside the of container.
export function placement(s: Scene, target: string | undefined, p: Point, at: Point = p): Placement {
  const g = s.view.grid;
  const parent = target ?? (s.view.of && !s.nodes.has(s.view.of) ? s.view.of : undefined);
  const policy = parent && (target ? policyOf(s, target) : s.view.policy);
  if (policy) {
    const i = slot(s, policy, policy.order, at);
    return { parent, ...(policy.order[i] !== undefined ? { before: policy.order[i] } : {}) };
  }
  const o = target ? s.nodes.get(target)!.origin! : { x: 0, y: 0 };
  const x = roundTo(p.x - o.x, g);
  const y = roundTo(p.y - o.y, g);
  return { ...(parent ? { parent } : {}), x: target ? Math.max(0, x) : x, y: target ? Math.max(0, y) : y };
}

// NEW_NODE is the offset from the pointer of a node dropped from the palette to its top left
// corner, about half a node, so the node lands around the pointer.
const NEW_NODE: Point = { x: 60, y: 20 };

// createOps creates a node of kind dropped at p in the view, pinned there (spec FR-11).
export function createOps(s: Scene, kind: string, p: Point): Op | undefined {
  let target = dropTarget(s, p);
  // Within a grid unit of the side of a child of a stack, between it and the next, the node goes
  // into the stack, so a lane can be added between two lanes.
  const t = target ? s.nodes.get(target) : undefined;
  const policy = t?.orderedIn ? policyOf(s, t.orderedIn) : undefined;
  if (t && policy?.layout === 'stack') {
    const [lo, hi, at] = policy.direction === 'vertical' ? [t.box.y, t.box.y + t.box.h, p.y] : [t.box.x, t.box.x + t.box.w, p.x];
    if (at - lo < s.view.grid || hi - at < s.view.grid) target = t.orderedIn;
  }
  const at = placement(s, target, { x: p.x - NEW_NODE.x, y: p.y - NEW_NODE.y }, p);
  return batch(withPins(s, [{ op: 'create_node', kind, view: s.view.view, ...at }], new Set()));
}

// Clip is what copy keeps: nodes with where they were drawn, and the edges among them.
export interface Clip {
  nodes: ClipNode[];
  edges: ModelEdge[];
}

// ClipNode is a copied node: frame is the copied container it was drawn in, rel its position
// there, ordered set when that container placed it in order, and pin its pinned size and ports.
export interface ClipNode {
  node: ModelNode;
  box: Box;
  frame?: string;
  rel?: Point;
  ordered: boolean;
  pin?: Pick<Pin, 'w' | 'h' | 'ports'>;
}

// copy returns the nodes of ids drawn in the scene, with the drawn nodes inside them, and the
// model edges among them, parents before children.
export function copy(s: Scene, ids: Iterable<string>): Clip | undefined {
  const set = moved(s, [...ids].filter((id) => s.nodes.has(id)));
  const byId = new Map(s.model.nodes.map((n) => [n.id, n]));
  const nodes = [...set]
    .filter((id) => byId.has(id))
    .sort((a, b) => ancestors(s, a).length - ancestors(s, b).length || cmp(a, b))
    .map((id): ClipNode => {
      const d = s.nodes.get(id)!;
      const frame = d.frame && set.has(d.frame) ? d.frame : undefined;
      const pin = s.view.nodes.find((n) => n.id === id)?.pin;
      const kept = pin && (pin.w !== undefined || pin.h !== undefined || pin.ports) ? { pin: { w: pin.w, h: pin.h, ports: pin.ports } } : {};
      return { node: byId.get(id)!, box: d.box, ordered: !!d.orderedIn, ...(frame ? { frame, rel: d.rel } : {}), ...kept };
    });
  if (!nodes.length) return undefined;
  const kept = new Set(nodes.map((n) => n.node.id));
  return { nodes, edges: s.model.edges.filter((e) => kept.has(e.from.node) && kept.has(e.to.node)) };
}

// pasteOps creates the nodes of clip with new IDs (spec 6.2), the top ones with the top left
// corner of their box at p and in the container there, the others as they were in theirs, and
// the edges among them, as one batch.
export function pasteOps(s: Scene, clip: Clip, p: Point): Op | undefined {
  const taken = new Set(s.model.nodes.map((n) => n.id));
  const ids = new Map<string, string>();
  for (const c of clip.nodes) {
    const id = nodeID(c.node.kind, c.node.label, taken);
    taken.add(id);
    ids.set(c.node.id, id);
  }
  const tops = clip.nodes.filter((c) => !c.frame);
  const x0 = Math.min(...tops.map((c) => c.box.x));
  const y0 = Math.min(...tops.map((c) => c.box.y));
  const target = dropTarget(s, p);
  const view = s.view.view;
  const ops: Op[] = [];
  for (const c of clip.nodes) {
    const n = c.node;
    const id = ids.get(n.id)!;
    let at: Placement;
    if (c.frame) at = { parent: ids.get(c.frame)!, ...(c.ordered || !c.rel ? {} : { x: c.rel.x, y: c.rel.y }) };
    else at = placement(s, target, { x: p.x + c.box.x - x0, y: p.y + c.box.y - y0 }, p);
    ops.push({ op: 'create_node', kind: n.kind, id, label: n.label, view, ...at });
    if (n.tags?.length) ops.push({ op: 'set_tags', id, tags: n.tags });
    if (n.props && Object.keys(n.props).length) ops.push({ op: 'set_props', id, props: n.props });
    for (const port of n.ports ?? []) ops.push({ op: 'add_port', id, name: port.name, ...(port.side ? { side: port.side } : {}) });
    if (n.compartments?.length) ops.push({ op: 'set_rows', id, compartments: n.compartments.map((c) => ({ ...(c.name ? { name: c.name } : {}), rows: c.rows })) });
    if (c.pin?.w !== undefined || c.pin?.h !== undefined) ops.push({ op: 'resize', view, id, w: c.pin.w ?? c.box.w, h: c.pin.h ?? c.box.h });
    for (const [port, p] of Object.entries(c.pin?.ports ?? {}).sort(([a], [b]) => cmp(a, b))) ops.push({ op: 'pin_port', view, id, port, side: p.side, ...(p.offset !== undefined ? { offset: p.offset } : {}) });
  }
  const end = (e: End): End => ({ ...e, node: ids.get(e.node)! });
  for (const e of clip.edges) {
    const labels = Object.keys(e.labels).length ? { labels: e.labels } : {};
    ops.push({ op: 'connect', from: end(e.from), to: end(e.to), ...(e.kind !== 'default' ? { kind: e.kind } : {}), ...labels, ...(e.tags?.length ? { tags: e.tags } : {}), view });
  }
  return batch(withPins(s, ops, new Set()));
}

// Reparent is a re-parenting drop (spec 7.6): the container dropped on (undefined: the canvas),
// and the batch to send, or why it is refused.
export interface Reparent {
  target?: string;
  ops?: Op[];
  refused?: string;
}

// reparentOps re-parents ids, moved by d with the pointer at p, into the innermost container
// under p, or the canvas: each node's parent changes and it is moved there, or put in the order
// of a stack or grid container. A node in a read-only file cannot change.
export function reparentOps(s: Scene, ids: string[], d: Point, p: Point): Reparent {
  const skip = moved(s, ids);
  const target = dropTarget(s, p, skip);
  const parent = target ?? (s.view.of && !s.nodes.has(s.view.of) ? s.view.of : undefined);
  const byId = new Map(s.model.nodes.map((n) => [n.id, n]));
  const ro = ids.find((id) => byId.get(id)?.read_only);
  if (ro) return { target, refused: `Node "${ro}" is defined in a read-only file; its parent cannot change.` };
  const policy = parent && (target ? policyOf(s, target) : s.view.policy);
  const ops: Op[] = [];
  const ordered: string[] = [];
  for (const id of [...ids].sort(cmp)) {
    if (byId.get(id)?.parent !== parent) ops.push({ op: 'set_parent', id, ...(parent ? { parent } : {}) });
    if (policy) {
      ordered.push(id);
      continue;
    }
    const b = s.nodes.get(id)!.box;
    const at = placement(s, target, { x: b.x + d.x, y: b.y + d.y });
    // Inside a container whose size is pinned the node stays within its padding (spec 7.6).
    const f = target ? s.nodes.get(target)! : undefined;
    const pin = target ? s.view.nodes.find((n) => n.id === target)?.pin : undefined;
    const g = s.view.grid;
    const room = (end: number, size: number) => Math.max(0, Math.floor((end - f!.style.padding - size) / g) * g);
    if (f && pin?.w !== undefined) at.x = Math.min(at.x!, room(f.box.x + f.box.w - f.origin!.x, b.w));
    if (f && pin?.h !== undefined) at.y = Math.min(at.y!, room(f.box.y + f.box.h - f.origin!.y, b.h));
    ops.push({ op: 'move', view: s.view.view, id, x: at.x!, y: at.y! });
  }
  if (!ops.some((o) => o.op === 'set_parent')) return { target };
  if (policy && parent) {
    const order = policy.order.filter((o) => !ordered.includes(o));
    order.splice(slot(s, policy, order, p), 0, ...ordered);
    ops.push({ op: 'reorder', view: s.view.view, container: parent, order });
    // A node a stack or grid places has no position (spec 7.3): its position goes, its size and
    // pinned ports stay.
    for (const id of ordered) {
      const pin = s.view.nodes.find((n) => n.id === id)?.pin;
      if (!pin || s.nodes.get(id)!.orderedIn) continue;
      ops.push({ op: 'unpin', view: s.view.view, id });
      const b = s.nodes.get(id)!.box;
      if (pin.w !== undefined || pin.h !== undefined) ops.push({ op: 'resize', view: s.view.view, id, w: pin.w ?? b.w, h: pin.h ?? b.h });
      for (const [port, at] of Object.entries(pin.ports ?? {}).sort(([a], [b]) => cmp(a, b))) ops.push({ op: 'pin_port', view: s.view.view, id, port, side: at.side, ...(at.offset !== undefined ? { offset: at.offset } : {}) });
    }
  }
  return { target, ops: withPins(s, ops, new Set(ids)) };
}

// endAt returns the end of an edge at p: a port within tol of p, else the row of a table under
// p, else the innermost node under p, leaves before containers.
export function endAt(s: Scene, p: Point, tol: number): End | undefined {
  for (const n of [...s.drawing.nodes].reverse()) {
    const port = n.ports.find((q) => Math.hypot(q.x - p.x, q.y - p.y) <= tol);
    if (port) return { node: n.id, port: port.name };
  }
  const under = s.drawing.nodes.filter((n) => within(n.box, p));
  under.sort((a, b) => Number(a.container) - Number(b.container) || ancestors(s, b.id).length - ancestors(s, a.id).length || cmp(a.id, b.id));
  const n = under[0];
  if (!n) return undefined;
  const rows = n.table?.rows ?? [];
  if (rows.length) {
    const gap = rows.length > 1 ? Math.min(...rows.slice(1).map((r, i) => r.y - rows[i].y)) : 2 * n.style.font_size;
    const row = rows.find((r) => Math.abs(r.y - p.y) <= gap / 2);
    if (row) return { node: n.id, row: row.name };
  }
  return { node: n.id };
}

// connectOp connects from to to with an edge of kind (default when empty) shown in the view.
export function connectOp(s: Scene, from: End, to: End, kind: string): Op {
  return { op: 'connect', from, to, ...(kind && kind !== 'default' ? { kind } : {}), view: s.view.view };
}

// deleteOps deletes the nodes of ids from the model; recursive also deletes what they hold.
export function deleteOps(ids: Iterable<string>, recursive = false): Op | undefined {
  return batch([...ids].sort(cmp).map((id): Op => ({ op: 'delete_node', id, ...(recursive ? { recursive } : {}) })));
}

// hideOps hides the nodes of ids from the view, editing its selectors.
export function hideOps(s: Scene, ids: Iterable<string>): Op | undefined {
  return batch([...ids].sort(cmp).map((id): Op => ({ op: 'hide_node', view: s.view.view, id })));
}

export type DecorationType = ViewDecoration['type'];

// decorationOp adds a decoration of type to the view with its top left corner at p, or, attached
// to node attach, centered above it; text holds a text decoration's text or an image's src.
export function decorationOp(s: Scene, type: DecorationType, p: Point, attach?: string, text = ''): Op {
  const g = s.view.grid;
  const fields: Record<DecorationType, object> = {
    text: { text },
    rect: { w: 12 * g, h: 6 * g },
    ellipse: { w: 12 * g, h: 6 * g },
    band: { w: 30 * g, h: 15 * g },
    image: { src: text, w: 6 * g, h: 6 * g },
    arrow: { points: [[0, 0], [10 * g, 0]] },
    freehand: { path: `M0 0 C${2 * g} ${-2 * g} ${4 * g} ${2 * g} ${6 * g} 0` },
  };
  const b = attach ? s.nodes.get(attach)?.box : undefined;
  const where = b ? { attach, dx: 0, dy: -roundTo(b.h / 2 + 3 * g, g) } : { x: roundTo(p.x, g), y: roundTo(p.y, g) };
  return { op: 'add_decoration', view: s.view.view, decoration: { type, ...where, ...fields[type] } };
}

function within(b: Box, p: Point): boolean {
  return p.x >= b.x && p.x <= b.x + b.w && p.y >= b.y && p.y <= b.y + b.h;
}

// selectors splits a list of selectors written as in views.hcl on spaces and commas outside
// brackets, so node["a b"] stays one.
export function selectors(text: string): string[] {
  return text.match(/(?:[^\s,[]|\[[^\]]*\])+/g) ?? [];
}

// nodeSelector is the selector of node id, as views.hcl writes it.
export function nodeSelector(id: string): string {
  return /^[A-Za-z_][A-Za-z0-9_-]*$/.test(id) && !RESERVED.includes(id) ? `node.${id}` : `node[${JSON.stringify(id)}]`;
}

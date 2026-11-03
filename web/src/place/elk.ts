import type { ElkExtendedEdge, ElkNode } from 'elkjs/lib/elk-api';
import { ceilTo, cmp, containerSize, depth, roundTo, type Layout, type NodeMeasure, type Point, type Size } from '../geometry/geometry';
import { arrangeFrame } from '../geometry/policy';
import type { ResolvedView, ViewNode } from '../protocol/view';

export interface Elk {
  layout(graph: ElkNode): Promise<ElkNode>;
}

const ROOT = 'root';
const nodeKey = (id: string) => 'n:' + id;
const edgeKey = (id: string) => 'e:' + id;

const NODE_SPACING = 40;

const spacing: Record<string, string> = {
  'elk.spacing.nodeNode': String(NODE_SPACING),
  'elk.spacing.edgeNode': '20',
  'elk.layered.spacing.nodeNodeBetweenLayers': '60',
  'elk.layered.spacing.edgeNodeBetweenLayers': '20',
};

// A view with edges between rows leaves room between the layers for the markers at both ends of
// an edge, and its layers run left to right unless it sets a direction, since rows attach at
// their left and right ends.
const rowSpacing: Record<string, string> = {
  'elk.layered.spacing.nodeNodeBetweenLayers': '100',
  'elk.layered.spacing.edgeNodeBetweenLayers': '40',
};

const rootOptions: Record<string, string> = {
  ...spacing,
  'elk.algorithm': 'layered',
  'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
  'elk.edgeRouting': 'ORTHOGONAL',
  'elk.randomSeed': '1',
  'elk.edgeLabels.placement': 'CENTER',
};

// EdgeLabelSizes are the sizes of an edge's labels by position.
export type EdgeLabelSizes = Partial<Record<'start' | 'mid' | 'end', Size>>;

const placement = { start: 'TAIL', mid: 'CENTER', end: 'HEAD' };

// elkGraph builds the elk input for a view with nothing pinned: nodes and edges in sorted ID
// order, containers holding their children, each edge in the innermost container holding both
// of its ends, with its labels' sizes so layers leave room for them. An edge with an open
// triangle at one end only (inheritance, realization) is given from that end, the general
// element, so it is drawn above the specific one. An edge between a container and itself or its
// own descendant gives elk nothing to arrange, so it is left out.
export function elkGraph(view: ResolvedView, measures: Map<string, NodeMeasure>, labels: Map<string, EdgeLabelSizes> = new Map()): ElkNode {
  const elkNodes = new Map<string, ElkNode>();
  const rows = view.edges.some((e) => e.from.row || e.to.row);
  const direction = { 'elk.direction': (view.direction ?? (rows ? 'right' : 'down')).toUpperCase() };
  const root: ElkNode = { id: ROOT, layoutOptions: { ...rootOptions, ...(rows ? rowSpacing : {}), ...direction }, children: [], edges: [] };
  for (const n of view.nodes) {
    const m = measures.get(n.id)!;
    const en: ElkNode = { id: nodeKey(n.id), width: m.size.w, height: m.size.h };
    if (n.container) {
      const pad = n.style.padding;
      en.children = [];
      en.edges = [];
      en.layoutOptions = {
        ...spacing,
        ...(rows ? rowSpacing : {}),
        ...direction,
        'elk.padding': `[top=${m.origin.y},left=${m.origin.x},bottom=${pad},right=${pad}]`,
        'elk.nodeSize.constraints': '[MINIMUM_SIZE]',
        'elk.nodeSize.minimum': `(${m.size.w},${m.size.h})`,
      };
    }
    elkNodes.set(n.id, en);
  }
  for (const n of view.nodes) {
    const parent = (n.frame && elkNodes.get(n.frame)) || root;
    parent.children!.push(elkNodes.get(n.id)!);
  }

  const frameOf = new Map(view.nodes.map((n) => [n.id, n.frame]));
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const chain = (id: string): string[] => {
    const ids = [id];
    for (let f = frameOf.get(id); f; f = frameOf.get(f)) ids.push(f);
    return ids;
  };
  const given = view.edges.flatMap((e) => {
    const to = new Set(chain(e.to.node));
    const from = chain(e.from.node);
    const loop = e.from.node === e.to.node;
    if (loop ? byId.get(e.from.node)!.container : to.has(e.from.node) || from.includes(e.to.node)) return [];
    const lca = from.find((f) => to.has(f) && elkNodes.get(f)?.edges);
    const holder = (lca && elkNodes.get(lca)) || root;
    const up = e.style.arrow_end === 'triangle-open' && e.style.arrow_start !== 'triangle-open';
    const [a, b] = up ? [e.to.node, e.from.node] : [e.from.node, e.to.node];
    return [{ e, holder, a, b }];
  });
  const back = backEdges(given.map(({ e, a, b }) => ({ id: e.id, a, b })));
  for (const { e, holder, a, b } of given) {
    const reversed = back.has(e.id);
    const edge: ElkExtendedEdge = { id: edgeKey(e.id), sources: [nodeKey(reversed ? b : a)], targets: [nodeKey(reversed ? a : b)] };
    const sizes = labels.get(e.id) ?? {};
    const ends = reversed ? { start: 'HEAD', mid: 'CENTER', end: 'TAIL' } : placement;
    const elkLabels = (['mid', 'start', 'end'] as const)
      .filter((pos) => sizes[pos])
      .map((pos) => ({ id: `l:${pos}:${e.id}`, width: sizes[pos]!.w, height: sizes[pos]!.h, layoutOptions: { 'elk.edgeLabels.placement': ends[pos] } }));
    if (elkLabels.length) edge.labels = elkLabels;
    holder.edges!.push(edge);
  }
  return root;
}

// backEdges returns the edges to give elk reversed: those a depth-first search meets back to a
// node it is still inside, starting from the nodes no edge enters, in ID order, and following
// edges in ID order. So a flow runs down from where it starts and a loop back to an earlier step
// is the edge reversed. Cycles no such node leads to are left to elk, whose greedy choice reads
// better there than any start dg could pick.
export function backEdges(edges: { id: string; a: string; b: string }[]): Set<string> {
  const out = new Map<string, { id: string; b: string }[]>();
  const into = new Map<string, number>();
  for (const { id, a, b } of edges) {
    if (a === b) continue;
    out.set(a, [...(out.get(a) ?? []), { id, b }]);
    into.set(b, (into.get(b) ?? 0) + 1);
    if (!into.has(a)) into.set(a, 0);
  }
  for (const list of out.values()) list.sort((x, y) => cmp(x.id, y.id));
  const state = new Map<string, 'open' | 'done'>();
  const back = new Set<string>();
  const visit = (n: string) => {
    state.set(n, 'open');
    for (const { id, b } of out.get(n) ?? []) {
      if (state.get(b) === 'open') back.add(id);
      else if (!state.has(b)) visit(b);
    }
    state.set(n, 'done');
  };
  for (const n of [...into.keys()].filter((n) => into.get(n) === 0).sort(cmp)) visit(n);
  return back;
}

// elkPositions returns each node's position from an elk result, relative to its frame's
// content origin and rounded to the grid.
export function elkPositions(result: ElkNode, measures: Map<string, NodeMeasure>, grid: number): Map<string, Point> {
  const out = new Map<string, Point>();
  const visit = (parent: ElkNode, origin: Point) => {
    const children = [...(parent.children ?? [])].sort((a, b) => cmp(a.id, b.id));
    for (const c of children) {
      const id = c.id.slice(2);
      out.set(id, { x: roundTo((c.x ?? 0) - origin.x, grid), y: roundTo((c.y ?? 0) - origin.y, grid) });
      visit(c, measures.get(id)!.origin);
    }
  };
  visit(result, { x: 0, y: 0 });
  return out;
}

// subView is the part of view holding the nodes ids, those whose frame is outside it on its
// canvas; a node already sized in boxes is a box of that size, its children not given. An edge
// end outside ids moves to its nearest ancestor in ids; an edge with an end that has none, or
// moved onto the other end, is left out.
function subView(view: ResolvedView, byId: Map<string, ViewNode>, measures: Map<string, NodeMeasure>, ids: Set<string>, boxes: Map<string, Size>, direction: ResolvedView['direction']) {
  const rep = (id: string) => {
    for (let c: string | undefined = id; c; c = byId.get(c)!.frame) if (ids.has(c)) return c;
    return undefined;
  };
  const nodes = view.nodes.filter((n) => ids.has(n.id)).map((n) => ({ ...n, frame: n.frame && ids.has(n.frame) ? n.frame : undefined, container: n.container && !boxes.has(n.id) }));
  const ms = new Map(measures);
  for (const id of ids) if (boxes.has(id)) ms.set(id, { ...measures.get(id)!, size: boxes.get(id)! });
  const edges = view.edges.flatMap((e) => {
    const a = rep(e.from.node);
    const b = rep(e.to.node);
    if (!a || !b || (a === b && e.from.node !== e.to.node)) return [];
    return [{ ...e, from: a === e.from.node ? e.from : { node: a }, to: b === e.to.node ? e.to : { node: b } }];
  });
  return { view: { ...view, nodes, edges, direction }, measures: ms };
}

// elkLayout lays out a view with nothing pinned (spec 7.4): stack and grid containers first,
// innermost first, by their policy, then the rest by elk with those containers as boxes of
// their size. The children of a free container that a policy places are laid out by elk
// first: in a stack, those of all such siblings together, flowing across the stack, then each
// one's moved to start its content and packed toward its start along the stack, so the
// containers line up as lanes; in a grid, each one's alone.
export async function elkLayout(elk: Elk, view: ResolvedView, measures: Map<string, NodeMeasure>, labels?: Map<string, EdgeLabelSizes>): Promise<Layout> {
  const grid = view.grid;
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const rel = new Map<string, Point>();
  const sizes = new Map<string, Size>();
  const tooSmall = new Set<string>();
  const kids = new Map<string, string[]>();
  for (const n of view.nodes) kids.set(n.frame ?? '', [...(kids.get(n.frame ?? '') ?? []), n.id]);
  const sizeOf = (id: string): Size => {
    let s = sizes.get(id);
    if (s) return s;
    const n = byId.get(id)!;
    const m = measures.get(id)!;
    if (!n.container) s = m.size;
    else {
      const fit = containerSize(n, m, (kids.get(id) ?? []).map((c) => ({ ...(rel.get(c) ?? { x: 0, y: 0 }), ...sizeOf(c) })), grid);
      if (fit.tooSmall) tooSmall.add(id);
      s = fit.size;
    }
    sizes.set(id, s);
    return s;
  };
  const policyAbove = (n: ViewNode): string | undefined => {
    for (let f = n.frame; f; f = byId.get(f)!.frame) if (byId.get(f)!.policy) return f;
    return undefined;
  };
  const within = (n: ViewNode, lanes: Set<string>) => {
    for (let f = n.frame; f; f = byId.get(f)!.frame) if (lanes.has(f)) return true;
    return false;
  };
  const run = async (ids: Set<string>, direction: ResolvedView['direction']) => {
    const sub = subView(view, byId, measures, ids, sizes, direction);
    const pos = elkPositions(await elk.layout(elkGraph(sub.view, sub.measures, labels)), sub.measures, grid);
    for (const [id, p] of pos) rel.set(id, p);
  };

  const frames = view.nodes.filter((n) => n.policy).sort((a, b) => depth(byId, b) - depth(byId, a) || cmp(a.id, b.id)).map((n) => n.id);
  for (const frame of frames) {
    const p = byId.get(frame)!.policy!;
    const lanes = p.order.filter((id) => byId.get(id)!.container && !byId.get(id)!.policy);
    const stack = p.layout === 'stack';
    const vertical = p.direction === 'vertical';
    for (const group of stack ? [lanes] : lanes.map((l) => [l])) {
      const set = new Set(group);
      const ids = new Set(view.nodes.filter((n) => policyAbove(n) === frame && within(n, set)).map((n) => n.id));
      if (ids.size === 0) continue;
      const across = vertical ? (view.direction === 'left' ? 'left' : 'right') : view.direction === 'up' ? 'up' : 'down';
      await run(ids, stack ? across : view.direction);
      const tops = [...ids].filter((id) => set.has(byId.get(id)!.frame!)).sort(cmp);
      if (stack) pack(tops, byId, rel, sizeOf, vertical, grid);
      else start(tops, rel, ['x', 'y']);
    }
    for (const id of p.order) sizeOf(id);
    arrangeFrame(byId, measures, frame, rel, sizes);
    sizeOf(frame);
  }
  await run(new Set(view.nodes.filter((n) => !policyAbove(n)).map((n) => n.id)), view.direction);
  for (const n of view.nodes) sizeOf(n.id);
  return { rel, sizes, tooSmall };
}

// start moves the nodes ids so that they start at 0 along each axis.
function start(ids: string[], rel: Map<string, Point>, axes: ('x' | 'y')[]) {
  for (const a of axes) {
    const min = Math.min(...ids.map((id) => rel.get(id)![a]));
    for (const id of ids) rel.set(id, { ...rel.get(id)!, [a]: rel.get(id)![a] - min });
  }
}

// pack lines up the children of the lanes of a stack laid out together: across the stack all
// start where the first starts; along it, each lane's fill rows from its start, in their order
// along the stack, each in the first row where it keeps the spacing from the others across the
// stack, centered in its row on the grid, and the rows elk's node spacing apart.
function pack(ids: string[], byId: Map<string, ViewNode>, rel: Map<string, Point>, sizeOf: (id: string) => Size, vertical: boolean, grid: number) {
  const spacing = ceilTo(NODE_SPACING, grid);
  const [along, across, alongSize, acrossSize] = vertical ? (['y', 'x', 'h', 'w'] as const) : (['x', 'y', 'w', 'h'] as const);
  start(ids, rel, [across]);
  const byLane = new Map<string, string[]>();
  for (const id of ids) byLane.set(byId.get(id)!.frame!, [...(byLane.get(byId.get(id)!.frame!) ?? []), id]);
  for (const lane of byLane.values()) {
    lane.sort((a, b) => rel.get(a)![along] - rel.get(b)![along] || rel.get(a)![across] - rel.get(b)![across] || cmp(a, b));
    const rows: string[][] = [];
    const clear = (a: string, b: string) => rel.get(a)![across] >= rel.get(b)![across] + sizeOf(b)[acrossSize] + spacing || rel.get(b)![across] >= rel.get(a)![across] + sizeOf(a)[acrossSize] + spacing;
    for (const id of lane) {
      const row = rows.find((r) => r.every((o) => clear(id, o)));
      if (row) row.push(id);
      else rows.push([id]);
    }
    let at = 0;
    for (const row of rows) {
      const thick = Math.max(...row.map((id) => sizeOf(id)[alongSize]));
      for (const id of row) rel.set(id, { ...rel.get(id)!, [along]: at + roundTo((thick - sizeOf(id)[alongSize]) / 2, grid) });
      at += thick + spacing;
    }
  }
}

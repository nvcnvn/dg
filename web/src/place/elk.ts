import type { ElkExtendedEdge, ElkNode } from 'elkjs/lib/elk-api';
import { cmp, roundTo, type NodeMeasure, type Point, type Size } from '../geometry/geometry';
import type { ResolvedView } from '../protocol/view';

export interface Elk {
  layout(graph: ElkNode): Promise<ElkNode>;
}

const ROOT = 'root';
const nodeKey = (id: string) => 'n:' + id;
const edgeKey = (id: string) => 'e:' + id;

const spacing: Record<string, string> = {
  'elk.spacing.nodeNode': '40',
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
  for (const e of view.edges) {
    const to = new Set(chain(e.to.node));
    const from = chain(e.from.node);
    const loop = e.from.node === e.to.node;
    if (loop ? byId.get(e.from.node)!.container : to.has(e.from.node) || from.includes(e.to.node)) continue;
    const lca = from.find((f) => to.has(f) && elkNodes.get(f)?.edges);
    const holder = (lca && elkNodes.get(lca)) || root;
    const up = e.style.arrow_end === 'triangle-open' && e.style.arrow_start !== 'triangle-open';
    const [a, b] = up ? [e.to.node, e.from.node] : [e.from.node, e.to.node];
    const edge: ElkExtendedEdge = { id: edgeKey(e.id), sources: [nodeKey(a)], targets: [nodeKey(b)] };
    const sizes = labels.get(e.id) ?? {};
    const elkLabels = (['mid', 'start', 'end'] as const)
      .filter((pos) => sizes[pos])
      .map((pos) => ({ id: `l:${pos}:${e.id}`, width: sizes[pos]!.w, height: sizes[pos]!.h, layoutOptions: { 'elk.edgeLabels.placement': placement[pos] } }));
    if (elkLabels.length) edge.labels = elkLabels;
    holder.edges!.push(edge);
  }
  return root;
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

export async function placeWithElk(elk: Elk, view: ResolvedView, measures: Map<string, NodeMeasure>, labels?: Map<string, EdgeLabelSizes>): Promise<Map<string, Point>> {
  return elkPositions(await elk.layout(elkGraph(view, measures, labels)), measures, view.grid);
}

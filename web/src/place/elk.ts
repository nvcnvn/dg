import type { ElkExtendedEdge, ElkNode } from 'elkjs/lib/elk-api';
import { CONTENT_ORIGIN, CONTAINER_PADDING, cmp, roundTo, type Point, type Size } from '../geometry/geometry';
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

const rootOptions: Record<string, string> = {
  ...spacing,
  'elk.algorithm': 'layered',
  'elk.direction': 'DOWN',
  'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
  'elk.edgeRouting': 'ORTHOGONAL',
  'elk.randomSeed': '1',
};

// elkGraph builds the elk input for a view with nothing pinned: nodes and
// edges in sorted ID order, containers holding their children, each edge in
// the innermost container holding both of its ends.
export function elkGraph(view: ResolvedView, sizes: Map<string, Size>): ElkNode {
  const elkNodes = new Map<string, ElkNode>();
  const root: ElkNode = { id: ROOT, layoutOptions: rootOptions, children: [], edges: [] };
  for (const n of view.nodes) {
    const size = sizes.get(n.id);
    const en: ElkNode = { id: nodeKey(n.id), width: size?.w, height: size?.h };
    if (n.container) {
      en.children = [];
      en.edges = [];
      en.layoutOptions = {
        ...spacing,
        'elk.padding': `[top=${CONTENT_ORIGIN.y},left=${CONTENT_ORIGIN.x},bottom=${CONTAINER_PADDING},right=${CONTAINER_PADDING}]`,
        'elk.nodeSize.constraints': '[MINIMUM_SIZE]',
        'elk.nodeSize.minimum': `(${size?.w ?? 0},${size?.h ?? 0})`,
      };
    }
    elkNodes.set(n.id, en);
  }
  for (const n of view.nodes) {
    const parent = (n.frame && elkNodes.get(n.frame)) || root;
    parent.children!.push(elkNodes.get(n.id)!);
  }

  const frameOf = new Map(view.nodes.map((n) => [n.id, n.frame]));
  const chain = (id: string): string[] => {
    const ids = [id];
    for (let f = frameOf.get(id); f; f = frameOf.get(f)) ids.push(f);
    return ids;
  };
  for (const e of view.edges) {
    const to = new Set(chain(e.to.node));
    const lca = chain(e.from.node).find((f) => to.has(f) && elkNodes.get(f)?.edges);
    const holder = (lca && elkNodes.get(lca)) || root;
    const edge: ElkExtendedEdge = { id: edgeKey(e.id), sources: [nodeKey(e.from.node)], targets: [nodeKey(e.to.node)] };
    holder.edges!.push(edge);
  }
  return root;
}

// elkPositions returns each node's position from an elk result, relative to
// its frame's content origin and rounded to the grid.
export function elkPositions(result: ElkNode, grid: number): Map<string, Point> {
  const out = new Map<string, Point>();
  const visit = (parent: ElkNode, inContainer: boolean) => {
    const children = [...(parent.children ?? [])].sort((a, b) => cmp(a.id, b.id));
    for (const c of children) {
      const dx = inContainer ? CONTENT_ORIGIN.x : 0;
      const dy = inContainer ? CONTENT_ORIGIN.y : 0;
      out.set(c.id.slice(2), { x: roundTo((c.x ?? 0) - dx, grid), y: roundTo((c.y ?? 0) - dy, grid) });
      visit(c, true);
    }
  };
  visit(result, false);
  return out;
}

export async function placeWithElk(elk: Elk, view: ResolvedView, sizes: Map<string, Size>): Promise<Map<string, Point>> {
  return elkPositions(await elk.layout(elkGraph(view, sizes)), view.grid);
}

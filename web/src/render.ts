import type { Drawing } from './canvas/export';
import type { DrawnEdge } from './canvas/markup';
import {
  LINE_HEIGHT,
  ceilTo,
  cmp,
  containerMinSize,
  depth,
  layoutBoxes,
  leafSize,
  type Box,
  type Point,
  type Size,
  type TextMeasure,
} from './geometry/geometry';
import { placeWithElk, type Elk } from './place/elk';
import type { ResolvedView } from './protocol/view';
import { clip, openArrow, pointAlong } from './route/route';

export type Router = (
  boxes: Map<string, Box>,
  ids: string[],
  obstacles: Set<string>,
  edges: { id: string; from: string; to: string }[],
) => Promise<Map<string, Point[]>>;

const MARGIN = 20;
const LABEL_PAD = 4;

// draw computes the geometry of a view with nothing pinned: sizes, the elk
// first layout, and routes.
export async function draw(view: ResolvedView, text: TextMeasure, elk: Elk, route: Router): Promise<Drawing> {
  const grid = view.grid;
  const leafSizes = new Map<string, Size>();
  const minSizes = new Map<string, Size>();
  for (const n of view.nodes) {
    if (n.container) minSizes.set(n.id, containerMinSize(n.label, text, grid));
    else leafSizes.set(n.id, leafSize(n.label, text, grid));
  }
  const rel = await placeWithElk(elk, view, new Map([...leafSizes, ...minSizes]));
  const boxes = layoutBoxes(view, leafSizes, minSizes, rel, grid);

  const ids = view.nodes.map((n) => n.id);
  const obstacles = new Set(view.nodes.filter((n) => !n.container).map((n) => n.id));
  const routes = await route(
    boxes,
    ids,
    obstacles,
    view.edges.map((e) => ({ id: e.id, from: e.from.node, to: e.to.node })),
  );

  const edges: DrawnEdge[] = view.edges.map((e) => {
    const points = clip(routes.get(e.id) ?? [], boxes.get(e.from.node)!, boxes.get(e.to.node)!);
    const drawn: DrawnEdge = { id: e.id, points, style: e.style };
    if (e.style.arrow_end === 'arrow') drawn.arrow = openArrow(points);
    if (e.labels.mid) {
      const c = pointAlong(points, 0.5);
      const w = ceilTo(text.width(e.labels.mid) + 2 * LABEL_PAD, grid);
      drawn.label = { text: e.labels.mid, box: { x: Math.round(c.x - w / 2), y: Math.round(c.y - LINE_HEIGHT / 2), w, h: LINE_HEIGHT } };
    }
    return drawn;
  });

  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const nodes = view.nodes
    .map((n) => ({ n, d: n.container ? depth(byId, n) : Infinity }))
    .sort((a, b) => a.d - b.d || cmp(a.n.id, b.n.id))
    .map(({ n }) => ({ id: n.id, box: boxes.get(n.id)!, label: n.label, container: !!n.container, style: n.style }));

  return { view: view.view, bounds: bounds(nodes.map((n) => n.box), edges, grid), nodes, edges, ascent: text.ascent, descent: text.descent };
}

function bounds(boxes: Box[], edges: DrawnEdge[], grid: number): Box {
  const all = [...boxes, ...edges.flatMap((e) => [...e.points.map((p) => ({ ...p, w: 0, h: 0 })), ...(e.label ? [e.label.box] : [])])];
  if (all.length === 0) return { x: 0, y: 0, w: 2 * MARGIN, h: 2 * MARGIN };
  const x = Math.floor((Math.min(...all.map((b) => b.x)) - MARGIN) / grid) * grid;
  const y = Math.floor((Math.min(...all.map((b) => b.y)) - MARGIN) / grid) * grid;
  const right = ceilTo(Math.max(...all.map((b) => b.x + b.w)) + MARGIN, grid);
  const bottom = ceilTo(Math.max(...all.map((b) => b.y + b.h)) + MARGIN, grid);
  return { x, y, w: right - x, h: bottom - y };
}

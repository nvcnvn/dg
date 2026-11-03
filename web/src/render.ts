import type { Drawing } from './canvas/export';
import type { DrawnEdge, DrawnNode } from './canvas/markup';
import { absoluteBoxes, ceilTo, cmp, contentLayout, depth, fitContainers, measureNode, type Box, type Layout, type NodeMeasure, type Point } from './geometry/geometry';
import { rowLines } from './geometry/table';
import { textBlock, type TextMeasure } from './geometry/text';
import { placeWithElk, type EdgeLabelSizes, type Elk } from './place/elk';
import { placeSpiral } from './place/spiral';
import type { ResolvedView, ViewNode } from './protocol/view';
import { LABEL_PAD, placeLabels, planPorts, routeEdges, type Router } from './route/edges';
import { marker } from './route/markers';
import type { PortPos } from './route/route';

const MARGIN = 20;

// draw computes the geometry of a view: sizes, placement, ports, routes and labels.
export async function draw(view: ResolvedView, text: TextMeasure, family: string, elk: Elk, router: Router): Promise<Drawing> {
  const grid = view.grid;
  const shapes = view.shapes ?? {};
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const measures = new Map(view.nodes.map((n) => [n.id, measureNode(n, text, grid, shapes[n.style.shape])]));
  const layout = await place(view, measures, text, elk);
  const boxes = absoluteBoxes(view, measures, layout);
  const headers = new Map(view.nodes.filter((n) => n.container && measures.get(n.id)!.content.h > 0).map((n) => [n.id, header(n, measures.get(n.id)!, boxes.get(n.id)!)]));
  const rows = new Map(view.nodes.filter((n) => measures.get(n.id)!.table).map((n) => [n.id, rowLines(measures.get(n.id)!.table!, boxes.get(n.id)!)]));
  const geometry = { view, boxes, headers, rows, grid };
  const ports = planPorts(geometry);
  const routes = await routeEdges(geometry, ports, router);
  const markers = new Map(view.edges.map((e) => [e.id, [marker(e.style.arrow_end, routes.get(e.id)!), marker(e.style.arrow_start, [...routes.get(e.id)!].reverse())]]));
  const labels = placeLabels(geometry, routes, markers, text);

  const nodes: DrawnNode[] = view.nodes
    .map((n) => ({ n, z: n.style.z, d: depth(byId, n) }))
    .sort((a, b) => a.z - b.z || a.d - b.d || cmp(a.n.id, b.n.id))
    .map(({ n }) => drawnNode(n, measures.get(n.id)!, boxes.get(n.id)!, layout, shapes[n.style.shape] ? n.style.shape : undefined, view, ports.get(n.id)!));
  const edges: DrawnEdge[] = view.edges.map((e) => {
    return { id: e.id, points: routes.get(e.id)!, style: e.style, markers: markers.get(e.id)!.flat(), labels: labels.get(e.id)! };
  });

  return {
    view: view.view,
    family,
    bounds: bounds(nodes.map((n) => n.box), edges, grid),
    nodes,
    edges,
    shapes,
    ascent: text.ascent,
    descent: text.descent,
  };
}

// place positions every node relative to its frame (spec 7.4): elk when nothing is pinned,
// otherwise the spiral placer. A node pinned in a stale frame that is still drawn is first
// placed as if unpinned; its stored position is then converted to its current frame and the
// view placed again with it pinned there (spec 7.1).
async function place(view: ResolvedView, measures: Map<string, NodeMeasure>, text: TextMeasure, elk: Elk): Promise<Layout> {
  if (!view.nodes.some((n) => n.pin)) return fitContainers(view, measures, await placeWithElk(elk, view, measures, edgeLabelSizes(view, text)), view.grid);
  const pins = new Map(view.nodes.filter((n) => n.pin && n.pin.in === undefined).map((n) => [n.id, { x: n.pin!.x, y: n.pin!.y }]));
  const layout = placeSpiral(view, measures, pins, view.grid);
  const stale = view.nodes.filter((n) => n.pin?.in !== undefined);
  if (stale.length === 0) return layout;
  const boxes = absoluteBoxes(view, measures, layout);
  const originOf = (id: string | undefined): Point => {
    if (!id) return { x: 0, y: 0 };
    const b = boxes.get(id)!;
    const o = measures.get(id)!.origin;
    return { x: b.x + o.x, y: b.y + o.y };
  };
  for (const n of stale) {
    const from = originOf(n.pin!.in);
    const to = originOf(n.frame);
    pins.set(n.id, { x: from.x + n.pin!.x - to.x, y: from.y + n.pin!.y - to.y });
  }
  return placeSpiral(view, measures, pins, view.grid);
}

function drawnNode(n: ViewNode, m: NodeMeasure, box: Box, layout: Layout, custom: string | undefined, view: ResolvedView, ports: Map<string, PortPos>): DrawnNode {
  const c = contentLayout(n, m, box, custom ? view.shapes![custom] : undefined);
  const t = m.table;
  const table = t && {
    lines: t.lines.map((y) => Math.round(box.y + y)),
    rows: t.rows.map((r) => ({ ...r, y: box.y + r.y })),
    keyX: box.x + n.style.padding,
    nameX: box.x + t.nameX,
    typeX: box.x + box.w - n.style.padding,
    font: t.font,
    keyFont: t.keyFont,
  };
  return {
    id: n.id,
    box,
    container: !!n.container,
    pinned: !!n.pin,
    tooSmall: layout.tooSmall.has(n.id),
    style: n.style,
    custom,
    text: m.text,
    ...c,
    ports: [...ports].map(([name, p]) => ({ name, ...p })),
    table,
  };
}

// header returns the box of a container's label and icon.
function header(n: ViewNode, m: NodeMeasure, b: Box): Box {
  const c = contentLayout(n, m, b);
  return { x: c.icon?.x ?? c.textBox.x, y: c.icon?.y ?? c.textBox.y, w: m.content.w, h: m.content.h };
}

function bounds(boxes: Box[], edges: DrawnEdge[], grid: number): Box {
  const all = [...boxes, ...edges.flatMap((e) => [...e.points.map((p) => ({ ...p, w: 0, h: 0 })), ...e.labels.map((l) => l.box)])];
  if (all.length === 0) return { x: 0, y: 0, w: 2 * MARGIN, h: 2 * MARGIN };
  const x = Math.floor((Math.min(...all.map((b) => b.x)) - MARGIN) / grid) * grid;
  const y = Math.floor((Math.min(...all.map((b) => b.y)) - MARGIN) / grid) * grid;
  const right = ceilTo(Math.max(...all.map((b) => b.x + b.w)) + MARGIN, grid);
  const bottom = ceilTo(Math.max(...all.map((b) => b.y + b.h)) + MARGIN, grid);
  return { x, y, w: right - x, h: bottom - y };
}


// edgeLabelSizes returns the sizes of each edge's labels by position, so the first layout leaves
// room for them.
function edgeLabelSizes(view: ResolvedView, text: TextMeasure): Map<string, EdgeLabelSizes> {
  const out = new Map<string, EdgeLabelSizes>();
  for (const e of view.edges) {
    const sizes: EdgeLabelSizes = {};
    for (const pos of ['start', 'mid', 'end'] as const) {
      const s = e.labels[pos];
      if (!s) continue;
      const t = textBlock(s, 0, { size: e.style.font_size, weight: 400 }, text);
      sizes[pos] = { w: ceilTo(t.w + 2 * LABEL_PAD, view.grid), h: t.h };
    }
    out.set(e.id, sizes);
  }
  return out;
}

import { FONT_SIZE, cmp, type Box } from '../geometry/geometry';
import { attrs, edgeMarkup, nodeMarkup, num, type DrawnEdge, type DrawnNode, type FontMetrics } from './markup';
import { FONT_FAMILY } from './measure';

// Drawing is a view with all geometry computed, in draw order.
export interface Drawing extends FontMetrics {
  view: string;
  bounds: Box;
  nodes: DrawnNode[];
  edges: DrawnEdge[];
}

export function toSVG(d: Drawing): string {
  const b = d.bounds;
  const root = attrs({
    viewBox: `${num(b.x)} ${num(b.y)} ${num(b.w)} ${num(b.h)}`,
    width: b.w,
    height: b.h,
    'font-family': `${FONT_FAMILY}, sans-serif`,
    'font-size': FONT_SIZE,
  });
  return [
    `<svg xmlns="http://www.w3.org/2000/svg"${root}>`,
    ...d.nodes.flatMap((n) => nodeMarkup(n, d)),
    ...d.edges.flatMap((e) => edgeMarkup(e, d)),
    '</svg>',
  ].join('\n') + '\n';
}

// toJSON returns every node's box and edge route in canvas coordinates,
// sorted by ID, with each point on its own line.
export function toJSON(d: Drawing): string {
  const nodes = [...d.nodes].sort((a, b) => cmp(a.id, b.id)).map((n) => ({ id: n.id, ...n.box }));
  const edges = [...d.edges]
    .sort((a, b) => cmp(a.id, b.id))
    .map((e) => ({ id: e.id, points: e.points.map((p) => [p.x, p.y]) }));
  const json = JSON.stringify({ view: d.view, nodes, edges }, null, 2);
  return json.replace(/\[\s+(-?[\d.e+-]+),\s+(-?[\d.e+-]+)\s+\]/g, '[$1, $2]') + '\n';
}

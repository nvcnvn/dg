import { cmp, type Box } from '../geometry/geometry';
import { attrs, defsMarkup, edgeMarkup, nodeMarkup, num, type DrawnEdge, type DrawnNode, type FontMetrics, type Shapes } from './markup';

// Drawing is a view with all geometry computed, nodes in draw order. family names the font
// an exported SVG asks for.
export interface Drawing extends FontMetrics, Shapes {
  view: string;
  family: string;
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
    'font-family': `${d.family}, sans-serif`,
  });
  const defs = defsMarkup(d.nodes, d);
  return (
    [
      `<svg xmlns="http://www.w3.org/2000/svg"${root}>`,
      ...(defs.length ? ['<defs>', ...defs, '</defs>'] : []),
      ...d.nodes.flatMap((n) => nodeMarkup(n, d, d)),
      ...d.edges.flatMap((e) => edgeMarkup(e, d)),
      '</svg>',
    ].join('\n') + '\n'
  );
}

// toJSON returns every node's box and port positions and every edge route in canvas
// coordinates, sorted by ID, with each point on its own line.
export function toJSON(d: Drawing): string {
  const nodes = [...d.nodes]
    .sort((a, b) => cmp(a.id, b.id))
    .map((n) => {
      const ports = [...n.ports].sort((a, b) => cmp(a.name, b.name)).map((p) => ({ name: p.name, side: p.side, x: Math.round(p.x), y: Math.round(p.y) }));
      return { id: n.id, ...n.box, ...(ports.length ? { ports } : {}) };
    });
  const edges = [...d.edges].sort((a, b) => cmp(a.id, b.id)).map((e) => ({ id: e.id, points: e.points.map((p) => [p.x, p.y]) }));
  const json = JSON.stringify({ view: d.view, nodes, edges }, null, 2);
  return json.replace(/\[\s+(-?[\d.e+-]+),\s+(-?[\d.e+-]+)\s+\]/g, '[$1, $2]') + '\n';
}

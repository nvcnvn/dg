import { cmp, type Box } from '../geometry/geometry';
import { attrs, decorationMarkup, defsMarkup, edgeMarkup, flowCSS, flowPeriods, nodeMarkup, num, overlayMarkup, type DrawnDecoration, type DrawnEdge, type DrawnNode, type DrawnOverlay, type FontMetrics, type Shapes } from './markup';

// Drawing is a view with all geometry computed, nodes in draw order and decorations in layout
// file order. family names the font an exported SVG asks for.
export interface Drawing extends FontMetrics, Shapes {
  view: string;
  family: string;
  bounds: Box;
  nodes: DrawnNode[];
  edges: DrawnEdge[];
  overlays: DrawnOverlay[];
  decorations: DrawnDecoration[];
}

// Layer is one part of a drawing in draw order (spec 5.7, 5.8): decorations with a negative z
// behind everything, then expanded containers, overlays, the other nodes and the edges, then
// the other decorations; decorations in order of z, then layout file order, and nodes in their
// drawing order.
export type Layer = { decoration: DrawnDecoration } | { overlay: DrawnOverlay } | { node: DrawnNode } | { edge: DrawnEdge };

export function layers(d: Drawing): Layer[] {
  const byZ = (list: DrawnDecoration[]) => list.map((dec, i) => ({ dec, i })).sort((a, b) => a.dec.z - b.dec.z || a.i - b.i).map(({ dec }) => ({ decoration: dec }));
  return [
    ...byZ(d.decorations.filter((x) => x.z < 0)),
    ...d.nodes.filter((n) => n.container).map((node) => ({ node })),
    ...d.overlays.map((overlay) => ({ overlay })),
    ...d.nodes.filter((n) => !n.container).map((node) => ({ node })),
    ...d.edges.map((edge) => ({ edge })),
    ...byZ(d.decorations.filter((x) => x.z >= 0)),
  ];
}

// layerMarkup draws one layer; periods are the drawing's flowPeriods.
export function layerMarkup(l: Layer, d: Drawing, periods: number[]): string[] {
  if ('decoration' in l) return decorationMarkup(l.decoration, d);
  if ('overlay' in l) return overlayMarkup(l.overlay, d);
  if ('node' in l) return nodeMarkup(l.node, d, d);
  return edgeMarkup(l.edge, d, periods);
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
  const periods = flowPeriods(d.edges);
  const css = flowCSS(periods);
  return (
    [
      `<svg xmlns="http://www.w3.org/2000/svg"${root}>`,
      ...(css ? [`<style>\n${css}</style>`] : []),
      ...(defs.length ? ['<defs>', ...defs, '</defs>'] : []),
      ...layers(d).flatMap((l) => layerMarkup(l, d, periods)),
      '</svg>',
    ].join('\n') + '\n'
  );
}

// toJSON returns every node's box, port positions and row lines (the y both ends of a row's line
// are at) and every edge route in canvas coordinates, sorted by ID, with each point on its own
// line; and every node's frame and position in it, as a move operation takes them, or, for a
// node a stack or grid container places, which a move cannot, that container (ordered_in).
export function toJSON(d: Drawing): string {
  const nodes = [...d.nodes]
    .sort((a, b) => cmp(a.id, b.id))
    .map((n) => {
      const ports = [...n.ports].sort((a, b) => cmp(a.name, b.name)).map((p) => ({ name: p.name, side: p.side, x: Math.round(p.x), y: Math.round(p.y) }));
      const rows = n.table?.rows.map((r) => ({ name: r.name, y: Math.round(r.y) })) ?? [];
      const frame = n.rel ? { ...(n.frame ? { frame: n.frame } : {}), rel: [n.rel.x, n.rel.y] } : {};
      return { id: n.id, ...n.box, ...frame, ...(n.orderedIn !== undefined ? { ordered_in: n.orderedIn } : {}), ...(ports.length ? { ports } : {}), ...(rows.length ? { rows } : {}) };
    });
  const edges = [...d.edges].sort((a, b) => cmp(a.id, b.id)).map((e) => ({ id: e.id, points: e.points.map((p) => [p.x, p.y]) }));
  const json = JSON.stringify({ view: d.view, nodes, edges }, null, 2);
  return json.replace(/\[\s+(-?[\d.e+-]+),\s+(-?[\d.e+-]+)\s+\]/g, '[$1, $2]') + '\n';
}

// checkStyle throws unless the page applied style, which a content security policy can refuse.
export function checkStyle(style: HTMLStyleElement | SVGStyleElement | null) {
  if (style && !style.sheet) throw new Error('the page refused the edge animation style');
}

// mountSVG shows svg alone on white at the page's top left, text in family, paused at time 0, once its pictures load.
export async function mountSVG(svg: string, family: string): Promise<void> {
  const doc = new DOMParser().parseFromString(svg, 'image/svg+xml');
  const el = document.importNode(doc.documentElement, true);
  el.setAttribute('font-family', `${family}, sans-serif`);
  let host = document.getElementById('export');
  if (!host) {
    host = document.createElement('div');
    host.id = 'export';
    document.body.append(host);
  }
  host.replaceChildren(el);
  checkStyle(el.querySelector('style'));
  await Promise.all(
    [...el.querySelectorAll('image')].map(
      (im) =>
        new Promise((done, fail) => {
          im.addEventListener('load', done);
          im.addEventListener('error', () => {
            const node = im.closest('[data-node]')?.getAttribute('data-node');
            const decoration = im.closest('[data-decoration]')?.getAttribute('data-decoration');
            const what = node != null ? `node "${node}"` : decoration != null ? `decoration "${decoration}"` : `shape ${im.id.replace('dg-shape-', '')}`;
            fail(new Error(`the picture of ${what} did not load`));
          });
        }),
    ),
  );
  await setClock(0);
}

// setClock pauses every animation of the page at ms milliseconds and waits for that frame to be drawn.
export async function setClock(ms: number): Promise<void> {
  for (const a of document.getAnimations()) {
    a.pause();
    a.currentTime = ms;
  }
  await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)));
}

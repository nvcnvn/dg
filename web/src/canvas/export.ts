import { cmp, type Box } from '../geometry/geometry';
import { attrs, decorationMarkup, defsMarkup, edgeMarkup, flowCSS, flowPeriods, nodeMarkup, num, overlayMarkup, type DrawnDecoration, type DrawnEdge, type DrawnNode, type DrawnOverlay, type FontMetrics, type Shapes } from './markup';

const REVOKE_MS = 60_000;

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

// picture is d as an export shows it: a drill-down badge is a control of the editor, so a picture
// keeps only the collapse badge.
export function picture(d: Drawing): Drawing {
  return { ...d, nodes: d.nodes.map(({ drill: _, ...n }) => n) };
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

// fontFaces is CSS giving the SVG of a drawing, drawn as a picture, which sees none of the page's
// fonts, the font of its text: the project's, or every @font-face of the bundled family.
async function fontFaces(family: string, font?: { family: string; data: string }): Promise<string> {
  if (font) return `@font-face { font-family: "${font.family}"; src: url(data:font/ttf;base64,${font.data}); }`;
  const rules: string[] = [];
  for (const sheet of document.styleSheets) {
    for (const rule of sheet.cssRules) {
      if (!(rule instanceof CSSFontFaceRule) || !rule.style.getPropertyValue('font-family').includes(family)) continue;
      const at = /url\("?([^")]+)"?\)/.exec(rule.cssText);
      if (!at) continue;
      const bytes = await fetch(new URL(at[1], sheet.href ?? location.href));
      rules.push(rule.cssText.replace(at[0], `url(${await dataURL(await bytes.blob())})`));
    }
  }
  return rules.join('\n');
}

function dataURL(b: Blob): Promise<string> {
  return new Promise((done, fail) => {
    const r = new FileReader();
    r.onload = () => done(r.result as string);
    r.onerror = () => fail(r.error);
    r.readAsDataURL(b);
  });
}

// toPNG draws d, one pixel per unit on white, as dg export --format png does (spec 15).
export async function toPNG(d: Drawing, font?: { family: string; data: string }): Promise<Blob> {
  const css = await fontFaces(d.family, font);
  const svg = toSVG(d).replace(/^(<svg[^>]*>)/, `$1\n<style>\n${css}\n</style>`);
  const image = new Image();
  image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
  await image.decode();
  const canvas = document.createElement('canvas');
  canvas.width = Math.ceil(d.bounds.w);
  canvas.height = Math.ceil(d.bounds.h);
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('no 2D canvas context');
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(image, 0, 0, canvas.width, canvas.height);
  return new Promise((done, fail) => canvas.toBlob((b) => (b ? done(b) : fail(new Error('the canvas gave no PNG'))), 'image/png'));
}

// saveBlob downloads blob as a file called name.
export function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  // Safari can still be reading the URL after click returns; revoking at once loses the download.
  setTimeout(() => URL.revokeObjectURL(url), REVOKE_MS);
}

// mountSVG shows svg alone on white at the page's top left, text in family, paused at time 0, once its pictures load.
export async function mountSVG(svg: string, family: string): Promise<void> {
  const doc = new DOMParser().parseFromString(svg, 'image/svg+xml');
  const el = document.importNode(doc.documentElement, true);
  el.setAttribute('font-family', `${family}, sans-serif`);
  const host = document.createElement('div');
  host.id = 'export';
  host.append(el);
  document.body.replaceChildren(host);
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

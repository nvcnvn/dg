import type { Box, Point } from '../geometry/geometry';
import { CYLINDER_RY, DOCUMENT_WAVE, LABELLESS, outline, personBodyTop, personHead, svgSize } from '../geometry/shapes';
import { lineHeight, type Font, type TextBlock } from '../geometry/text';
import type { CustomShape, EdgeStyle, NodeStyle, OverlayStyle, Side, ViewDecoration } from '../protocol/view';
import type { PlacedLabel } from '../route/edges';
import type { MarkerPart } from '../route/markers';

// Markup for one node or edge from its geometry. The SVG export and the editor canvas both
// draw with these functions, so there is one drawing path (spec 15).

export interface DrawnNode {
  id: string;
  box: Box;
  // frame is the node's frame, and rel its drawn position relative to the frame's content origin
  // (spec 7.1), the coordinates a move operation takes.
  frame?: string;
  rel?: Point;
  container: boolean;
  // origin is a container's content origin in canvas coordinates.
  origin?: Point;
  pinned: boolean;
  // orderedIn is the stack or grid container that places the node, which a move cannot.
  orderedIn?: string;
  tooSmall: boolean;
  style: NodeStyle;
  custom?: string;
  text: TextBlock | null;
  textBox: Box;
  align: 'center' | 'left';
  icon?: Box;
  picture?: Box;
  ports: DrawnPort[];
  table?: DrawnTable;
  // badge is a collapsed container's "+" badge.
  badge?: Box;
  // drill is the drill-down badge of a node other views show the inside of.
  drill?: Box;
  // rotate turns the label and icon a quarter counterclockwise around it; divider is the x of
  // the line between a left label and the content.
  rotate?: Point;
  divider?: number;
}

// DrawnOverlay is an overlay's outline (SVG path data) and its label, left-aligned above it.
export interface DrawnOverlay {
  id: string;
  path: string;
  box: Box;
  label?: { text: TextBlock; box: Box };
  style: OverlayStyle;
}

// DrawnDecoration is a decoration in canvas coordinates: the box it covers, its text, an
// arrow's points with its marker, or the offset of a freehand path.
export interface DrawnDecoration {
  id: string;
  type: ViewDecoration['type'];
  z: number;
  box: Box;
  text?: TextBlock;
  points?: Point[];
  marker?: MarkerPart[];
  origin: Point;
  path?: string;
  image?: string;
  style: Pick<ViewDecoration, 'fill' | 'stroke' | 'stroke_width' | 'stroke_dash' | 'opacity' | 'text_color'>;
}

// DrawnTable is the rows of a table or class node: separator lines across the box at each y in
// lines, and each row's key at keyX, name at nameX and type ending at typeX, centered on its y.
export interface DrawnTable {
  lines: number[];
  rows: { name: string; key?: string; type?: string; y: number }[];
  keyX: number;
  nameX: number;
  typeX: number;
  font: Font;
  keyFont: Font;
}

export interface DrawnPort extends Point {
  name: string;
  side: Side;
}

export interface DrawnEdge {
  id: string;
  points: Point[];
  markers: MarkerPart[];
  labels: PlacedLabel[];
  style: EdgeStyle;
}

// FontMetrics are the ascent and descent of the drawing font as fractions of its size.
export interface FontMetrics {
  ascent: number;
  descent: number;
}

export interface Shapes {
  shapes: Record<string, CustomShape>;
}

const LABEL_BACKGROUND = '#FFFFFF';
const WARNING = '#D93025';
const BADGE_STROKE = '#3C4043';
export const LABEL_PAD = 4;

// num writes a coordinate as an integer, so output does not depend on sub-pixel differences in
// text shaping.
export function num(v: number): string {
  const r = Math.round(v);
  return String(r === 0 ? 0 : r);
}

// dec writes a width or a fraction with at most two decimals.
export function dec(v: number): string {
  const r = Math.round(v * 100) / 100;
  return String(r === 0 ? 0 : r);
}

export function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

export function attrs(a: Record<string, string | number | undefined>): string {
  return Object.entries(a)
    .filter(([, v]) => v !== undefined)
    .map(([k, v]) => ` ${k}="${typeof v === 'number' ? num(v) : esc(v!)}"`)
    .join('');
}

function path(pts: Point[]): string {
  return pts.map((p, i) => `${i ? 'L' : 'M'}${num(p.x)} ${num(p.y)}`).join(' ');
}

function dash(d: number[]): string | undefined {
  return d.length ? d.map(dec).join(' ') : undefined;
}

export function dataURI(svg: string): string {
  return 'data:image/svg+xml,' + encodeURIComponent(svg);
}

function textLines(t: TextBlock, b: Box, align: 'center' | 'left', color: string, m: FontMetrics): string[] {
  const lh = lineHeight(t.font.size);
  const top = b.y + (b.h - t.h) / 2;
  return t.lines.map((line, i) => textLine(line, align === 'center' ? b.x + b.w / 2 : b.x, top + i * lh + lh / 2, align === 'center' ? 'middle' : undefined, t.font, color, m));
}

// textLine draws one line of text whose middle is at y.
function textLine(line: string, x: number, y: number, anchor: 'middle' | 'end' | undefined, f: Font, color: string, m: FontMetrics): string {
  const a = attrs({
    x,
    y: y + ((m.ascent - m.descent) * f.size) / 2,
    'text-anchor': anchor,
    fill: color,
    'font-size': dec(f.size),
    'font-weight': f.weight === 400 ? undefined : String(f.weight),
  });
  return `<text${a}>${esc(line)}</text>`;
}

function tableMarkup(n: DrawnNode, t: DrawnTable, m: FontMetrics): string[] {
  const st = n.style;
  const out = t.lines.map((y) => `<path${attrs({ d: path([{ x: n.box.x, y }, { x: n.box.x + n.box.w, y }]), stroke: st.stroke, 'stroke-width': dec(st.stroke_width) })}/>`);
  for (const r of t.rows) {
    if (r.key) out.push(textLine(r.key, t.keyX, r.y, undefined, t.keyFont, st.text_color, m));
    out.push(textLine(r.name, t.nameX, r.y, undefined, t.font, st.text_color, m));
    if (r.type) out.push(textLine(r.type, t.typeX, r.y, 'end', t.font, st.text_color, m));
  }
  return out;
}

function picture(svg: string, b: Box, fit: 'meet' | 'none'): string {
  return `<image${attrs({ x: b.x, y: b.y, width: b.w, height: b.h, preserveAspectRatio: fit === 'meet' ? 'xMidYMid meet' : 'none', href: dataURI(svg) })}/>`;
}

function outlineMarkup(n: DrawnNode, s: Shapes): string[] {
  const { x, y, w, h } = n.box;
  const st = n.style;
  const paint = { fill: st.fill, stroke: st.stroke, 'stroke-width': dec(st.stroke_width), 'stroke-dasharray': dash(st.stroke_dash) };
  const rect = (rx: number) => `<rect${attrs({ x, y, width: w, height: h, rx: rx > 0 ? dec(rx) : undefined, ...paint })}/>`;
  if (n.custom) return customMarkup(n.custom, s.shapes[n.custom], n.box);
  switch (st.shape) {
    case 'rounded':
      return [rect(Math.min(st.corner_radius, w / 2, h / 2))];
    case 'stadium':
      return [rect(h / 2)];
    case 'ellipse':
      return [`<ellipse${attrs({ cx: x + w / 2, cy: y + h / 2, rx: w / 2, ry: h / 2, ...paint })}/>`];
    case 'diamond':
    case 'hexagon':
    case 'parallelogram':
      return [`<path${attrs({ d: path(outline(st.shape, n.box)) + ' Z', ...paint })}/>`];
    case 'cylinder': {
      const ry = CYLINDER_RY;
      const rx = w / 2;
      const body = `M${num(x)} ${num(y + ry)} L${num(x)} ${num(y + h - ry)} A${num(rx)} ${ry} 0 0 0 ${num(x + w)} ${num(y + h - ry)} L${num(x + w)} ${num(y + ry)} A${num(rx)} ${ry} 0 0 0 ${num(x)} ${num(y + ry)} Z`;
      const rim = `M${num(x)} ${num(y + ry)} A${num(rx)} ${ry} 0 0 0 ${num(x + w)} ${num(y + ry)}`;
      return [`<path${attrs({ d: body, ...paint })}/>`, `<path${attrs({ d: rim, ...paint, fill: 'none' })}/>`];
    }
    case 'document': {
      const b = y + h - DOCUMENT_WAVE;
      const d = `M${num(x)} ${num(y)} L${num(x + w)} ${num(y)} L${num(x + w)} ${num(b)} Q${num(x + 0.75 * w)} ${num(b - 2 * DOCUMENT_WAVE)} ${num(x + w / 2)} ${num(b)} T${num(x)} ${num(b)} Z`;
      return [`<path${attrs({ d, ...paint })}/>`];
    }
    case 'person': {
      const head = personHead(n.box);
      const top = personBodyTop(n.box);
      const body = `<rect${attrs({ x, y: top, width: w, height: y + h - top, rx: dec(Math.min(head / 2, w / 2, (y + h - top) / 2)), ...paint })}/>`;
      return [body, `<circle${attrs({ cx: x + w / 2, cy: y + head / 2, r: dec(head / 2), ...paint })}/>`];
    }
    case 'cloud': {
      const p = (fx: number, fy: number) => `${num(x + fx * w)} ${num(y + fy * h)}`;
      const d =
        `M${p(0.25, 0.25)} C${p(0.05, 0.25)} ${p(0, 0.5)} ${p(0.16, 0.55)} C${p(0, 0.6)} ${p(0.02, 0.9)} ${p(0.2, 0.8)} ` +
        `C${p(0.25, 1)} ${p(0.7, 1)} ${p(0.75, 0.8)} C${p(1, 0.8)} ${p(1, 0.6)} ${p(0.875, 0.5)} ` +
        `C${p(1, 0.3)} ${p(0.8, 0.1)} ${p(0.625, 0.2)} C${p(0.5, 0.05)} ${p(0.3, 0.05)} ${p(0.25, 0.25)} Z`;
      return [`<path${attrs({ d, ...paint })}/>`];
    }
    case 'point':
      return [`<circle${attrs({ cx: x + w / 2, cy: y + h / 2, r: dec(Math.min(w, h) / 2), ...paint })}/>`];
    case 'bullseye': {
      const r = Math.min(w, h) / 2;
      return [
        `<circle${attrs({ cx: x + w / 2, cy: y + h / 2, r: dec(r), ...paint, fill: 'none' })}/>`,
        `<circle${attrs({ cx: x + w / 2, cy: y + h / 2, r: dec(r * 0.6), fill: st.fill })}/>`,
      ];
    }
    case 'text':
    case 'image':
      return [];
  }
  return [rect(0)];
}

// customMarkup draws a custom shape's picture in box b: scaled to fit (uniform), at its natural
// size (none), or in nine slices whose corners keep their natural size and whose edges and
// center stretch (nine-slice, cut at a quarter of the picture's width and height).
function customMarkup(name: string, sh: CustomShape, b: Box): string[] {
  const nat = svgSize(sh.svg);
  if (sh.stretch === 'uniform') return [picture(sh.svg, b, 'meet')];
  if (sh.stretch === 'none') return [picture(sh.svg, { x: b.x + (b.w - nat.w) / 2, y: b.y + (b.h - nat.h) / 2, ...nat }, 'none')];
  const k = Math.min(1, b.w / (nat.w / 2), b.h / (nat.h / 2));
  const cols = slices(nat.w, b.x, b.w, (nat.w / 4) * k);
  const rows = slices(nat.h, b.y, b.h, (nat.h / 4) * k);
  const out: string[] = [];
  for (const r of rows) {
    for (const c of cols) {
      const a = attrs({ x: c.to, y: r.to, width: c.size, height: r.size, viewBox: `${dec(c.from)} ${dec(r.from)} ${dec(c.span)} ${dec(r.span)}`, preserveAspectRatio: 'none' });
      out.push(`<svg${a}><use href="#${shapeID(name)}"/></svg>`);
    }
  }
  return out;
}

function slices(natural: number, at: number, size: number, corner: number) {
  const q = natural / 4;
  return [
    { from: 0, span: q, to: at, size: corner },
    { from: q, span: natural - 2 * q, to: at + corner, size: size - 2 * corner },
    { from: natural - q, span: q, to: at + size - corner, size: corner },
  ];
}

function shapeID(name: string): string {
  return 'dg-shape-' + name;
}

// defsMarkup defines the pictures of the nine-slice custom shapes the nodes use.
export function defsMarkup(nodes: DrawnNode[], s: Shapes): string[] {
  const names = [...new Set(nodes.map((n) => n.custom).filter((c): c is string => !!c && s.shapes[c].stretch === 'nine-slice'))].sort();
  return names.map((name) => {
    const nat = svgSize(s.shapes[name].svg);
    return `<image${attrs({ id: shapeID(name), width: dec(nat.w), height: dec(nat.h), preserveAspectRatio: 'none', href: dataURI(s.shapes[name].svg) })}/>`;
  });
}

export function nodeMarkup(n: DrawnNode, m: FontMetrics, s: Shapes): string[] {
  const st = n.style;
  const out = [`<g${attrs({ 'data-node': n.id || undefined, opacity: st.opacity < 1 ? dec(st.opacity) : undefined })}>`, ...outlineMarkup(n, s)];
  if (n.divider !== undefined) out.push(`<path${attrs({ d: path([{ x: n.divider, y: n.box.y }, { x: n.divider, y: n.box.y + n.box.h }]), stroke: st.stroke, 'stroke-width': dec(st.stroke_width) })}/>`);
  if (n.picture && st.image) out.push(picture(st.image, n.picture, 'meet'));
  if (n.rotate) out.push(`<g transform="rotate(-90 ${num(n.rotate.x)} ${num(n.rotate.y)})">`);
  if (n.icon && st.icon) out.push(picture(st.icon, n.icon, 'meet'));
  if (n.text) out.push(...textLines(n.text, n.textBox, n.align, st.text_color, m));
  if (n.rotate) out.push('</g>');
  if (n.table) out.push(...tableMarkup(n, n.table, m));
  if (n.badge) out.push(...collapseBadge(n.badge));
  if (n.drill) out.push(...drillBadge(n.id, n.drill));
  if (n.tooSmall) {
    const b = n.box;
    out.push(`<rect${attrs({ x: b.x - 3, y: b.y - 3, width: b.w + 6, height: b.h + 6, fill: 'none', stroke: WARNING, 'stroke-dasharray': '4 4' })}/>`);
  }
  out.push('</g>');
  return out;
}

// collapseBadge draws a "+" in a box: the container holds more than it shows.
function collapseBadge(b: Box): string[] {
  const { x, y, w, h } = b;
  return [
    `<g data-collapsed="">`,
    `<rect${attrs({ x, y, width: w, height: h, rx: 2, fill: LABEL_BACKGROUND, stroke: BADGE_STROKE })}/>`,
    `<path${attrs({ d: `M${num(x + 3)} ${num(y + h / 2)} H${num(x + w - 3)} M${num(x + w / 2)} ${num(y + 3)} V${num(y + h - 3)}`, stroke: BADGE_STROKE, 'stroke-width': 1.5 })}/>`,
    '</g>',
  ];
}

// drillBadge draws a magnifying glass in a box: another view shows the inside of node id.
function drillBadge(id: string, b: Box): string[] {
  const { x, y, w, h } = b;
  return [
    `<g${attrs({ 'data-drill': id })}>`,
    `<rect${attrs({ x, y, width: w, height: h, rx: 2, fill: LABEL_BACKGROUND, stroke: BADGE_STROKE })}/>`,
    `<circle${attrs({ cx: x + 5, cy: y + 5, r: 2.5, fill: 'none', stroke: BADGE_STROKE, 'stroke-width': 1.5 })}/>`,
    `<path${attrs({ d: `M${num(x + 7)} ${num(y + 7)} L${num(x + w - 3)} ${num(y + h - 3)}`, stroke: BADGE_STROKE, 'stroke-width': 1.5 })}/>`,
    '</g>',
  ];
}

// overlayMarkup draws an overlay's outline and label.
export function overlayMarkup(o: DrawnOverlay, m: FontMetrics): string[] {
  const st = o.style;
  const out = [
    `<g${attrs({ 'data-overlay': o.id, opacity: st.opacity < 1 ? dec(st.opacity) : undefined })}>`,
    `<path${attrs({ d: o.path, fill: st.fill, stroke: st.stroke, 'stroke-width': dec(st.stroke_width), 'stroke-dasharray': dash(st.stroke_dash) })}/>`,
  ];
  if (o.label) out.push(...textLines(o.label.text, o.label.box, 'left', st.stroke === 'none' ? '#202124' : st.stroke, m));
  out.push('</g>');
  return out;
}

// decorationMarkup draws a decoration.
export function decorationMarkup(d: DrawnDecoration, m: FontMetrics): string[] {
  const st = d.style;
  const { x, y, w, h } = d.box;
  const paint = { fill: st.fill, stroke: st.stroke, 'stroke-width': dec(st.stroke_width), 'stroke-dasharray': dash(st.stroke_dash) };
  const out = [`<g${attrs({ 'data-decoration': d.id, opacity: st.opacity < 1 ? dec(st.opacity) : undefined })}>`];
  switch (d.type) {
    case 'text':
      if (d.text) out.push(...textLines(d.text, d.box, 'left', st.text_color, m));
      break;
    case 'rect':
    case 'band':
      out.push(`<rect${attrs({ x, y, width: w, height: h, ...paint })}/>`);
      break;
    case 'ellipse':
      out.push(`<ellipse${attrs({ cx: x + w / 2, cy: y + h / 2, rx: w / 2, ry: h / 2, ...paint })}/>`);
      break;
    case 'image':
      if (d.image) out.push(picture(d.image, d.box, 'meet'));
      break;
    case 'arrow': {
      const line = { fill: 'none', stroke: st.stroke, 'stroke-width': dec(st.stroke_width) };
      out.push(`<path${attrs({ d: path(d.points ?? []), ...line, 'stroke-dasharray': dash(st.stroke_dash) })}/>`);
      for (const p of d.marker ?? []) if ('line' in p) out.push(`<path${attrs({ d: path(p.line), ...line })}/>`);
      break;
    }
    case 'freehand':
      out.push(`<path${attrs({ d: d.path, transform: `translate(${num(d.origin.x)} ${num(d.origin.y)})`, ...paint })}/>`);
      break;
  }
  out.push('</g>');
  return out;
}

const FLOW_DASH = [6, 4];
const FLOW_PERIOD_MS = 500;

// flowDash returns an animated edge's dash pattern as written and its period (spec 4.3).
export function flowDash(st: EdgeStyle): { dash: number[]; period: number } {
  const d = st.stroke_dash.map((v) => Number(dec(v)));
  const pattern = d.some((v) => v > 0) ? d : FLOW_DASH;
  const sum = pattern.reduce((a, b) => a + b, 0);
  // SVG repeats a pattern with an odd number of values, so its period is twice the sum.
  return { dash: pattern, period: Number(dec(pattern.length % 2 ? 2 * sum : sum)) };
}

// flowPeriods returns the distinct dash periods of the animated edges, sorted; edge animation i is named dg-flow-<i>.
export function flowPeriods(edges: DrawnEdge[]): number[] {
  return [...new Set(edges.filter((e) => e.style.animate === 'flow').map((e) => flowDash(e.style).period))].sort((a, b) => a - b);
}

// flowCSS returns the animation of each period: the dash offset from 0 to minus the period, linear, repeating every FLOW_PERIOD_MS.
export function flowCSS(periods: number[]): string {
  return periods.map((p, i) => `@keyframes dg-flow-${i} { from { stroke-dashoffset: 0; } to { stroke-dashoffset: -${dec(p)}px; } }\n.dg-flow-${i} { animation: dg-flow-${i} ${FLOW_PERIOD_MS}ms linear infinite; }\n`).join('');
}

// edgeMarkup draws an edge; periods are the drawing's flowPeriods.
export function edgeMarkup(e: DrawnEdge, m: FontMetrics, periods: number[]): string[] {
  const st = e.style;
  const stroke = { fill: 'none', stroke: st.stroke, 'stroke-width': dec(st.stroke_width) };
  const flow = st.animate === 'flow' ? flowDash(st) : undefined;
  const line = { d: path(e.points), class: flow && `dg-flow-${periods.indexOf(flow.period)}`, ...stroke, 'stroke-dasharray': dash(flow?.dash ?? st.stroke_dash) };
  const out = [`<g${attrs({ 'data-edge': e.id })}>`, `<path${attrs(line)}/>`];
  const paint = (filled: boolean) => ({ fill: filled ? st.stroke : LABEL_BACKGROUND, stroke: st.stroke, 'stroke-width': dec(st.stroke_width) });
  for (const p of e.markers) {
    if ('line' in p) out.push(`<path${attrs({ d: path(p.line), ...stroke })}/>`);
    else if ('polygon' in p) out.push(`<path${attrs({ d: path(p.polygon) + ' Z', ...paint(p.filled) })}/>`);
    else out.push(`<circle${attrs({ cx: p.circle.x, cy: p.circle.y, r: dec(p.r), ...paint(p.filled) })}/>`);
  }
  for (const l of e.labels) {
    out.push(`<rect${attrs({ x: l.box.x, y: l.box.y, width: l.box.w, height: l.box.h, fill: LABEL_BACKGROUND })}/>`);
    out.push(...textLines(l.text, l.box, 'center', '#202124', m));
  }
  out.push('</g>');
  return out;
}

// shiftNode returns n moved by (dx, dy), for drawing it in coordinates local to its box.
export function shiftNode(n: DrawnNode, dx: number, dy: number): DrawnNode {
  const mv = <T extends Point>(p: T): T => ({ ...p, x: p.x + dx, y: p.y + dy });
  return {
    ...n,
    box: mv(n.box),
    textBox: mv(n.textBox),
    icon: n.icon && mv(n.icon),
    picture: n.picture && mv(n.picture),
    ports: n.ports.map(mv),
    badge: n.badge && mv(n.badge),
    drill: n.drill && mv(n.drill),
    rotate: n.rotate && mv(n.rotate),
    divider: n.divider === undefined ? undefined : n.divider + dx,
    table: n.table && {
      ...n.table,
      lines: n.table.lines.map((y) => y + dy),
      rows: n.table.rows.map((r) => ({ ...r, y: r.y + dy })),
      keyX: n.table.keyX + dx,
      nameX: n.table.nameX + dx,
      typeX: n.table.typeX + dx,
    },
  };
}

const PICTURE = { w: 44, h: 32, inset: 4 };

// kindPicture is a small SVG of a node of a kind in style, with no label: the palette's picture
// of it, drawn by nodeMarkup like the canvas.
export function kindPicture(style: NodeStyle, shapes: Record<string, CustomShape>): string {
  const { w, h, inset } = PICTURE;
  const round = !!shapes[style.shape] || LABELLESS.has(style.shape);
  const bw = round ? h - 2 * inset : w - 2 * inset;
  const box = { x: (w - bw) / 2, y: inset, w: bw, h: h - 2 * inset };
  const icon = style.icon ? { x: w / 2 - 7, y: h / 2 - 7, w: 14, h: 14 } : undefined;
  const n: DrawnNode = {
    id: '',
    box,
    container: false,
    pinned: false,
    tooSmall: false,
    style: { ...style, stroke_width: Math.min(style.stroke_width, 1.5), corner_radius: Math.min(style.corner_radius, 4), opacity: 1 },
    custom: shapes[style.shape] ? style.shape : undefined,
    text: null,
    textBox: box,
    align: 'center',
    icon,
    picture: style.image ? box : undefined,
    ports: [],
  };
  const s = { shapes };
  const defs = defsMarkup([n], s);
  return `<svg xmlns="http://www.w3.org/2000/svg" class="picture" viewBox="0 0 ${w} ${h}" aria-hidden="true">${defs.length ? `<defs>${defs.join('')}</defs>` : ''}${nodeMarkup(n, { ascent: 1, descent: 0 }, s).join('')}</svg>`;
}

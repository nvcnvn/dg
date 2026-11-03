import type { Box, Point } from '../geometry/geometry';
import { CYLINDER_RY, DOCUMENT_WAVE, outline, personBodyTop, personHead, svgSize } from '../geometry/shapes';
import { lineHeight, type TextBlock } from '../geometry/text';
import type { CustomShape, EdgeStyle, NodeStyle, Side } from '../protocol/view';

// Markup for one node or edge from its geometry. The SVG export and the editor canvas both
// draw with these functions, so there is one drawing path (spec 15).

export interface DrawnNode {
  id: string;
  box: Box;
  container: boolean;
  pinned: boolean;
  tooSmall: boolean;
  style: NodeStyle;
  custom?: string;
  text: TextBlock | null;
  textBox: Box;
  align: 'center' | 'left';
  icon?: Box;
  picture?: Box;
  ports: DrawnPort[];
}

export interface DrawnPort extends Point {
  name: string;
  side: Side;
}

export interface DrawnEdge {
  id: string;
  points: Point[];
  arrowStart?: [Point, Point];
  arrowEnd?: [Point, Point];
  label?: { text: TextBlock; box: Box };
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
  const shift = ((m.ascent - m.descent) * t.font.size) / 2;
  return t.lines.map((line, i) => {
    const a = attrs({
      x: align === 'center' ? b.x + b.w / 2 : b.x,
      y: top + i * lh + lh / 2 + shift,
      'text-anchor': align === 'center' ? 'middle' : undefined,
      fill: color,
      'font-size': dec(t.font.size),
      'font-weight': t.font.weight === 400 ? undefined : String(t.font.weight),
    });
    return `<text${a}>${esc(line)}</text>`;
  });
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
  const out = [`<g${attrs({ 'data-node': n.id, opacity: st.opacity < 1 ? dec(st.opacity) : undefined })}>`, ...outlineMarkup(n, s)];
  if (n.picture && st.image) out.push(picture(st.image, n.picture, 'meet'));
  if (n.icon && st.icon) out.push(picture(st.icon, n.icon, 'meet'));
  if (n.text) out.push(...textLines(n.text, n.textBox, n.align, st.text_color, m));
  if (n.tooSmall) {
    const b = n.box;
    out.push(`<rect${attrs({ x: b.x - 3, y: b.y - 3, width: b.w + 6, height: b.h + 6, fill: 'none', stroke: WARNING, 'stroke-dasharray': '4 4' })}/>`);
  }
  out.push('</g>');
  return out;
}

export function edgeMarkup(e: DrawnEdge, m: FontMetrics): string[] {
  const st = e.style;
  const stroke = { fill: 'none', stroke: st.stroke, 'stroke-width': dec(st.stroke_width) };
  const out = [`<g${attrs({ 'data-edge': e.id })}>`, `<path${attrs({ d: path(e.points), ...stroke, 'stroke-dasharray': dash(st.stroke_dash) })}/>`];
  if (e.arrowEnd) out.push(`<path${attrs({ d: path([e.arrowEnd[0], e.points[e.points.length - 1], e.arrowEnd[1]]), ...stroke })}/>`);
  if (e.arrowStart) out.push(`<path${attrs({ d: path([e.arrowStart[0], e.points[0], e.arrowStart[1]]), ...stroke })}/>`);
  if (e.label) {
    const lb = e.label.box;
    out.push(`<rect${attrs({ x: lb.x, y: lb.y, width: lb.w, height: lb.h, fill: LABEL_BACKGROUND })}/>`);
    out.push(...textLines(e.label.text, lb, 'center', '#202124', m));
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
  };
}

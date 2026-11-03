import { CONTAINER_PADDING, LINE_HEIGHT, type Box, type Point } from '../geometry/geometry';
import type { EdgeStyle, NodeStyle } from '../protocol/view';

// Markup for one node or edge from its geometry, kept apart from any output
// format so that every way of drawing a view can share it.

export interface DrawnNode {
  id: string;
  box: Box;
  label: string;
  container: boolean;
  style: NodeStyle;
}

export interface DrawnEdge {
  id: string;
  points: Point[];
  arrow?: [Point, Point];
  label?: { text: string; box: Box };
  style: EdgeStyle;
}

export interface FontMetrics {
  ascent: number;
  descent: number;
}

const TEXT_COLOR = '#202124';
const CORNER_RADIUS = 8;

// num writes a coordinate as an integer, so output does not depend on
// sub-pixel differences in text shaping.
export function num(v: number): string {
  const r = Math.round(v);
  return String(r === 0 ? 0 : r);
}

export function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

export function attrs(a: Record<string, string | number>): string {
  return Object.entries(a)
    .map(([k, v]) => ` ${k}="${typeof v === 'number' ? num(v) : esc(v)}"`)
    .join('');
}

function path(pts: Point[]): string {
  return pts.map((p, i) => `${i ? 'L' : 'M'}${num(p.x)} ${num(p.y)}`).join(' ');
}

function baseline(centerY: number, m: FontMetrics): number {
  return centerY + (m.ascent - m.descent) / 2;
}

export function nodeMarkup(n: DrawnNode, m: FontMetrics): string[] {
  const { x, y, w, h } = n.box;
  const rx: Record<string, number> = n.style.shape === 'rounded' ? { rx: CORNER_RADIUS } : {};
  const text = n.container
    ? attrs({ x: x + CONTAINER_PADDING, y: baseline(y + CONTAINER_PADDING / 2 + LINE_HEIGHT / 2, m), fill: TEXT_COLOR })
    : attrs({ x: x + w / 2, y: baseline(y + h / 2, m), 'text-anchor': 'middle', fill: TEXT_COLOR });
  return [
    `<g${attrs({ 'data-node': n.id })}>`,
    `<rect${attrs({ x, y, width: w, height: h, ...rx, fill: n.style.fill, stroke: n.style.stroke })}/>`,
    `<text${text}>${esc(n.label)}</text>`,
    '</g>',
  ];
}

export function edgeMarkup(e: DrawnEdge, m: FontMetrics): string[] {
  const stroke = { fill: 'none', stroke: e.style.stroke, 'stroke-width': '1.5' };
  const out = [`<g${attrs({ 'data-edge': e.id })}>`, `<path${attrs({ d: path(e.points), ...stroke })}/>`];
  if (e.arrow) {
    const tip = e.points[e.points.length - 1];
    out.push(`<path${attrs({ d: path([e.arrow[0], tip, e.arrow[1]]), ...stroke })}/>`);
  }
  if (e.label) {
    const lb = e.label.box;
    out.push(`<rect${attrs({ x: lb.x, y: lb.y, width: lb.w, height: lb.h, fill: '#FFFFFF' })}/>`);
    out.push(
      `<text${attrs({ x: lb.x + lb.w / 2, y: baseline(lb.y + lb.h / 2, m), 'text-anchor': 'middle', fill: TEXT_COLOR })}>${esc(e.label.text)}</text>`,
    );
  }
  out.push('</g>');
  return out;
}

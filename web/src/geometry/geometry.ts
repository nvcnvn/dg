import type { CustomShape, ResolvedView, ViewNode } from '../protocol/view';
import { innerBox, LABELLESS, outerSize, svgSize } from './shapes';
import { isTable, measureTable, type TableMeasure } from './table';
import { drawnWeight, textBlock, type TextBlock, type TextMeasure } from './text';

export interface Point {
  x: number;
  y: number;
}

export interface Size {
  w: number;
  h: number;
}

export interface Box extends Point, Size {}

// GAP separates an icon or picture from the label under it, and a label from the collapse badge
// under it.
export const GAP = 4;
// BADGE is the side of a collapsed container's "+" badge and of a drill-down badge.
export const BADGE = 12;

export function ceilTo(v: number, grid: number): number {
  return Math.ceil(v / grid) * grid;
}

export function roundTo(v: number, grid: number): number {
  return Math.round(v / grid) * grid;
}

export function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

// NodeMeasure is what a node's content needs. For a leaf, size is its drawn size; for a
// container, the smallest size that holds its label, and origin is its content origin relative
// to its top-left corner (spec 7.1).
export interface NodeMeasure {
  text: TextBlock | null;
  icon: number;
  picture: number;
  content: Size;
  size: Size;
  origin: Point;
  table?: TableMeasure;
}

export function iconSide(fontSize: number): number {
  return Math.round(fontSize * 2);
}

export function pictureSide(fontSize: number): number {
  return Math.round(fontSize * 6);
}

// measureNode sizes a node by spec 7.2 rules 1–3; a container's size is completed by
// containerSize once its children are placed.
export function measureNode(n: ViewNode, m: TextMeasure, grid: number, custom?: CustomShape): NodeMeasure {
  const s = n.style;
  const pad = s.padding;
  const font = { size: s.font_size, weight: drawnWeight(s.font_weight) };
  const text = LABELLESS.has(s.shape) || n.label === '' ? null : textBlock(n.label, s.max_w > 0 ? s.max_w - 2 * pad : 0, font, m);
  const icon = s.icon ? iconSide(s.font_size) : 0;
  const picture = s.shape === 'image' && !n.container ? pictureSide(s.font_size) : 0;
  const top = Math.max(icon, picture);
  const content = { w: Math.max(text?.w ?? 0, top), h: top + (top && text ? GAP : 0) + (text?.h ?? 0) };
  if (n.collapsed) {
    content.w = Math.max(content.w, BADGE);
    content.h += (content.h ? GAP : 0) + BADGE;
  }
  const pin = n.pin;

  if (n.container) {
    const origin = { x: pad, y: content.h ? pad / 2 + content.h + pad : pad };
    const size = {
      w: Math.max(s.min_w, ceilTo(content.w + 2 * pad, grid)),
      h: Math.max(s.min_h, ceilTo(origin.y + pad, grid)),
    };
    return { text, icon, picture, content, size, origin };
  }

  const table = isTable(n) ? measureTable(n, text, m) : undefined;
  const outer = table ? table.size : LABELLESS.has(s.shape) ? { w: 0, h: 0 } : outerSize(s.shape, { w: content.w + 2 * pad, h: content.h + 2 * pad }, custom);
  let w = Math.max(s.min_w, outer.w);
  let h = Math.max(s.min_h, outer.h);
  if (custom?.stretch === 'none') {
    const nat = svgSize(custom.svg);
    w = Math.max(w, nat.w);
    h = Math.max(h, nat.h);
  }
  let size = { w: ceilTo(w, grid), h: ceilTo(h, grid) };
  if (custom?.stretch === 'uniform') {
    // The larger side goes on the grid and the other keeps the picture's aspect ratio.
    const nat = svgSize(custom.svg);
    const r = nat.w / nat.h;
    if (w / h < r) w = h * r;
    else h = w / r;
    size = w >= h ? { w: ceilTo(w, grid), h: Math.round(ceilTo(w, grid) / r) } : { w: Math.round(ceilTo(h, grid) * r), h: ceilTo(h, grid) };
  }
  size = { w: pin?.w ?? size.w, h: pin?.h ?? size.h };
  return { text, icon, picture, content, size, origin: { x: 0, y: 0 }, table };
}

// drillBadge returns where a node's drill-down badge goes: the top right corner of its content
// area, or of its box for a container.
export function drillBadge(n: ViewNode, b: Box, custom?: CustomShape): Box {
  const area = n.container ? b : innerBox(n.style.shape, b, custom);
  const inset = 3;
  return { x: Math.round(area.x + area.w - BADGE - inset), y: Math.round(area.y + inset), w: BADGE, h: BADGE };
}

// containerSize fits a container to its label and its children, whose boxes are relative to
// its content origin (spec 7.2 rule 4), unless its size is pinned; tooSmall reports a pinned
// size that does not hold the children (rule 5).
export function containerSize(n: ViewNode, m: NodeMeasure, children: Box[], grid: number): { size: Size; tooSmall: boolean } {
  let right = 0;
  let bottom = 0;
  for (const c of children) {
    right = Math.max(right, Math.max(0, c.x) + c.w);
    bottom = Math.max(bottom, Math.max(0, c.y) + c.h);
  }
  const pad = n.style.padding;
  const fit = {
    w: Math.max(m.size.w, ceilTo(m.origin.x + right + pad, grid)),
    h: Math.max(m.size.h, ceilTo(m.origin.y + bottom + pad, grid)),
  };
  const size = { w: n.pin?.w ?? fit.w, h: n.pin?.h ?? fit.h };
  return { size, tooSmall: size.w < fit.w || size.h < fit.h };
}

export function depth(byId: Map<string, ViewNode>, node: ViewNode): number {
  let d = 0;
  for (let n = node.frame ? byId.get(node.frame) : undefined; n; n = n.frame ? byId.get(n.frame) : undefined) {
    d++;
  }
  return d;
}

// Layout is every node's position relative to its frame's content origin, and its size.
export interface Layout {
  rel: Map<string, Point>;
  sizes: Map<string, Size>;
  tooSmall: Set<string>;
}

// fitContainers completes a layout whose leaf sizes and positions are known by sizing every
// container, innermost first.
export function fitContainers(view: ResolvedView, measures: Map<string, NodeMeasure>, rel: Map<string, Point>, grid: number): Layout {
  const sizes = new Map<string, Size>();
  const tooSmall = new Set<string>();
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const deepestFirst = view.nodes.map((n) => ({ n, d: depth(byId, n) })).sort((a, b) => b.d - a.d || cmp(a.n.id, b.n.id));
  for (const { n } of deepestFirst) {
    if (!n.container) {
      sizes.set(n.id, measures.get(n.id)!.size);
      continue;
    }
    const children = view.nodes.filter((c) => c.frame === n.id).map((c) => ({ ...at(rel, c.id), ...sizes.get(c.id)! }));
    const { size, tooSmall: small } = containerSize(n, measures.get(n.id)!, children, grid);
    sizes.set(n.id, size);
    if (small) tooSmall.add(n.id);
  }
  return { rel, sizes, tooSmall };
}

// absoluteBoxes returns every node's box in canvas coordinates. A negative coordinate inside a
// container is drawn as 0 (spec 7.1).
export function absoluteBoxes(view: ResolvedView, measures: Map<string, NodeMeasure>, layout: Layout): Map<string, Box> {
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const shallowFirst = view.nodes.map((n) => ({ n, d: depth(byId, n) })).sort((a, b) => a.d - b.d || cmp(a.n.id, b.n.id));
  const boxes = new Map<string, Box>();
  for (const { n } of shallowFirst) {
    const p = at(layout.rel, n.id);
    const frame = n.frame ? boxes.get(n.frame) : undefined;
    const o = frame ? measures.get(n.frame!)!.origin : { x: 0, y: 0 };
    const x = frame ? frame.x + o.x + Math.max(0, p.x) : p.x;
    const y = frame ? frame.y + o.y + Math.max(0, p.y) : p.y;
    boxes.set(n.id, { x, y, ...layout.sizes.get(n.id)! });
  }
  return boxes;
}

function at(rel: Map<string, Point>, id: string): Point {
  return rel.get(id) ?? { x: 0, y: 0 };
}

export function center(b: Box): Point {
  return { x: b.x + b.w / 2, y: b.y + b.h / 2 };
}

export function intersects(a: Box, b: Box): boolean {
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
}

// ContentLayout is where a node's text block, icon and picture go in its box.
export interface ContentLayout {
  textBox: Box;
  align: 'center' | 'left';
  icon?: Box;
  picture?: Box;
  // badge is the "+" badge of a collapsed container, under its label.
  badge?: Box;
}

// contentLayout lays out a node's content in box b: a container's at its top-left corner, its
// label half a padding from the top and a padding above the content origin; a leaf's centered in its shape's
// content area, with an image shape's picture filling the area above the label.
export function contentLayout(n: ViewNode, m: NodeMeasure, b: Box, custom?: CustomShape): ContentLayout {
  const pad = n.style.padding;
  const th = m.text?.h ?? 0;
  if (n.container) {
    const x = b.x + pad;
    const y = b.y + pad / 2;
    const icon = m.icon ? { x, y, w: m.icon, h: m.icon } : undefined;
    const top = y + (icon ? m.icon + GAP : 0);
    return { textBox: { x, y: top, w: m.text?.w ?? 0, h: th }, align: 'left', icon };
  }
  if (m.table) return { textBox: { x: b.x + pad, y: b.y + pad, w: Math.max(0, b.w - 2 * pad), h: th }, align: 'center' };
  const inner = innerBox(n.style.shape, b, custom);
  const area = { x: inner.x + pad, y: inner.y + pad, w: Math.max(0, inner.w - 2 * pad), h: Math.max(0, inner.h - 2 * pad) };
  if (m.picture) {
    const ph = Math.max(0, area.h - th - (th ? GAP : 0));
    return { textBox: { x: area.x, y: area.y + area.h - th, w: area.w, h: th }, align: 'center', picture: { x: area.x, y: area.y, w: area.w, h: ph } };
  }
  const top = area.y + (area.h - m.content.h) / 2;
  const icon = m.icon ? { x: area.x + (area.w - m.icon) / 2, y: top, w: m.icon, h: m.icon } : undefined;
  const textBox = { x: area.x, y: top + (icon ? m.icon + GAP : 0), w: area.w, h: th };
  const badge = n.collapsed ? { x: Math.round(area.x + (area.w - BADGE) / 2), y: Math.round(textBox.y + th + (th ? GAP : 0)), w: BADGE, h: BADGE } : undefined;
  return { textBox, align: 'center', icon, ...(badge ? { badge } : {}) };
}

import type { ResolvedView, ViewNode } from '../protocol/view';

export interface Point {
  x: number;
  y: number;
}

export interface Size {
  w: number;
  h: number;
}

export interface Box extends Point, Size {}

export interface TextMeasure {
  width(text: string): number;
  ascent: number;
  descent: number;
}

export const FONT_SIZE = 14;
export const LINE_HEIGHT = 20;
export const PADDING = 10;
export const CONTAINER_PADDING = 20;
export const MIN_W = 100;
export const MIN_H = 40;

export function ceilTo(v: number, grid: number): number {
  return Math.ceil(v / grid) * grid;
}

export function roundTo(v: number, grid: number): number {
  return Math.round(v / grid) * grid;
}

export function leafSize(label: string, text: TextMeasure, grid: number): Size {
  return {
    w: Math.max(MIN_W, ceilTo(text.width(label) + 2 * PADDING, grid)),
    h: Math.max(MIN_H, ceilTo(LINE_HEIGHT + 2 * PADDING, grid)),
  };
}

// The top-left of a container's inside, past its label and padding.
export const CONTENT_ORIGIN: Point = { x: CONTAINER_PADDING, y: CONTAINER_PADDING + LINE_HEIGHT };

export function containerMinSize(label: string, text: TextMeasure, grid: number): Size {
  return {
    w: Math.max(MIN_W, ceilTo(text.width(label) + 2 * CONTAINER_PADDING, grid)),
    h: Math.max(MIN_H, CONTENT_ORIGIN.y + CONTAINER_PADDING),
  };
}

// fitContainer sizes a container to its label and its children, whose boxes
// are relative to its content origin (spec 7.2 rule 4).
export function fitContainer(min: Size, children: Box[], grid: number): Size {
  let right = 0;
  let bottom = 0;
  for (const c of children) {
    right = Math.max(right, Math.max(0, c.x) + c.w);
    bottom = Math.max(bottom, Math.max(0, c.y) + c.h);
  }
  return {
    w: Math.max(min.w, ceilTo(CONTENT_ORIGIN.x + right + CONTAINER_PADDING, grid)),
    h: Math.max(min.h, ceilTo(CONTENT_ORIGIN.y + bottom + CONTAINER_PADDING, grid)),
  };
}

export function depth(byId: Map<string, ViewNode>, node: ViewNode): number {
  let d = 0;
  for (let n = node.frame ? byId.get(node.frame) : undefined; n; n = n.frame ? byId.get(n.frame) : undefined) {
    d++;
  }
  return d;
}

// layoutBoxes sizes every node and returns its absolute box, given each
// node's position relative to its frame's content origin. Leaf sizes come
// from leafSizes; containers fit their children.
export function layoutBoxes(
  view: ResolvedView,
  leafSizes: Map<string, Size>,
  minSizes: Map<string, Size>,
  rel: Map<string, Point>,
  grid: number,
): Map<string, Box> {
  const sizes = new Map(leafSizes);
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const deepestFirst = view.nodes
    .map((n) => ({ n, d: depth(byId, n) }))
    .sort((a, b) => b.d - a.d || cmp(a.n.id, b.n.id));
  for (const { n } of deepestFirst) {
    if (!n.container) continue;
    const children = view.nodes
      .filter((c) => c.frame === n.id)
      .map((c) => ({ ...at(rel, c.id), ...sizeOf(sizes, c.id) }));
    sizes.set(n.id, fitContainer(minSizes.get(n.id) ?? { w: MIN_W, h: MIN_H }, children, grid));
  }

  const boxes = new Map<string, Box>();
  for (const { n } of deepestFirst.reverse()) {
    const p = at(rel, n.id);
    const frame = n.frame ? boxes.get(n.frame) : undefined;
    const origin = frame ? { x: frame.x + CONTENT_ORIGIN.x, y: frame.y + CONTENT_ORIGIN.y } : { x: 0, y: 0 };
    const x = frame ? Math.max(0, p.x) : p.x;
    const y = frame ? Math.max(0, p.y) : p.y;
    boxes.set(n.id, { x: origin.x + x, y: origin.y + y, ...sizeOf(sizes, n.id) });
  }
  return boxes;
}

function at(rel: Map<string, Point>, id: string): Point {
  return rel.get(id) ?? { x: 0, y: 0 };
}

function sizeOf(sizes: Map<string, Size>, id: string): Size {
  return sizes.get(id) ?? { w: MIN_W, h: MIN_H };
}

export function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

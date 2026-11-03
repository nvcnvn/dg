import type { CustomShape } from '../protocol/view';
import type { Box, Point, Size } from './geometry';

// Shapes that draw no label.
export const LABELLESS = new Set(['point', 'bullseye']);

export const CYLINDER_RY = 7;
export const DOCUMENT_WAVE = 8;
const PERSON_HEAD = 0.3;
const PERSON_OVERLAP = 0.8;

// outerSize returns the size of the box a shape needs so that its content area holds c.
export function outerSize(shape: string, c: Size, custom?: CustomShape): Size {
  if (custom) {
    const [, , fw, fh] = custom.text_box;
    return { w: c.w / Math.max(fw, 0.01), h: c.h / Math.max(fh, 0.01) };
  }
  switch (shape) {
    case 'ellipse':
      return { w: c.w * Math.SQRT2, h: c.h * Math.SQRT2 };
    case 'diamond':
      return { w: c.w * 2, h: c.h * 2 };
    case 'stadium':
    case 'hexagon':
    case 'parallelogram':
      return { w: c.w + c.h / 2, h: c.h };
    case 'cylinder':
      return { w: c.w, h: c.h + 3 * CYLINDER_RY };
    case 'document':
      return { w: c.w, h: c.h + DOCUMENT_WAVE };
    case 'person':
      return { w: c.w, h: c.h / (1 - PERSON_HEAD * PERSON_OVERLAP) };
    case 'cloud':
      return { w: c.w / 0.7, h: c.h / 0.65 };
    case 'point':
    case 'bullseye':
      return { w: 0, h: 0 };
  }
  return c;
}

// innerBox returns the content area of a shape drawn in box b.
export function innerBox(shape: string, b: Box, custom?: CustomShape): Box {
  if (custom) {
    const [fx, fy, fw, fh] = custom.text_box;
    return { x: b.x + fx * b.w, y: b.y + fy * b.h, w: fw * b.w, h: fh * b.h };
  }
  const centered = (w: number, h: number): Box => ({ x: b.x + (b.w - w) / 2, y: b.y + (b.h - h) / 2, w, h });
  switch (shape) {
    case 'ellipse':
      return centered(b.w / Math.SQRT2, b.h / Math.SQRT2);
    case 'diamond':
      return centered(b.w / 2, b.h / 2);
    case 'stadium':
    case 'hexagon':
    case 'parallelogram':
      return { x: b.x + b.h / 4, y: b.y, w: b.w - b.h / 2, h: b.h };
    case 'cylinder':
      return { x: b.x, y: b.y + 2 * CYLINDER_RY, w: b.w, h: b.h - 3 * CYLINDER_RY };
    case 'document':
      return { x: b.x, y: b.y, w: b.w, h: b.h - DOCUMENT_WAVE };
    case 'person': {
      const top = personBodyTop(b);
      return { x: b.x, y: top, w: b.w, h: b.y + b.h - top };
    }
    case 'cloud':
      return centered(b.w * 0.7, b.h * 0.65);
  }
  return b;
}

export function personHead(b: Box): number {
  return Math.min(b.h * PERSON_HEAD, b.w / 2);
}

export function personBodyTop(b: Box): number {
  return b.y + personHead(b) * PERSON_OVERLAP;
}

// outline returns the polygon of a shape's outline, which edges ending at the shape are
// clipped to.
export function outline(shape: string, b: Box): Point[] {
  const { x, y, w, h } = b;
  const cx = x + w / 2;
  const cy = y + h / 2;
  const ellipse = (rx: number, ry: number) =>
    Array.from({ length: 32 }, (_, i) => ({ x: cx + rx * Math.cos((i * Math.PI) / 16), y: cy + ry * Math.sin((i * Math.PI) / 16) }));
  switch (shape) {
    case 'ellipse':
      return ellipse(w / 2, h / 2);
    case 'point':
    case 'bullseye':
      return ellipse(Math.min(w, h) / 2, Math.min(w, h) / 2);
    case 'diamond':
      return [{ x: cx, y }, { x: x + w, y: cy }, { x: cx, y: y + h }, { x, y: cy }];
    case 'hexagon':
      return [
        { x: x + h / 4, y },
        { x: x + w - h / 4, y },
        { x: x + w, y: cy },
        { x: x + w - h / 4, y: y + h },
        { x: x + h / 4, y: y + h },
        { x, y: cy },
      ];
    case 'parallelogram':
      return [{ x: x + h / 4, y }, { x: x + w, y }, { x: x + w - h / 4, y: y + h }, { x, y: y + h }];
  }
  return [{ x, y }, { x: x + w, y }, { x: x + w, y: y + h }, { x, y: y + h }];
}

// svgSize returns the natural size of an SVG picture: its viewBox, else its width and height,
// else 100 by 100.
export function svgSize(svg: string): Size {
  const tag = /<svg\b[^>]*>/i.exec(svg)?.[0] ?? '';
  const attr = (name: string) => new RegExp(`\\s${name}\\s*=\\s*["']([^"']*)["']`, 'i').exec(tag)?.[1];
  const vb = attr('viewBox')?.trim().split(/[\s,]+/).map(Number);
  if (vb && vb.length === 4 && vb[2] > 0 && vb[3] > 0) return { w: vb[2], h: vb[3] };
  const w = parseFloat(attr('width') ?? '');
  const h = parseFloat(attr('height') ?? '');
  return w > 0 && h > 0 ? { w, h } : { w: 100, h: 100 };
}

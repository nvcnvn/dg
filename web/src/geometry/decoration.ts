import type { ViewDecoration } from '../protocol/view';
import { center, type Box, type NodeMeasure, type Point } from './geometry';
import { textBlock, type TextBlock, type TextMeasure } from './text';

// PlacedDecoration is a decoration with its geometry in canvas coordinates: the box it covers,
// its text, and an arrow's points or the offset a freehand path is drawn at.
export interface PlacedDecoration {
  d: ViewDecoration;
  box: Box;
  text?: TextBlock;
  points?: Point[];
  origin: Point;
}

// placeDecorations places a view's decorations (spec 5.7). A decoration's position is (x, y) in
// its frame, the content origin of its in node or the canvas; an attached decoration's is its
// node's center moved by (x, y). A box or text is drawn with its top left corner at the
// position, or, attached, centered on it; arrow points and a freehand path are relative to it.
export function placeDecorations(decorations: ViewDecoration[], boxes: Map<string, Box>, measures: Map<string, NodeMeasure>, text: TextMeasure): PlacedDecoration[] {
  return decorations.map((d) => {
    let origin: Point = { x: d.x, y: d.y };
    if (d.attach) {
      const c = center(boxes.get(d.attach)!);
      origin = { x: c.x + d.x, y: c.y + d.y };
    } else if (d.in) {
      const b = boxes.get(d.in)!;
      const o = measures.get(d.in)!.origin;
      origin = { x: b.x + o.x + d.x, y: b.y + o.y + d.y };
    }
    const at = (w: number, h: number): Box => (d.attach ? { x: origin.x - w / 2, y: origin.y - h / 2, w, h } : { x: origin.x, y: origin.y, w, h });
    switch (d.type) {
      case 'text': {
        const t = textBlock(d.text ?? '', 0, { size: d.font_size, weight: 400 }, text);
        return { d, box: at(t.w, t.h), text: t, origin };
      }
      case 'arrow': {
        const points = (d.points ?? []).map(([x, y]) => ({ x: origin.x + x, y: origin.y + y }));
        return { d, box: boundsOf(points), points, origin };
      }
      case 'freehand': {
        const b = pathBounds(d.path ?? '');
        return { d, box: { ...b, x: b.x + origin.x, y: b.y + origin.y }, origin };
      }
    }
    return { d, box: at(d.w ?? 0, d.h ?? 0), origin };
  });
}

function boundsOf(points: Point[]): Box {
  if (points.length === 0) return { x: 0, y: 0, w: 0, h: 0 };
  const xs = points.map((p) => p.x);
  const ys = points.map((p) => p.y);
  return { x: Math.min(...xs), y: Math.min(...ys), w: Math.max(...xs) - Math.min(...xs), h: Math.max(...ys) - Math.min(...ys) };
}

const ARGS: Record<string, number> = { M: 2, L: 2, H: 1, V: 1, C: 6, S: 4, Q: 4, T: 2, A: 7, Z: 0 };

// pathBounds returns a box holding SVG path data: its end and control points, and the full
// radii around an arc's ends, which hold the arc.
export function pathBounds(path: string): Box {
  const tokens = path.match(/[MmLlHhVvCcSsQqTtAaZz]|[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?/g) ?? [];
  const pts: Point[] = [];
  let cur = { x: 0, y: 0 };
  let start = cur;
  let cmd = '';
  let i = 0;
  while (i < tokens.length) {
    if (/[a-z]/i.test(tokens[i])) cmd = tokens[i++];
    const up = cmd.toUpperCase();
    const n = ARGS[up];
    if (n === undefined) break;
    if (n === 0) {
      cur = start;
      continue;
    }
    const args = tokens.slice(i, i + n).map(Number);
    if (args.length < n || args.some((v) => !Number.isFinite(v))) break;
    i += n;
    const rel = cmd !== up;
    const abs = (x: number, y: number): Point => (rel ? { x: cur.x + x, y: cur.y + y } : { x, y });
    let next: Point;
    switch (up) {
      case 'H':
        next = { x: rel ? cur.x + args[0] : args[0], y: cur.y };
        break;
      case 'V':
        next = { x: cur.x, y: rel ? cur.y + args[0] : args[0] };
        break;
      case 'A': {
        next = abs(args[5], args[6]);
        const r = Math.max(Math.abs(args[0]), Math.abs(args[1]));
        for (const p of [cur, next]) pts.push({ x: p.x - r, y: p.y - r }, { x: p.x + r, y: p.y + r });
        break;
      }
      default:
        for (let k = 0; k + 2 < n; k += 2) pts.push(abs(args[k], args[k + 1]));
        next = abs(args[n - 2], args[n - 1]);
    }
    if (up !== 'M') pts.push(cur);
    pts.push(next);
    cur = next;
    if (up === 'M') {
      start = next;
      cmd = rel ? 'l' : 'L';
    }
  }
  return boundsOf(pts.length ? pts : [{ x: 0, y: 0 }]);
}

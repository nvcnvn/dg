import type { ViewNode } from '../protocol/view';
import type { Box, Size } from './geometry';
import { lineHeight, type Font, type TextBlock, type TextMeasure } from './text';

export interface TableRow {
  name: string;
  key?: string;
  type?: string;
  // y is the center of the row's line, relative to the top of the box.
  y: number;
}

export interface TableMeasure {
  // lines are the separators' y, relative to the top of the box.
  lines: number[];
  rows: TableRow[];
  // nameX is the left of the name column, relative to the left of the box.
  nameX: number;
  font: Font;
  keyFont: Font;
  size: Size;
}

export const KEY_WEIGHT = 700;

export function isTable(n: ViewNode): boolean {
  return n.style.shape === 'table' && !n.container && (n.compartments?.length ?? 0) > 0;
}

// measureTable lays out a table's header and rows (spec 5.4); label is its wrapped label, if any.
export function measureTable(n: ViewNode, label: TextBlock | null, m: TextMeasure): TableMeasure {
  const pad = n.style.padding;
  const font = { size: n.style.font_size, weight: 400 };
  const keyFont = { ...font, weight: KEY_WEIGHT };
  const gap = n.style.font_size;
  const rowH = lineHeight(font.size) + pad / 2;
  const all = n.compartments!.flatMap((c) => c.rows);
  const widest = (texts: (string | undefined)[], f: Font) => Math.max(0, ...texts.map((t) => (t ? m.width(t, f) : 0)));
  const keyW = widest(all.map((r) => r.key), keyFont);
  const nameW = widest(all.map((r) => r.name), font);
  const typeW = widest(all.map((r) => r.type), font);
  const nameX = pad + (keyW ? keyW + gap : 0);

  const lines: number[] = [];
  const rows: TableRow[] = [];
  let y = label ? 2 * pad + label.h : 0;
  for (const c of n.compartments!) {
    if (y > 0) lines.push(y);
    y += pad / 2;
    for (const r of c.rows) {
      rows.push({ name: r.name, key: r.key || undefined, type: r.type || undefined, y: y + rowH / 2 });
      y += rowH;
    }
    y += pad / 2;
  }
  const w = Math.max((label?.w ?? 0) + 2 * pad, nameX + nameW + (typeW ? gap + typeW : 0) + pad);
  return { lines, rows, nameX, font, keyFont, size: { w, h: y } };
}

// rowLines returns the y, in canvas coordinates, of each row's line of a table drawn in box b.
export function rowLines(t: TableMeasure, b: Box): Map<string, number> {
  return new Map(t.rows.map((r) => [r.name, Math.round(b.y + r.y)]));
}

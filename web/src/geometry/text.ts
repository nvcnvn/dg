export interface Font {
  size: number;
  weight: number;
}

// TextMeasure measures text in the drawing font. ascent and descent are fractions of the font
// size.
export interface TextMeasure {
  width(text: string, font: Font): number;
  ascent: number;
  descent: number;
}

export interface TextBlock {
  lines: string[];
  font: Font;
  w: number;
  h: number;
  // sub styles the lines from from on.
  sub?: SubStyle & { from: number };
}

// SubStyle is how a label's lines after its first line break differ from the first.
export interface SubStyle {
  font: Font;
  color?: string;
}

const graphemes = new Intl.Segmenter('en', { granularity: 'grapheme' });

// drawnWeight returns the bundled weight text of a given font_weight is drawn with (spec 3.4).
export function drawnWeight(weight: number): number {
  return weight >= 600 ? 700 : 400;
}

export function lineHeight(size: number): number {
  return Math.ceil(size * 1.4);
}

// textBlock breaks text at newlines and wraps each line at maxWidth (no limit when it is 0),
// greedily by words, breaking a word that is wider on its own between grapheme clusters.
export function textBlock(text: string, maxWidth: number, font: Font, m: TextMeasure): TextBlock {
  const lines: string[] = [];
  const fits = (s: string) => maxWidth <= 0 || m.width(s, font) <= maxWidth;
  for (const para of text.split('\n')) {
    let line = '';
    for (const word of para.split(/ +/)) {
      const joined = line ? line + ' ' + word : word;
      if (fits(joined)) {
        line = joined;
        continue;
      }
      if (line) lines.push(line);
      line = '';
      for (const { segment: ch } of graphemes.segment(word)) {
        if (line && !fits(line + ch)) {
          lines.push(line);
          line = '';
        }
        line += ch;
      }
    }
    lines.push(line);
  }
  const w = Math.max(0, ...lines.map((l) => m.width(l, font)));
  return { lines, font, w, h: text === '' ? 0 : lines.length * lineHeight(font.size) };
}

// labelBlock is textBlock for a node's label: the text after the first line break, if any, is
// wrapped and drawn in sub.
export function labelBlock(text: string, maxWidth: number, font: Font, sub: SubStyle | undefined, m: TextMeasure): TextBlock {
  const cut = text.indexOf('\n');
  if (!sub || cut < 0) return textBlock(text, maxWidth, font, m);
  const head = textBlock(text.slice(0, cut), maxWidth, font, m);
  const rest = textBlock(text.slice(cut + 1), maxWidth, sub.font, m);
  return {
    lines: [...head.lines, ...rest.lines],
    font,
    w: Math.max(head.w, rest.w),
    h: head.lines.length * lineHeight(font.size) + rest.lines.length * lineHeight(sub.font.size),
    sub: { ...sub, from: head.lines.length },
  };
}

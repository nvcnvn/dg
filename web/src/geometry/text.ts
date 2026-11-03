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

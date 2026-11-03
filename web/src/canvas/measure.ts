import { FONT_SIZE, type TextMeasure } from '../geometry/geometry';

export const FONT_FAMILY = 'Inter';
const font = `${FONT_SIZE}px ${FONT_FAMILY}`;

// loadFont loads the bundled font for every character in texts and fails if
// it cannot, so text is never measured with a system font.
export async function loadFont(texts: string[]): Promise<void> {
  const faces = await document.fonts.load(font, 'A' + texts.join(''));
  if (faces.length === 0) throw new Error(`bundled font ${FONT_FAMILY} did not load`);
}

export function canvasMeasure(): TextMeasure {
  const ctx = document.createElement('canvas').getContext('2d');
  if (!ctx) throw new Error('no 2D canvas context');
  ctx.font = font;
  const m = ctx.measureText('Hg');
  return {
    width: (text) => ctx.measureText(text).width,
    ascent: m.fontBoundingBoxAscent,
    descent: m.fontBoundingBoxDescent,
  };
}

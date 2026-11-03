import { drawnWeight, type Font, type TextMeasure } from '../geometry/text';
import type { ResolvedView } from '../protocol/view';

export const BUNDLED_FAMILY = 'Inter';
const PROJECT_FAMILY = 'dg-project';

const faces = new Map<string, FontFace>();

// FontChoice is the family text is measured and drawn with in the browser, and the name an
// exported SVG gives it.
export interface FontChoice {
  family: string;
  name: string;
}

// loadFonts loads the font for every text of the view, in each weight it is drawn with, and
// fails if it cannot, so text is never measured with a system font. A project font is loaded
// from its bytes in the view.
export async function loadFonts(view: ResolvedView): Promise<FontChoice> {
  let choice: FontChoice = { family: BUNDLED_FAMILY, name: BUNDLED_FAMILY };
  if (view.font) {
    let face = faces.get(view.font.data);
    if (!face) {
      const bytes = Uint8Array.from(atob(view.font.data), (c) => c.charCodeAt(0));
      face = new FontFace(PROJECT_FAMILY, bytes);
      faces.set(view.font.data, face);
      document.fonts.add(face);
    }
    await face.load();
    choice = { family: PROJECT_FAMILY, name: view.font.family };
  }
  const texts = new Map<number, string>();
  const add = (weight: number, text: string) => texts.set(weight, (texts.get(weight) ?? 'A') + text);
  for (const n of view.nodes) add(drawnWeight(n.style.font_weight), n.label);
  for (const e of view.edges) add(400, e.labels.mid ?? '');
  for (const [weight, text] of [...texts].sort((a, b) => a[0] - b[0])) {
    const loaded = await document.fonts.load(cssFont(choice.family, { size: 16, weight }), text);
    if (loaded.length === 0) throw new Error(`font ${choice.name} did not load`);
  }
  return choice;
}

function cssFont(family: string, f: Font): string {
  return `${f.weight} ${f.size}px "${family}"`;
}

export function canvasMeasure(family: string): TextMeasure {
  const ctx = document.createElement('canvas').getContext('2d');
  if (!ctx) throw new Error('no 2D canvas context');
  ctx.font = cssFont(family, { size: 100, weight: 400 });
  const m = ctx.measureText('Hg');
  const widths = new Map<string, number>();
  return {
    width(text, font) {
      const key = cssFont(family, font) + '\n' + text;
      let w = widths.get(key);
      if (w === undefined) {
        ctx.font = cssFont(family, font);
        w = ctx.measureText(text).width;
        widths.set(key, w);
      }
      return w;
    },
    ascent: m.fontBoundingBoxAscent / 100,
    descent: m.fontBoundingBoxDescent / 100,
  };
}

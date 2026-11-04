import { describe, expect, it } from 'vitest';
import app from './app.css?raw';
import theme from './theme.css?raw';

type Colors = Map<string, string>;

// declared returns the color tokens of the block that follows selector.
function declared(selector: string, pick: (value: string) => string | undefined): Colors {
  const at = theme.indexOf(selector);
  const body = theme.slice(at, theme.indexOf('\n}', at));
  const out: Colors = new Map();
  for (const [, name, value] of body.matchAll(/(--[\w-]+):\s*([^;]+);/g)) {
    const color = pick(value);
    if (color) out.set(name, color);
  }
  return out;
}

const hex = (v: string) => (/^#[0-9a-f]{6}$/i.test(v) ? v : undefined);
const side = (i: 0 | 1) => (v: string) => hex(/^light-dark\((#\w+), (#\w+)\)$/.exec(v)?.[i + 1] ?? v);

const LIGHT = declared(':root {', side(0));
const DARK = declared(':root {', side(1));
const CONTRAST = new Map([...DARK, ...declared(":root[data-theme='high-contrast']", hex)]);

function luminance(c: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const v = parseInt(c.slice(i, i + 2), 16) / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// pairs are the token pairs text is drawn with: every --text… on every --bg…, and every --on-x
// on --x.
function pairs(colors: Colors): [string, string][] {
  const names = [...colors.keys()];
  const on = names.filter((n) => n.startsWith('--on-') && !n.endsWith('-muted')).map((n): [string, string] => [n, '--' + n.slice('--on-'.length)]);
  const text = names.filter((n) => n === '--text' || n.startsWith('--text-'));
  const bg = names.filter((n) => n.startsWith('--bg-'));
  return [...text.flatMap((t) => bg.map((b): [string, string] => [t, b])), ...on, ['--on-paper-muted', '--paper']];
}

describe('theme', () => {
  it.each([
    ['light', LIGHT],
    ['dark', DARK],
    ['high-contrast', CONTRAST],
  ])('%s text meets 4.5:1', (_, colors) => {
    const list = pairs(colors);
    expect(list.length).toBeGreaterThan(30);
    for (const [fg, bg] of list) {
      expect(colors.has(bg), bg).toBe(true);
      expect(contrast(colors.get(fg)!, colors.get(bg)!), `${fg} on ${bg}`).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('keeps every color in theme.css', () => {
    expect(app.match(/#[0-9a-f]{3,8}\b|rgba?\(/gi)?.filter((m) => m !== 'rgba(') ?? []).toEqual([]);
    expect([...app.matchAll(/rgba?\([^)]*\)/g)].map((m) => m[0])).toEqual(['rgba(0, 0, 0, 0.4)']);
  });

  it('names only tokens it defines', () => {
    const defined = new Set([...theme.matchAll(/(--[\w-]+):/g)].map((m) => m[1]));
    for (const [, name] of app.matchAll(/var\((--[\w-]+)\)/g)) expect(defined.has(name), name).toBe(true);
  });
});

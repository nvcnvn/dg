import { describe, expect, it } from 'vitest';
import { toJSON, toSVG, type Drawing } from './export';

const drawing: Drawing = {
  view: 'main',
  bounds: { x: -20, y: -20, w: 340, h: 100 },
  ascent: 14,
  descent: 4,
  nodes: [
    { id: 'b', box: { x: 200, y: 0, w: 100, h: 40 }, label: 'B', container: false, style: { shape: 'rect', fill: '#FFF', stroke: '#000' } },
    { id: 'a"<&>', box: { x: 0, y: 0, w: 100, h: 40 }, label: 'A & <b>', container: false, style: { shape: 'rounded', fill: '#FFF', stroke: '#000' } },
  ],
  edges: [
    {
      id: 'e',
      points: [{ x: 100, y: 20 }, { x: 200, y: 20 }],
      arrow: [{ x: 190, y: 25 }, { x: 190, y: 15 }],
      label: { text: 'x', box: { x: 140, y: 10, w: 20, h: 20 } },
      style: { stroke: '#000', arrow_end: 'arrow' },
    },
  ],
};

describe('toSVG', () => {
  it('escapes text and attributes', () => {
    const svg = toSVG(drawing);
    expect(svg).toContain('data-node="a&quot;&lt;&amp;&gt;"');
    expect(svg).toContain('>A &amp; &lt;b&gt;</text>');
    expect(svg).toContain('rx="8"');
    expect(svg).toContain('stroke-width="1.5"');
    expect(svg).toContain('d="M100 20 L200 20"');
    expect(svg).toContain('d="M190 25 L200 20 L190 15"');
  });
});

describe('toJSON', () => {
  it('sorts nodes by ID and writes each point on one line', () => {
    const text = toJSON(drawing);
    expect(text).toContain('        [100, 20],\n        [200, 20]\n');
    const json = JSON.parse(text);
    expect(json.nodes.map((n: { id: string }) => n.id)).toEqual(['a"<&>', 'b']);
    expect(json.edges).toEqual([{ id: 'e', points: [[100, 20], [200, 20]] }]);
  });
});

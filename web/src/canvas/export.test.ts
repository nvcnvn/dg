import { describe, expect, it } from 'vitest';
import { edgeStyle, nodeStyle } from '../fixtures';
import type { NodeStyle } from '../protocol/view';
import { toJSON, toSVG, type Drawing } from './export';
import { flowDash, nodeMarkup, type DrawnNode } from './markup';

const font = { size: 14, weight: 400 };

function drawn(id: string, style: Partial<NodeStyle> = {}, label = id): DrawnNode {
  const box = { x: 0, y: 0, w: 100, h: 40 };
  return {
    id,
    box,
    container: false,
    pinned: true,
    tooSmall: false,
    style: nodeStyle(style),
    text: { lines: [label], font, w: 10, h: 20 },
    textBox: { x: 10, y: 10, w: 80, h: 20 },
    align: 'center',
    ports: [],
  };
}

const drawing: Drawing = {
  view: 'main',
  family: 'Inter',
  bounds: { x: -20, y: -20, w: 340, h: 100 },
  ascent: 0.9,
  descent: 0.2,
  shapes: {},
  nodes: [{ ...drawn('b'), box: { x: 200, y: 0, w: 100, h: 40 }, ports: [{ name: 'p', side: 'left', x: 200, y: 20 }] }, drawn('a"<&>', { shape: 'rounded' }, 'A & <b>')],
  edges: [
    {
      id: 'e',
      points: [{ x: 100, y: 20 }, { x: 200, y: 20 }],
      markers: [{ line: [{ x: 190, y: 25 }, { x: 200, y: 20 }, { x: 190, y: 15 }] }, { circle: { x: 104, y: 20 }, r: 4, filled: false }],
      labels: [{ text: { lines: ['x'], font, w: 8, h: 20 }, box: { x: 140, y: 10, w: 20, h: 20 } }],
      style: edgeStyle({ stroke_dash: [6, 4] }),
    },
  ],
};

describe('toSVG', () => {
  it('escapes text and attributes and draws the style', () => {
    const svg = toSVG(drawing);
    expect(svg).toContain('data-node="a&quot;&lt;&amp;&gt;"');
    expect(svg).toContain('>A &amp; &lt;b&gt;</text>');
    expect(svg).toContain('rx="8"');
    expect(svg).toContain('stroke-width="1.5"');
    expect(svg).toContain('stroke-dasharray="6 4"');
    expect(svg).toContain('d="M100 20 L200 20"');
    expect(svg).toContain('d="M190 25 L200 20 L190 15"');
    expect(svg).toContain('<circle cx="104" cy="20" r="4" fill="#FFFFFF" stroke="#3C4043" stroke-width="1.5"/>');
    expect(svg).toContain('font-family="Inter, sans-serif"');
  });
});

describe('nodeMarkup', () => {
  const m = { ascent: 0.9, descent: 0.2 };
  it.each([
    ['ellipse', '<ellipse'],
    ['diamond', '<path'],
    ['cylinder', ' A50 7 0 0 0'],
    ['person', '<circle'],
    ['bullseye', '<circle'],
    ['stadium', 'rx="20"'],
  ])('draws a %s', (shape, want) => {
    expect(nodeMarkup(drawn('n', { shape }), m, { shapes: {} }).join('')).toContain(want);
  });
  it('draws no outline for text and fades by opacity', () => {
    const out = nodeMarkup(drawn('n', { shape: 'text', opacity: 0.5 }), m, { shapes: {} }).join('');
    expect(out).not.toContain('<rect');
    expect(out).toContain('opacity="0.5"');
  });
  it('draws a nine-slice custom shape from its definition', () => {
    const shapes = { shapes: { gpu: { svg: '<svg viewBox="0 0 40 40"/>', text_box: [0, 0, 1, 1] as [number, number, number, number], stretch: 'nine-slice' as const } } };
    const out = nodeMarkup({ ...drawn('n'), custom: 'gpu' }, m, shapes).join('');
    expect(out.match(/<use href="#dg-shape-gpu"\/>/g)).toHaveLength(9);
    expect(toSVG({ ...drawing, nodes: [{ ...drawn('n'), custom: 'gpu' }], edges: [], ...shapes })).toContain('<image id="dg-shape-gpu"');
  });
});

describe('tables', () => {
  it('draws separators and each row key, name and right-aligned type', () => {
    const table = { lines: [30], rows: [{ name: 'id', key: 'PK', type: 'uuid', y: 42 }], keyX: 8, nameX: 40, typeX: 92, font, keyFont: { ...font, weight: 700 } };
    const out = nodeMarkup({ ...drawn('t', { shape: 'table' }), table }, { ascent: 0.9, descent: 0.2 }, { shapes: {} }).join('\n');
    expect(out).toContain('<path d="M0 30 L100 30"');
    expect(out).toMatch(/<text x="8" y="\d+" fill="#202124" font-size="14" font-weight="700">PK<\/text>/);
    expect(out).toMatch(/<text x="92" y="\d+" text-anchor="end"[^>]*>uuid<\/text>/);
    expect(JSON.parse(toJSON({ ...drawing, nodes: [{ ...drawn('t'), table }], edges: [] })).nodes[0].rows).toEqual([{ name: 'id', y: 42 }]);
  });
});

describe('toJSON', () => {
  it('sorts nodes by ID and writes ports and each point on one line', () => {
    const text = toJSON(drawing);
    expect(text).toContain('        [100, 20],\n        [200, 20]\n');
    const json = JSON.parse(text);
    expect(json.nodes.map((n: { id: string }) => n.id)).toEqual(['a"<&>', 'b']);
    expect(json.nodes[1].ports).toEqual([{ name: 'p', side: 'left', x: 200, y: 20 }]);
    expect(json.edges).toEqual([{ id: 'e', points: [[100, 20], [200, 20]] }]);
  });
});

describe('animated edges', () => {
  it.each([
    [[], [6, 4], 10],
    [[0, 0], [6, 4], 10],
    [[6, 3], [6, 3], 9],
    [[5], [5], 10],
    [[1.234, 2], [1.23, 2], 3.23],
  ])('dash %j is drawn as %j with period %d', (stroke_dash, dash, period) => {
    expect(flowDash(edgeStyle({ stroke_dash, animate: 'flow' }))).toEqual({ dash, period });
  });

  it('moves the dashes one period toward the end each half second, markers solid', () => {
    const e = drawing.edges[0];
    const svg = toSVG({ ...drawing, edges: [{ ...e, style: edgeStyle({ animate: 'flow' }) }, { ...e, id: 'f', style: edgeStyle({ animate: 'flow', stroke_dash: [5] }) }] });
    expect(svg).toContain('<style>\n@keyframes dg-flow-0 { from { stroke-dashoffset: 0; } to { stroke-dashoffset: -10px; } }\n.dg-flow-0 { animation: dg-flow-0 500ms linear infinite; }\n</style>');
    expect(svg).toContain('<path d="M100 20 L200 20" class="dg-flow-0" fill="none" stroke="#3C4043" stroke-width="1.5" stroke-dasharray="6 4"/>');
    expect(svg).toContain('class="dg-flow-0" fill="none" stroke="#3C4043" stroke-width="1.5" stroke-dasharray="5"/>');
    expect(svg.match(/@keyframes/g)).toHaveLength(1);
    expect(svg).toContain('<path d="M190 25 L200 20 L190 15" fill="none"');
    expect(toSVG(drawing)).not.toContain('<style>');
  });

  it('names animations by sorted period, whatever the period', () => {
    const e = drawing.edges[0];
    const svg = toSVG({ ...drawing, edges: [{ ...e, style: edgeStyle({ animate: 'flow', stroke_dash: [1e21, 1] }) }, { ...e, id: 'f', style: edgeStyle({ animate: 'flow', stroke_dash: [3, 1.5] }) }] });
    expect(svg).toContain('@keyframes dg-flow-0 { from { stroke-dashoffset: 0; } to { stroke-dashoffset: -4.5px; } }');
    expect(svg).toMatch(/@keyframes dg-flow-1 \{ from \{ stroke-dashoffset: 0; \} to \{ stroke-dashoffset: -\d+px; \} \}\n\.dg-flow-1 \{/);
    expect(svg).toMatch(/class="dg-flow-1"[^>]*stroke-dasharray="\d+ 1"/);
  });
});

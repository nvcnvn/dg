import type { Drawing } from './canvas/export';
import type { DrawnNode } from './canvas/markup';
import { scene, type Scene } from './edit/edit';
import type { Box } from './geometry/geometry';
import type { TextMeasure } from './geometry/text';
import type { ModelInfo } from './protocol/client';
import type { EdgeStyle, NodeStyle, Policy, ResolvedView, ViewEdge, ViewNode } from './protocol/view';

// Fixtures for unit tests: the generic pack's defaults and a text measure where every
// character is 8 pixels wide at 14 pixels.

// On Windows a file URL's path is /D:/…, which the file system would read as \D:\….
/** libavoidWasm is the path of libavoid's WebAssembly module, for the tests that route. */
export const libavoidWasm = decodeURIComponent(new URL('../node_modules/libavoid-js/dist/libavoid.wasm', import.meta.url).pathname).replace(/^\/(?=[A-Za-z]:)/, '');

export const text: TextMeasure = { width: (s, f) => (s.length * 8 * f.size) / 14, ascent: 0.9, descent: 0.2 };

export function nodeStyle(over: Partial<NodeStyle> = {}): NodeStyle {
  return {
    shape: 'rect',
    fill: '#FFFFFF',
    stroke: '#3C4043',
    stroke_width: 1,
    stroke_dash: [],
    corner_radius: 8,
    opacity: 1,
    text_color: '#202124',
    font_size: 14,
    font_weight: 400,
    padding: 10,
    min_w: 100,
    min_h: 40,
    max_w: 240,
    z: 0,
    ...over,
  };
}

export function edgeStyle(over: Partial<EdgeStyle> = {}): EdgeStyle {
  return { stroke: '#3C4043', stroke_width: 1.5, stroke_dash: [], arrow_start: 'none', arrow_end: 'arrow', routing: 'orthogonal', font_size: 12, ...over };
}

export function node(id: string, over: Partial<ViewNode> = {}, style: Partial<NodeStyle> = {}): ViewNode {
  return { id, kind: 'node', label: id, ...over, style: nodeStyle(style) };
}

export function edge(id: string, from: string, to: string, over: Partial<ViewEdge> = {}): ViewEdge {
  return { id, kind: 'default', from: { node: from }, to: { node: to }, labels: {}, style: edgeStyle(), ...over };
}

// A view on a grid of 10: a and b on the canvas, container g at (0, 200) with its content origin
// 20 in and 40 down, holding c; stack s at (400, 0) holding s1, s2, s3 one under another.
export function editScene(over: { pinned?: boolean; gPin?: { w?: number; h?: number }; model?: ModelInfo } = {}): Scene {
  const pinned = over.pinned ?? true;
  const pin = pinned ? { x: 0, y: 0 } : undefined;
  const stack: Policy = { layout: 'stack', direction: 'vertical', gap: 10, columns: 1, align: 'start', order: ['s1', 's2', 's3'] };
  const nodes: ViewNode[] = [
    node('a', { pin }),
    node('b', { pin }),
    node('g', { container: true, pin: pinned ? { x: 0, y: 200, ...over.gPin } : undefined }),
    node('c', { frame: 'g', pin }),
    node('s', { container: true, policy: stack, pin }),
    node('s1', { frame: 's' }),
    node('s2', { frame: 's' }),
    node('s3', { frame: 's', pin: { x: 0, y: 0, w: 120 } }),
  ];
  const view: ResolvedView = { view: 'main', grid: 10, nodes, edges: [edge('ab', 'a', 'b', { labels: { mid: 'x' } }), edge('ac', 'a', 'c', { hint: { to_side: 'top' } })] };
  const drawn = (id: string, box: Box, extra: Partial<DrawnNode> = {}): DrawnNode => ({
    id,
    box,
    container: false,
    pinned,
    tooSmall: false,
    overflows: false,
    style: nodeStyle(),
    text: null,
    textBox: box,
    align: 'center',
    ports: [],
    rel: { x: box.x, y: box.y },
    ...extra,
  });
  const drawing: Drawing = {
    view: 'main',
    family: 'Inter',
    bounds: { x: 0, y: 0, w: 600, h: 400 },
    ascent: 0.9,
    descent: 0.2,
    shapes: {},
    overlays: [],
    decorations: [],
    nodes: [
      drawn('a', { x: 0, y: 0, w: 100, h: 40 }),
      drawn('b', { x: 200, y: 100, w: 100, h: 40 }),
      drawn('g', { x: 0, y: 200, w: 200, h: 120 }, { container: true, origin: { x: 20, y: 240 } }),
      drawn('c', { x: 30, y: 250, w: 100, h: 40 }, { frame: 'g', rel: { x: 10, y: 10 } }),
      drawn('s', { x: 400, y: 0, w: 140, h: 200 }, { container: true, origin: { x: 410, y: 30 } }),
      drawn('s1', { x: 410, y: 30, w: 100, h: 40 }, { frame: 's', orderedIn: 's', rel: { x: 0, y: 0 }, pinned }),
      drawn('s2', { x: 410, y: 80, w: 100, h: 40 }, { frame: 's', orderedIn: 's', rel: { x: 0, y: 50 }, pinned }),
      drawn('s3', { x: 410, y: 130, w: 120, h: 40 }, { frame: 's', orderedIn: 's', rel: { x: 0, y: 100 }, pinned }),
    ],
    edges: [
      { id: 'ab', points: [{ x: 100, y: 20 }, { x: 150, y: 20 }, { x: 150, y: 120 }, { x: 200, y: 120 }], markers: [], labels: [], style: edgeStyle() },
      { id: 'ac', points: [{ x: 50, y: 40 }, { x: 50, y: 250 }], markers: [], labels: [], style: edgeStyle() },
    ],
  };
  return scene(view, drawing, over.model);
}

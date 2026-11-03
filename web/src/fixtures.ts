import type { TextMeasure } from './geometry/text';
import type { EdgeStyle, NodeStyle, ViewEdge, ViewNode } from './protocol/view';

// Fixtures for unit tests: the generic pack's defaults and a text measure where every
// character is 8 pixels wide at 14 pixels.

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

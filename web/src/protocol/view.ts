// Types of the resolved view Go sends (internal/view.Resolved).

export interface ResolvedView {
  view: string;
  grid: number;
  font?: { family: string; data: string };
  nodes: ViewNode[];
  edges: ViewEdge[];
  shapes?: Record<string, CustomShape>;
}

export type Side = 'top' | 'right' | 'bottom' | 'left';

export interface ViewNode {
  id: string;
  kind: string;
  label: string;
  frame?: string;
  container?: boolean;
  read_only?: boolean;
  ports?: Port[];
  compartments?: Compartment[];
  pin?: Pin;
  style: NodeStyle;
}

export interface Port {
  name: string;
  side: Side | 'auto';
}

export interface Compartment {
  name: string;
  rows: Row[];
}

export interface Row {
  name: string;
  type?: string;
  key?: string;
}

export interface Pin {
  in?: string;
  x: number;
  y: number;
  w?: number;
  h?: number;
  ports?: Record<string, { side: Side; offset?: number }>;
}

export interface NodeStyle {
  shape: string;
  fill: string;
  stroke: string;
  stroke_width: number;
  stroke_dash: number[];
  corner_radius: number;
  opacity: number;
  text_color: string;
  font_size: number;
  font_weight: number;
  icon?: string;
  image?: string;
  padding: number;
  min_w: number;
  min_h: number;
  max_w: number;
  z: number;
}

export interface ViewEdge {
  id: string;
  kind: string;
  from: End;
  to: End;
  labels: Labels;
  read_only?: boolean;
  hint?: Hint;
  style: EdgeStyle;
}

export interface Labels {
  start?: string;
  mid?: string;
  end?: string;
}

export interface End {
  node: string;
  port?: string;
  row?: string;
}

export interface Hint {
  from_side?: Side;
  to_side?: Side;
  via?: [number, number][];
  label_pos?: number;
}

export interface EdgeStyle {
  stroke: string;
  stroke_width: number;
  stroke_dash: number[];
  arrow_start: string;
  arrow_end: string;
  routing: 'orthogonal' | 'polyline' | 'straight';
  font_size: number;
}

export interface CustomShape {
  svg: string;
  text_box: [number, number, number, number];
  anchors?: Record<string, [number, number]>;
  stretch: 'nine-slice' | 'uniform' | 'none';
}

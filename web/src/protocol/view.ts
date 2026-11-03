// Types of the resolved view Go sends (internal/view.Resolved).

export interface ResolvedView {
  view: string;
  grid: number;
  // direction is the way the layers of the first layout run, when the view sets it.
  direction?: 'down' | 'right' | 'up' | 'left';
  // of is the container whose inside the view shows.
  of?: string;
  // breadcrumbs are the levels of views this view leads back to, outermost first.
  breadcrumbs?: string[][];
  // policy is set in a view whose of is a stack or grid container it does not draw: the
  // container's policy places the nodes on the canvas.
  policy?: Policy;
  font?: { family: string; data: string };
  nodes: ViewNode[];
  edges: ViewEdge[];
  overlays?: ViewOverlay[];
  decorations?: ViewDecoration[];
  shapes?: Record<string, CustomShape>;
}

export interface ViewOverlay {
  id: string;
  label?: string;
  members: string[];
  style: OverlayStyle;
}

export interface OverlayStyle {
  fill: string;
  stroke: string;
  stroke_width: number;
  stroke_dash: number[];
  opacity: number;
  padding: number;
}

// ViewDecoration is a decoration with every style property set. x and y are relative to the
// content origin of the frame in, or to the canvas; for an attached decoration they move the
// decoration's center from its node's center.
export interface ViewDecoration {
  id: string;
  type: 'text' | 'rect' | 'ellipse' | 'band' | 'image' | 'arrow' | 'freehand';
  in?: string;
  attach?: string;
  x: number;
  y: number;
  w?: number;
  h?: number;
  z: number;
  text?: string;
  image?: string;
  points?: [number, number][];
  path?: string;
  fill: string;
  stroke: string;
  stroke_width: number;
  stroke_dash: number[];
  opacity: number;
  text_color: string;
  font_size: number;
}

export type Side = 'top' | 'right' | 'bottom' | 'left';

export interface ViewNode {
  id: string;
  kind: string;
  label: string;
  frame?: string;
  container?: boolean;
  // collapsed marks a collapsed container, drawn as a leaf with a "+" badge.
  collapsed?: boolean;
  // drill lists the views that show the inside of this node.
  drill?: string[];
  read_only?: boolean;
  ports?: Port[];
  compartments?: Compartment[];
  pin?: Pin;
  // policy is set on an expanded stack or grid container.
  policy?: Policy;
  style: NodeStyle;
}

// Policy is how a stack or grid container places its drawn children (spec 7.3), which order
// lists in the order they take.
export interface Policy {
  container?: string;
  layout: 'stack' | 'grid';
  direction: 'horizontal' | 'vertical';
  gap: number;
  stretch?: boolean;
  columns: number;
  align: 'start' | 'center' | 'end';
  order: string[];
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

// Pin is a node's layout entry. A node a stack or grid container places has no position: x and
// y are 0 and only its size and ports apply.
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
  label_side?: 'left';
}

export interface ViewEdge {
  id: string;
  // edges are the model edges a lifted edge stands for.
  edges?: string[];
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
  animate?: 'flow';
}

export interface CustomShape {
  svg: string;
  text_box: [number, number, number, number];
  anchors?: Record<string, [number, number]>;
  stretch: 'nine-slice' | 'uniform' | 'none';
}

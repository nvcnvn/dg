// Types of the resolved view Go sends (internal/view.Resolved).

export interface ResolvedView {
  view: string;
  grid: number;
  nodes: ViewNode[];
  edges: ViewEdge[];
}

export interface ViewNode {
  id: string;
  kind: string;
  label: string;
  frame?: string;
  container?: boolean;
  style: NodeStyle;
}

export interface NodeStyle {
  shape: 'rect' | 'rounded';
  fill: string;
  stroke: string;
}

export interface ViewEdge {
  id: string;
  kind: string;
  from: { node: string };
  to: { node: string };
  labels: { mid?: string };
  style: EdgeStyle;
}

export interface EdgeStyle {
  stroke: string;
  arrow_end: string;
}

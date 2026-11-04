import type { Op } from './ops';
import type { Compartment, CustomShape, End, Labels, NodeStyle, Port, ResolvedView } from './view';

// Messages of the dg serve WebSocket protocol (internal/server).

export interface Diagnostic {
  severity: 'error' | 'warning' | 'info';
  summary: string;
  detail?: string;
  file?: string;
  line?: number;
  column?: number;
}

export interface ViewInfo {
  id: string;
  title?: string;
}

// Result is the result of one operation: the ID it created or changed, and for move and resize
// the values stored.
export interface Result {
  op: string;
  id?: string;
  x?: number;
  y?: number;
  w?: number;
  h?: number;
}

// ModelInfo is what the editor knows of the whole project to edit the model: the loaded packs in
// load order, the kinds and edge kinds they define, the tags in use, the custom shapes kinds use
// by default, and every node and edge, drawn or not.
export interface ModelInfo {
  packs?: string[];
  kinds: KindInfo[];
  edge_kinds: string[];
  tags: string[];
  shapes?: Record<string, CustomShape>;
  nodes: ModelNode[];
  edges: ModelEdge[];
}

export interface KindInfo {
  name: string;
  // pack names the pack that defines the kind; the project's own spec.hcl has none.
  pack?: string;
  // style is what a node of the kind looks like by default; only the model message has it.
  style?: NodeStyle;
  extends?: string;
  container?: boolean;
  layout?: string;
  rows?: boolean;
  ports?: string[];
  properties?: { name: string; type: 'string' | 'number' | 'bool'; default?: string | number | boolean }[];
}

// ModelNode is a node of the model; its ports are its instance ports.
export interface ModelNode {
  id: string;
  kind: string;
  label: string;
  parent?: string;
  tags?: string[];
  props?: Record<string, string | number | boolean>;
  ports?: Port[];
  compartments?: Compartment[];
  read_only?: boolean;
}

export interface ModelEdge {
  id: string;
  kind: string;
  from: End;
  to: End;
  labels: Labels;
  tags?: string[];
  read_only?: boolean;
}

// A model message comes before the snapshot that needs it, when the model changed. An ack or
// error answering a request carries the request's seq.
export type ServerMessage =
  | { type: 'model'; model: ModelInfo }
  | { type: 'snapshot'; views: ViewInfo[]; resolved: ResolvedView }
  | { type: 'diagnostics'; diagnostics: Diagnostic[] }
  | { type: 'history'; undo: boolean; redo: boolean }
  | { type: 'ack'; seq?: number; results: Result[] | null; files: string[]; warnings?: Diagnostic[] }
  | { type: 'error'; seq?: number; message: string; diagnostics?: Diagnostic[] };

export type Request = { type: 'op'; op: Op } | { type: 'undo' } | { type: 'redo' };

export interface Connection {
  selectView(id: string): void;
  // send sends a request and returns its seq, or undefined when the connection is closed.
  send(r: Request): number | undefined;
}

const RETRY_MS = 1000;

// connect opens the WebSocket of the server that served the page and reopens it when it
// closes. On every open it asks for the current view ("" for the server's default).
export function connect(onMessage: (m: ServerMessage) => void, onOpen: (open: boolean) => void): Connection {
  let view = '';
  let seq = 0;
  let ws: WebSocket | undefined;
  const send = () => ws?.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ type: 'select-view', view }));
  const open = () => {
    ws = new WebSocket(`ws://${location.host}/ws`);
    ws.onopen = () => {
      onOpen(true);
      send();
    };
    ws.onmessage = (e) => onMessage(JSON.parse(e.data) as ServerMessage);
    ws.onclose = () => {
      onOpen(false);
      setTimeout(open, RETRY_MS);
    };
  };
  open();
  return {
    selectView(id) {
      view = id;
      send();
    },
    send(r) {
      if (ws?.readyState !== WebSocket.OPEN) return undefined;
      ws.send(JSON.stringify({ ...r, seq: ++seq }));
      return seq;
    },
  };
}

import type { Op } from './ops';
import type { Compartment, CustomShape, End, Labels, NodeStyle, Port, ResolvedView } from './view';

// Messages of the editor session (internal/session), over a WebSocket from dg serve.

export interface Diagnostic {
  severity: 'error' | 'warning' | 'info';
  summary: string;
  detail?: string;
  file?: string;
  line?: number;
  column?: number;
  end_line?: number;
  end_column?: number;
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
// load order, the kinds and edge kinds they define, the tags in use, and every node and edge,
// drawn or not.
export interface ModelInfo {
  packs?: string[];
  kinds: KindInfo[];
  edge_kinds: string[];
  tags: string[];
  nodes: ModelNode[];
  edges: ModelEdge[];
}

// Pictures are what the palette draws and the model leaves out for its size: each kind's default
// icon and image, and the custom shapes kinds use by default.
export interface Pictures {
  kinds: Record<string, { icon?: string; image?: string }>;
  shapes: Record<string, CustomShape>;
}

export interface KindInfo {
  name: string;
  // pack names the pack that defines the kind; the project's own spec.hcl has none.
  pack?: string;
  // short is the name without the prefix of an installed pack, set only for such a kind.
  short?: string;
  // category is the kind's group within its pack in the palette.
  category?: string;
  // style is what a node of the kind looks like by default, without its pictures; only the model
  // message has it.
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

// A model message comes before the snapshot that needs it, when the model changed, and a pictures
// message before the first model and whenever the pictures changed. An ack or error answering a
// request carries the request's seq.
export type ServerMessage =
  | ({ type: 'pictures' } & Pictures)
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

// Connect opens a transport to the session; onOpen tells when it comes up and goes down.
export type Connect = (onMessage: (m: ServerMessage) => void, onOpen: (open: boolean) => void) => Connection;

const RETRY_MS = 1000;

// connect opens the WebSocket of the server that served the page and reopens it when it
// closes. On every open it asks for the current view ("" for the server's default).
export const connect: Connect = (onMessage, onOpen) => {
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
};

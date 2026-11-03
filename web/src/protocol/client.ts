import type { ResolvedView } from './view';

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

export type ServerMessage =
  | { type: 'snapshot'; views: ViewInfo[]; resolved: ResolvedView }
  | { type: 'diagnostics'; diagnostics: Diagnostic[] }
  | { type: 'error'; message: string };

export interface Connection {
  selectView(id: string): void;
}

const RETRY_MS = 1000;

// connect opens the WebSocket of the server that served the page and reopens it when it
// closes. On every open it asks for the current view ("" for the server's default).
export function connect(onMessage: (m: ServerMessage) => void, onOpen: (open: boolean) => void): Connection {
  let view = '';
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
  };
}

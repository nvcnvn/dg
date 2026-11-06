import type { Connect, Diagnostic, Request, ServerMessage } from '../protocol/client';
import type { Project } from './examples';

// PageMessage is what the page itself wants from Go: what only the playground sends
// (internal/playground), and the diagnostics, which the text editors show too.
export type PageMessage = { type: 'ready' } | { type: 'loaded' } | { type: 'files'; files: Project } | { type: 'zip'; seq?: number; data: string } | { type: 'diagnostics'; diagnostics: Diagnostic[] };

// Transport is the client's second transport (FR-19): the session's messages to and from Go in
// this page, through post and receive.
export interface Transport {
  connect: Connect;
  // receive takes a message from Go.
  receive(json: string): void;
  // load replaces the project; the editor then starts over on it.
  load(files: Project): void;
  // saveFile sends what a text editor holds.
  saveFile(path: string, text: string): void;
  // zip asks for the project as a .zip, in base64.
  zip(): Promise<string>;
}

// transport talks through post. Before each request it calls flush, so that an operation comes
// after what was typed before it. page gets the playground's own messages.
export function transport(post: (json: string) => void, flush: () => void, page: (m: PageMessage) => void): Transport {
  let toEditor: (m: ServerMessage) => void = () => {};
  let opened: (open: boolean) => void = () => {};
  let view = '';
  let seq = 0;
  let loaded = false;
  const zips = new Map<number, { resolve(data: string): void; reject(err: Error): void }>();
  const send = (m: object) => post(JSON.stringify(m));
  const select = () => send({ type: 'select-view', view });
  return {
    connect(onMessage, onOpen) {
      toEditor = onMessage;
      opened = onOpen;
      return {
        selectView(id) {
          view = id;
          if (loaded) select();
        },
        send(r: Request) {
          if (!loaded) return undefined;
          flush();
          send({ ...r, seq: ++seq });
          return seq;
        },
      };
    },
    receive(json) {
      const m = JSON.parse(json) as ServerMessage | PageMessage;
      switch (m.type) {
        case 'ready':
        case 'files':
          page(m);
          break;
        case 'loaded':
          loaded = true;
          view = '';
          opened(false);
          opened(true);
          select();
          page(m);
          break;
        case 'diagnostics':
          page(m);
          toEditor(m);
          break;
        case 'zip':
          zips.get(m.seq!)?.resolve(m.data);
          zips.delete(m.seq!);
          break;
        case 'error':
          if (m.seq !== undefined && zips.has(m.seq)) {
            zips.get(m.seq)!.reject(new Error(m.message));
            zips.delete(m.seq);
            break;
          }
          toEditor(m);
          break;
        default:
          toEditor(m);
      }
    },
    load(files) {
      send({ type: 'load', files });
    },
    saveFile(path, text) {
      send({ type: 'write-file', path, text });
    },
    zip() {
      flush();
      return new Promise((resolve, reject) => {
        zips.set(++seq, { resolve, reject });
        send({ type: 'zip', seq });
      });
    },
  };
}

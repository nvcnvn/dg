import type { Connect, Diagnostic, Request, ServerMessage } from '../protocol/client';
import type { Project } from './examples';

// PageMessage is what the page itself wants from Go (internal/playground).
export type PageMessage = { type: 'ready' } | { type: 'loaded' } | { type: 'files'; files?: Project } | { type: 'zip'; seq?: number; data: string } | { type: 'unzipped'; seq?: number; files: Project } | { type: 'diagnostics'; diagnostics: Diagnostic[] };

// Transport carries the session's messages to and from Go in this page (FR-19).
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
  // unzip asks for the files of a .zip in base64url.
  unzip(data: string): Promise<Project>;
}

// transport talks through post. Before each request it calls flush, so that an operation comes
// after what was typed before it. page gets the playground's own messages.
export function transport(post: (json: string) => void, flush: () => void, page: (m: PageMessage) => void): Transport {
  let toEditor: (m: ServerMessage) => void = () => {};
  let opened: (open: boolean) => void = () => {};
  let view = '';
  let seq = 0;
  let loaded = false;
  const waiting = new Map<number, { resolve(m: { data?: string; files?: Project }): void; reject(err: Error): void }>();
  const send = (m: object) => post(JSON.stringify(m));
  const ask = (m: object) =>
    new Promise<{ data?: string; files?: Project }>((resolve, reject) => {
      waiting.set(++seq, { resolve, reject });
      send({ ...m, seq });
    });
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
        case 'unzipped':
          waiting.get(m.seq!)?.resolve(m);
          waiting.delete(m.seq!);
          break;
        case 'error':
          if (m.seq !== undefined && waiting.has(m.seq)) {
            waiting.get(m.seq)!.reject(new Error(m.message));
            waiting.delete(m.seq);
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
      return ask({ type: 'zip' }).then((m) => m.data!);
    },
    unzip(data) {
      return ask({ type: 'unzip', data }).then((m) => m.files!);
    },
  };
}

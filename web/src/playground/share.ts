import { asProject, type Project } from './examples';

// LONG is the length from which a link may not survive a chat app.
export const LONG = 8000;

// MAX_BYTES bounds what a link can inflate to, since a short one can hold a great deal.
export const MAX_BYTES = 8_000_000;

const FRAGMENT = '#p=';

async function through(data: Uint8Array<ArrayBuffer>, stream: CompressionStream | DecompressionStream): Promise<Uint8Array<ArrayBuffer>> {
  const chunks: Uint8Array<ArrayBuffer>[] = [];
  let size = 0;
  const reader = new Blob([data]).stream().pipeThrough(stream).getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.length;
    if (size > MAX_BYTES) {
      await reader.cancel();
      throw new Error('too large');
    }
    chunks.push(value as Uint8Array<ArrayBuffer>);
  }
  return new Uint8Array(await new Blob(chunks).arrayBuffer());
}

export function base64url(data: Uint8Array): string {
  let binary = '';
  for (const b of data) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function bytes(text: string): Uint8Array<ArrayBuffer> {
  const binary = atob(text.replace(/-/g, '+').replace(/_/g, '/'));
  return Uint8Array.from(binary, (c) => c.charCodeAt(0));
}

// shareFragment returns the URL fragment that holds the files: sorted, as JSON, compressed, in
// base64url. The host never sees a fragment.
export async function shareFragment(files: Project): Promise<string> {
  const json = JSON.stringify(Object.fromEntries(Object.entries(files).sort(([a], [b]) => (a < b ? -1 : 1))));
  return FRAGMENT + base64url(await through(new TextEncoder().encode(json), new CompressionStream('deflate-raw')));
}

// sharedFiles returns the files of a URL fragment, undefined when it holds no project, and throws
// when it is damaged.
export async function sharedFiles(fragment: string): Promise<Project | undefined> {
  if (!fragment.startsWith(FRAGMENT)) return undefined;
  const json = await through(bytes(fragment.slice(FRAGMENT.length)), new DecompressionStream('deflate-raw'));
  return asProject(JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(json)));
}

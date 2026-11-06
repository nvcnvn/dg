import { describe, expect, it, vi } from 'vitest';
import type { ServerMessage } from '../protocol/client';
import { transport, type PageMessage } from './transport';

function open() {
  const sent: Record<string, unknown>[] = [];
  const page: PageMessage[] = [];
  const editor: ServerMessage[] = [];
  const up: boolean[] = [];
  const flush = vi.fn();
  const t = transport((json) => sent.push(JSON.parse(json)), flush, (m) => page.push(m));
  const conn = t.connect((m) => editor.push(m), (o) => up.push(o));
  return { t, conn, sent, page, editor, up, flush };
}

describe('the WebAssembly transport', () => {
  it('asks for the default view each time a project is loaded', () => {
    const { t, conn, sent, up } = open();
    conn.selectView('other');
    expect(sent).toEqual([]);
    t.receive('{"type":"loaded"}');
    t.receive('{"type":"loaded"}');
    expect(up).toEqual([false, true, false, true]);
    expect(sent).toEqual([
      { type: 'select-view', view: '' },
      { type: 'select-view', view: '' },
    ]);
  });

  it('sends nothing before a project is loaded', () => {
    const { conn, sent } = open();
    expect(conn.send({ type: 'undo' })).toBeUndefined();
    expect(sent).toEqual([]);
  });

  it('saves what was typed before a request', () => {
    const { t, conn, sent, flush } = open();
    t.receive('{"type":"loaded"}');
    flush.mockImplementation(() => t.saveFile('model.hcl', 'typed'));
    const seq = conn.send({ type: 'undo' });
    expect(sent.slice(1)).toEqual([
      { type: 'write-file', path: 'model.hcl', text: 'typed' },
      { type: 'undo', seq },
    ]);
  });

  it('gives each message to the page or to the editor', () => {
    const { t, page, editor } = open();
    t.receive('{"type":"files","files":{"model.hcl":"x"}}');
    t.receive('{"type":"history","undo":true,"redo":false}');
    expect(page).toEqual([{ type: 'files', files: { 'model.hcl': 'x' } }]);
    expect(editor).toEqual([{ type: 'history', undo: true, redo: false }]);
  });

  it('answers a zip by its seq', async () => {
    const { t, sent } = open();
    const first = t.zip();
    const second = t.zip();
    t.receive(JSON.stringify({ type: 'error', seq: sent[1].seq, message: 'no' }));
    t.receive(JSON.stringify({ type: 'zip', seq: sent[0].seq, data: 'UEs=' }));
    await expect(first).resolves.toBe('UEs=');
    await expect(second).rejects.toThrow('no');
  });
});

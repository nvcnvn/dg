import { describe, expect, it } from 'vitest';
import { shareFragment, sharedFiles } from './share';

describe('share links', () => {
  const files = { 'model.hcl': 'node "a" {\n  label = "Ålpha ✓ 日本"\n}\n', 'style.hcl': '', 'views/main.layout.yaml': 'version: 1\n' };

  it('loads files byte-identical to the ones shared', async () => {
    const link = await shareFragment(files);
    expect(link).toMatch(/^#p=[\w-]+$/);
    expect(await sharedFiles(link)).toEqual(files);
  });

  it('makes one link of the same files, whatever their order', async () => {
    const reversed = Object.fromEntries(Object.entries(files).reverse());
    expect(await shareFragment(reversed)).toBe(await shareFragment(files));
  });

  it('finds no project in another fragment', async () => {
    expect(await sharedFiles('')).toBeUndefined();
    expect(await sharedFiles('#top')).toBeUndefined();
  });

  it('refuses a damaged link', async () => {
    await expect(sharedFiles('#p=AAAA')).rejects.toThrow();
    await expect(sharedFiles('#p=!')).rejects.toThrow();
    const notFiles = await compress('{"model.hcl":1}');
    await expect(sharedFiles(notFiles)).rejects.toThrow('not a project');
    await expect(sharedFiles(await compress('{"dg.hcl":""}'))).rejects.toThrow('not a project');
    await expect(sharedFiles(await compress('{}'))).rejects.toThrow('not a project');
  });

  it('holds what the view gestures add', async () => {
    const more = { ...files, 'views.hcl': 'view "other" {}\n', 'views/other.layout.yaml': 'version: 1\n' };
    expect(await sharedFiles(await shareFragment(more))).toEqual(more);
  });

  it('refuses a link that inflates past its limit', async () => {
    await expect(sharedFiles(await compress(JSON.stringify({ 'model.hcl': ' '.repeat(9_000_000) })))).rejects.toThrow('too large');
  });
});

async function compress(json: string): Promise<string> {
  const stream = new Blob([json]).stream().pipeThrough(new CompressionStream('deflate-raw'));
  const data = new Uint8Array(await new Response(stream).arrayBuffer());
  return '#p=' + btoa(Array.from(data, (b) => String.fromCharCode(b)).join('')).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

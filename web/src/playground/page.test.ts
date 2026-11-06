import { expect, it } from 'vitest';
import serve from '../../index.html?raw';
import page from '../../playground.html?raw';

const parts = (html: string) => [/<main id="main">[^]*<\/main>/, /<div id="overflow-menu"[^]*<\/dialog>/].map((re) => re.exec(html)![0].replace(/\s+/g, ' '));

it('holds the same editor as the page of dg serve', () => {
  expect(parts(page)).toEqual(parts(serve));
});

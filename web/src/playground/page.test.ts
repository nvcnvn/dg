import { expect, it } from 'vitest';
import serve from '../../index.html?raw';
import page from '../../playground.html?raw';

const main = (html: string) => /<main id="main">[^]*<\/main>/.exec(html)![0].replace(/\s+/g, ' ');

it('holds the same editor as the page of dg serve', () => {
  expect(main(page)).toBe(main(serve));
});

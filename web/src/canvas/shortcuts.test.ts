import { expect, it } from 'vitest';
import doc from '../../../docs/reference/editor.md?raw';
import { SHORTCUTS } from './shortcuts';

it('is the table of the reference', () => {
  const table = doc.slice(doc.indexOf('## Keyboard shortcuts'));
  const rows = table
    .split('\n')
    .filter((l) => l.startsWith('| ') && !l.startsWith('| Key'))
    .map((l) =>
      l
        .slice(2, -2)
        .split(' | ')
        .map((c) => c.replaceAll('`', '')),
    );
  expect(rows).toEqual(SHORTCUTS);
});

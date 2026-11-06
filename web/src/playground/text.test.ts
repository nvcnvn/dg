import { Text } from '@codemirror/state';
import { describe, expect, it } from 'vitest';
import type { Diagnostic } from '../protocol/client';
import { marks, positionIn } from './text';

describe('positions of diagnostics', () => {
  it.each([
    ['ASCII', 'abcd nope', 'nope'],
    ['precomposed letters', 'Nguyễn nope', 'nope'],
    ['CJK', '日本語 nope', 'nope'],
    ['emoji', '😀😀😀 nope', 'nope'],
    ['combining marks', 'éé nope', 'nope'],
    ['joined emoji', '👨‍👩‍👧 nope', 'nope'],
  ])('counts characters a reader sees: %s', (_, line, want) => {
    const doc = Text.of(['# first', line]);
    const column = [...new Intl.Segmenter().segment(line)].findIndex((s) => s.index === line.indexOf('nope')) + 1;
    expect(doc.sliceString(positionIn(doc, 2, column))).toBe(want);
  });

  it('stays within the text', () => {
    const doc = Text.of(['ab']);
    expect(positionIn(doc, 1, 99)).toBe(2);
    expect(positionIn(doc, 9, 1)).toBe(2);
    expect(positionIn(doc, 0, 0)).toBe(0);
  });

  it('underlines the errors and warnings of the file, at least a character each', () => {
    const doc = Text.of(['node {', '}']);
    const all: Diagnostic[] = [
      { severity: 'error', summary: 'a', detail: 'b', file: 'model.hcl', line: 1, column: 6 },
      { severity: 'info', summary: 'c', file: 'model.hcl', line: 1, column: 1 },
      { severity: 'error', summary: 'd', file: 'style.hcl', line: 1, column: 1 },
      { severity: 'warning', summary: 'e', file: 'model.hcl' },
    ];
    expect(marks(doc, 'model.hcl', all)).toEqual([{ from: 5, to: 6, severity: 'error', message: 'a: b' }]);
  });
});

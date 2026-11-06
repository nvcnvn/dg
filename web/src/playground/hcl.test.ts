import { StringStream } from '@codemirror/language';
import { describe, expect, it } from 'vitest';
import { tokenizer } from './hcl';

// tokens returns the tokens of each line as [text, type], without spaces.
function tokens(src: string): [string, string | null][] {
  const state = tokenizer.startState!(2);
  const out: [string, string | null][] = [];
  for (const line of src.split('\n')) {
    const stream = new StringStream(line, 2, 2);
    while (!stream.eol()) {
      const type = tokenizer.token(stream, state);
      if (stream.current().trim()) out.push([stream.current(), type]);
      stream.start = stream.pos;
    }
  }
  return out;
}

describe('the HCL tokenizer', () => {
  it.each([
    ['a block', 'node "a" {', [['node', 'keyword'], ['"a"', 'string'], ['{', 'bracket']]],
    ['an attribute', '  label = "A \\" b"', [['label', 'propertyName'], ['=', 'operator'], ['"A \\" b"', 'string']]],
    ['a reference', 'from = node.a', [['from', 'propertyName'], ['=', 'operator'], ['node', 'variableName'], ['.', 'operator'], ['a', 'variableName']]],
    ['literals', 'x = [1, -2.5e3, true, null]', [['x', 'propertyName'], ['=', 'operator'], ['[', 'bracket'], ['1', 'number'], [',', 'operator'], ['-2.5e3', 'number'], [',', 'operator'], ['true', 'atom'], [',', 'operator'], ['null', 'atom'], [']', 'bracket']]],
    ['comments', '# a\n// b\nc /* d\ne */ f', [['# a', 'comment'], ['// b', 'comment'], ['c', 'keyword'], ['/*', 'comment'], [' d', 'comment'], ['e */', 'comment'], ['f', 'variableName']]],
    ['a heredoc', 'x = <<EOT\n  a "b\nEOT\ny = 1', [['x', 'propertyName'], ['=', 'operator'], ['<<EOT', 'string'], ['  a "b', 'string'], ['EOT', 'string'], ['y', 'propertyName'], ['=', 'operator'], ['1', 'number']]],
    ['an open string', '"abc', [['"abc', 'string']]],
  ] as [string, string, [string, string][]][])('%s', (_, src, want) => {
    expect(tokens(src)).toEqual(want);
  });
});

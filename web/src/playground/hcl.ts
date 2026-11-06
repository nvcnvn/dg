import { StreamLanguage, type StreamParser, type StringStream } from '@codemirror/language';

// State is where the tokenizer is between lines: in a /* comment, or a heredoc up to its marker.
interface State {
  comment: boolean;
  heredoc?: string;
}

const LITERALS = new Set(['true', 'false', 'null']);

function string(stream: StringStream): string {
  while (!stream.eol()) {
    const c = stream.next();
    if (c === '\\') stream.next();
    else if (c === '"') break;
  }
  return 'string';
}

function word(stream: StringStream): string {
  stream.eatWhile(/[\w-]/);
  const text = stream.current();
  if (LITERALS.has(text)) return 'atom';
  if (/^\s*=(?!=)/.test(stream.string.slice(stream.pos))) return 'propertyName';
  if (stream.string.slice(0, stream.start).trim() === '') return 'keyword';
  return 'variableName';
}

// tokenizer is the HCL of dg's files by the token names of StreamLanguage.
export const tokenizer: StreamParser<State> = {
  startState: () => ({ comment: false }),
  token(stream, state) {
    if (state.heredoc !== undefined) {
      if (stream.sol() && stream.string.trim() === state.heredoc) state.heredoc = undefined;
      stream.skipToEnd();
      return 'string';
    }
    if (state.comment) {
      if (stream.skipTo('*/')) {
        stream.pos += 2;
        state.comment = false;
      } else stream.skipToEnd();
      return 'comment';
    }
    if (stream.eatSpace()) return null;
    if (stream.match('#') || stream.match('//')) {
      stream.skipToEnd();
      return 'comment';
    }
    if (stream.match('/*')) {
      state.comment = true;
      return 'comment';
    }
    const heredoc = stream.match(/^<<-?([A-Za-z_]\w*)$/) as RegExpMatchArray | null;
    if (heredoc) {
      state.heredoc = heredoc[1];
      return 'string';
    }
    if (stream.eat('"')) return string(stream);
    if (stream.match(/^-?\d+(\.\d+)?([eE][+-]?\d+)?/)) return 'number';
    if (stream.match(/^[A-Za-z_]/, false)) return word(stream);
    stream.next();
    return /[{}[\]()]/.test(stream.current()) ? 'bracket' : 'operator';
  },
  languageData: { commentTokens: { line: '#', block: { open: '/*', close: '*/' } } },
};

export const hcl = StreamLanguage.define(tokenizer);

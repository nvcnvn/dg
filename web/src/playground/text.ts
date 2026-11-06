import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { lintGutter, setDiagnostics, type Diagnostic as Mark } from '@codemirror/lint';
import { Annotation, EditorState, Transaction, type Text } from '@codemirror/state';
import { EditorView, keymap, lineNumbers } from '@codemirror/view';
import { tags } from '@lezer/highlight';
import type { Diagnostic } from '../protocol/client';
import { hcl } from './hcl';

const fromGo = Annotation.define<boolean>();

const highlight = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--text-accent)', fontWeight: 'var(--weight-strong)' },
  { tag: tags.propertyName, color: 'var(--text-info)' },
  { tag: [tags.string, tags.number, tags.atom], color: 'var(--text-warning)' },
  { tag: tags.comment, color: 'var(--text-muted)', fontStyle: 'italic' },
]);

const theme = EditorView.theme({
  '&': { height: '100%', background: 'var(--bg-panel)', color: 'var(--text)' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace', fontSize: 'var(--type-m)', lineHeight: '1.5' },
  '.cm-content': { caretColor: 'var(--text)' },
  '.cm-cursor': { borderLeftColor: 'var(--text)' },
  '.cm-gutters': { background: 'var(--bg-panel)', color: 'var(--text-muted)', borderRight: '1px solid var(--border)' },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': { background: 'var(--bg-selected)' },
});

// positionIn returns the offset of a 1-based line and column of a diagnostic, within doc.
export function positionIn(doc: Text, line: number, column: number): number {
  if (line > doc.lines) return doc.length;
  const l = doc.line(Math.max(line, 1));
  return Math.min(l.from + Math.max(column, 1) - 1, l.to);
}

// marks returns the errors and warnings of a file as the text editor underlines them, each at
// least one character long so that it shows. Information stays in the diagnostics bar: a first
// look at an example should not show anything to fix.
export function marks(doc: Text, file: string, diagnostics: Diagnostic[]): Mark[] {
  return diagnostics
    .filter((d) => d.file === file && d.line && d.severity !== 'info')
    .map((d) => {
      const from = positionIn(doc, d.line!, d.column ?? 1);
      const to = d.end_line ? positionIn(doc, d.end_line, d.end_column ?? 1) : from;
      return { from: Math.min(from, doc.length), to: Math.min(Math.max(to, from + 1), doc.length), severity: d.severity, message: d.detail ? `${d.summary}: ${d.detail}` : d.summary };
    });
}

// TextEditor is one of the page's two text editors.
export interface TextEditor {
  text(): string;
  // show replaces the text with Go's, where it differs, keeping the cursor where it can.
  show(text: string): void;
  diagnostics(all: Diagnostic[]): void;
  // reveal moves the cursor to a line and column.
  reveal(line: number, column: number): void;
}

// textEditor makes an editor of file in parent; typed gets each change the visitor makes.
export function textEditor(parent: HTMLElement, file: string, label: string, typed: () => void): TextEditor {
  const view = new EditorView({
    parent,
    state: EditorState.create({
      extensions: [
        lineNumbers(),
        lintGutter(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        hcl,
        syntaxHighlighting(highlight),
        theme,
        EditorView.contentAttributes.of({ 'aria-label': label }),
        EditorView.updateListener.of((u) => {
          if (u.docChanged && !u.transactions.some((t) => t.annotation(fromGo))) typed();
        }),
      ],
    }),
  });
  return {
    text: () => view.state.doc.toString(),
    show(text) {
      const now = view.state.doc.toString();
      if (now === text) return;
      let from = 0;
      while (from < now.length && from < text.length && now[from] === text[from]) from++;
      let tail = 0;
      while (tail < now.length - from && tail < text.length - from && now[now.length - 1 - tail] === text[text.length - 1 - tail]) tail++;
      view.dispatch({ changes: { from, to: now.length - tail, insert: text.slice(from, text.length - tail) }, annotations: [fromGo.of(true), Transaction.addToHistory.of(false)] });
    },
    diagnostics(all) {
      view.dispatch(setDiagnostics(view.state, marks(view.state.doc, file, all)));
    },
    reveal(line, column) {
      const at = positionIn(view.state.doc, line, column);
      view.dispatch({ selection: { anchor: at }, effects: EditorView.scrollIntoView(at, { y: 'center' }) });
      view.focus();
    },
  };
}

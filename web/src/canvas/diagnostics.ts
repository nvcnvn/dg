import { ChevronDown, CircleAlert, Info, TriangleAlert } from 'lucide';
import type { Diagnostic } from '../protocol/client';
import { h, icon } from './dialog';

const ICONS = { error: CircleAlert, warning: TriangleAlert, info: Info };

// counted says how many errors and warnings there are: "2 errors, 1 warning".
export function counted(list: Diagnostic[]): string {
  const of = (severity: Diagnostic['severity'], name: string) => {
    const n = list.filter((d) => d.severity === severity).length;
    return n ? [`${n} ${name}${n === 1 ? '' : 's'}`] : [];
  };
  return [...of('error', 'error'), ...of('warning', 'warning')].join(', ') || 'No errors or warnings';
}

// diagnosticList lists ds, each with its severity's icon, its file and line, and its message; a
// click on one calls open, when given.
export function diagnosticList(ds: Diagnostic[], open?: (d: Diagnostic) => void): HTMLUListElement {
  return h(
    'ul',
    { class: 'diags' },
    ...ds.map((d) => {
      const parts = [
        icon(ICONS[d.severity], { 'aria-label': d.severity, role: 'img' }),
        h('span', { class: 'message' }, d.file ? h('span', { class: 'place' }, `${d.file}:${d.line}:${d.column}`) : null, d.summary + (d.detail ? `: ${d.detail}` : '')),
      ];
      const openable = open && d.file && !d.file.startsWith('<');
      return h('li', { class: d.severity }, openable ? h('button', { type: 'button', onclick: () => open(d) }, ...parts) : h('div', {}, ...parts));
    }),
  );
}

// diagnosticsBar shows the count of root's diagnostics, and lists them when expanded (FR-20);
// information is listed only after "Show information".
export function diagnosticsBar(root: HTMLElement, open?: (d: Diagnostic) => void) {
  let expanded = false;
  let showInfo = false;
  let list: Diagnostic[] = [];
  let note = '';

  const draw = () => {
    const infos = list.filter((d) => d.severity === 'info');
    const toggle = h(
      'button',
      { type: 'button', 'aria-expanded': String(expanded), onclick: () => ((expanded = !expanded), draw(), root.querySelector('button')?.focus()) },
      h('span', {}, counted(list)),
      note ? h('span', { class: 'note' }, note) : null,
      h('span', { class: 'chevron' }, icon(ChevronDown)),
    );
    const shown = showInfo ? list : list.filter((d) => d.severity !== 'info');
    root.replaceChildren(
      toggle,
      h(
        'div',
        { class: 'body', hidden: !expanded },
        diagnosticList(shown, open),
        infos.length && !showInfo ? h('button', { type: 'button', onclick: () => ((showInfo = true), draw()) }, 'Show information') : null,
      ),
    );
    root.hidden = list.length === 0;
  };

  return {
    show(next: Diagnostic[], text: string) {
      list = next;
      note = text;
      draw();
    },
  };
}

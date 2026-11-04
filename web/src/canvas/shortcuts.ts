import { h } from './dialog';

// SHORTCUTS is the table of keyboard shortcuts of docs/reference/editor.md, which a test keeps
// equal to this one.
export const SHORTCUTS: [string, string][] = [
  ['Ctrl/⌘+Z, Ctrl/⌘+Shift+Z, Ctrl+Y', 'undo, redo'],
  ['Ctrl/⌘+A', 'select every node'],
  ['Ctrl/⌘+C, Ctrl/⌘+V', 'copy, paste'],
  ['Delete, Backspace', 'delete the selection'],
  ['F2, Enter', 'edit the selected label'],
  ['C', 'connect mode'],
  ['U', 'unpin the selection'],
  ['+ or =, -, 0', 'zoom in, zoom out, actual size'],
  ['Shift+1, Shift+2', 'fit, zoom to selection'],
  ['Space+drag', 'pan'],
  ['Shift+F10, menu key', 'context menu'],
  ['?', 'this list of shortcuts'],
  ['Escape', 'cancel a drag, leave connect mode, close a menu, or else clear the selection'],
];

// shortcutSheet is the table the `?` dialog shows.
export function shortcutSheet(): HTMLTableElement {
  return h(
    'table',
    { id: 'shortcuts' },
    h('thead', {}, h('tr', {}, h('th', {}, 'Key'), h('th', {}, 'Does'))),
    h('tbody', {}, ...SHORTCUTS.map(([key, does]) => h('tr', {}, h('td', {}, h('kbd', {}, key)), h('td', {}, does)))),
  );
}

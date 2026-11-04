import type { Diagnostic } from '../protocol/client';

const THEMES = ['light', 'dark', 'high-contrast'];

const params = new URLSearchParams(location.search);

// inPanel reports whether the page is in the VS Code panel, whose extension opens it with
// host=vscode.
export const inPanel = params.get('host') === 'vscode' && window.parent !== window;

const setTheme = (theme: unknown) => {
  if (typeof theme === 'string' && THEMES.includes(theme)) document.documentElement.dataset.theme = theme;
};

// followHost takes the theme from the VS Code panel: its kind when the page opens, in the URL,
// and in a message each time it changes. Elsewhere the page follows the system's.
export function followHost() {
  if (!inPanel) return;
  setTheme(params.get('theme'));
  window.addEventListener('message', (ev) => {
    if (ev.source === window.parent && ev.data?.type === 'dg-theme') setTheme(ev.data.theme);
  });
}

// openDiagnostic asks the VS Code panel to open the file of d at its position.
export function openDiagnostic(d: Diagnostic) {
  window.parent.postMessage({ type: 'dg-open', file: d.file, line: d.line, column: d.column }, '*');
}

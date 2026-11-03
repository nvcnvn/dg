import type { Box } from '../geometry/geometry';

// editInPlace edits value in a text area over box, in page pixels, at fontSize pixels, and calls
// done with the text once Enter is pressed or the focus leaves, or with undefined on Escape.
// Shift+Enter breaks the line.
export function editInPlace(box: Box, value: string, fontSize: number, label: string, done: (text: string | undefined) => void) {
  const ta = document.createElement('textarea');
  ta.className = 'dg-inline';
  ta.value = value;
  ta.setAttribute('aria-label', label);
  Object.assign(ta.style, { left: `${box.x}px`, top: `${box.y}px`, width: `${Math.max(box.w, 120)}px`, height: `${Math.max(box.h, 2.2 * fontSize)}px`, fontSize: `${fontSize}px` });
  document.body.appendChild(ta);
  ta.focus();
  ta.select();
  let finished = false;
  const finish = (ok: boolean) => {
    if (finished) return;
    finished = true;
    const text = ta.value;
    ta.remove();
    done(ok ? text : undefined);
  };
  ta.addEventListener('keydown', (ev) => {
    ev.stopPropagation();
    if (ev.key === 'Enter' && !ev.shiftKey) {
      ev.preventDefault();
      finish(true);
    } else if (ev.key === 'Escape') {
      ev.preventDefault();
      finish(false);
    }
  });
  ta.addEventListener('blur', () => finish(true));
}

// DELAY is how long, in milliseconds, the pointer rests on a control before its tooltip shows.
const DELAY = 400;
const GAP = 6;

// tooltips shows, beside whatever the pointer rests on or the keyboard focus is on, the text of
// its data-tip attribute, or what text gives for it. A disabled control keeps its tooltip, so
// controls that can be disabled use aria-disabled and stay focusable.
export function tooltips(tip: HTMLElement, text: (el: Element) => string | undefined = () => undefined) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let shown: Element | undefined;

  const find = (t: EventTarget | null): [Element, string] | undefined => {
    for (let el = t instanceof Element ? t : null; el; el = el.parentElement) {
      const s = el.getAttribute('data-tip') ?? text(el);
      if (s) return [el, s];
    }
    return undefined;
  };
  const hide = () => {
    clearTimeout(timer);
    shown?.removeAttribute('aria-describedby');
    shown = undefined;
    tip.hidden = true;
  };
  const show = (el: Element, s: string) => {
    tip.textContent = s;
    tip.hidden = false;
    shown = el;
    el.setAttribute('aria-describedby', tip.id);
    const r = el.getBoundingClientRect();
    const w = tip.offsetWidth;
    const h = tip.offsetHeight;
    const below = r.bottom + GAP + h <= window.innerHeight;
    tip.style.left = `${Math.max(GAP, Math.min(r.left + r.width / 2 - w / 2, window.innerWidth - w - GAP))}px`;
    tip.style.top = `${below ? r.bottom + GAP : r.top - GAP - h}px`;
  };

  document.addEventListener('pointerover', (ev) => {
    const hit = find(ev.target);
    if (hit?.[0] === shown) return;
    hide();
    if (hit && !ev.buttons) timer = setTimeout(() => show(...hit), DELAY);
  });
  document.addEventListener('pointerdown', hide);
  document.addEventListener('keydown', (ev) => ev.key === 'Escape' && hide());
  document.addEventListener('focusin', (ev) => {
    hide();
    const hit = find(ev.target);
    if (hit && (ev.target as Element).matches(':focus-visible')) show(...hit);
  });
  document.addEventListener('focusout', hide);
  return { hide };
}

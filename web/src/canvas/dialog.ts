// Small DOM helpers and the editor's modal dialogs, built on the native <dialog> element, which
// keeps the focus inside, closes on Escape and gives the focus back when it closes.

type Child = Node | string | null | undefined | false;

// h makes an element with attributes (a function for on… handlers, true for a bare attribute)
// and children.
export function h<K extends keyof HTMLElementTagNameMap>(tag: K, attrs: Record<string, unknown> = {}, ...children: Child[]): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === false || v === null) continue;
    if (typeof v === 'function') el.addEventListener(k.slice(2), v as EventListener);
    else if (k === 'value') (el as HTMLInputElement).value = String(v);
    else if (k === 'checked') (el as HTMLInputElement).checked = !!v;
    else el.setAttribute(k, v === true ? '' : String(v));
  }
  for (const c of children) if (c) el.append(c);
  return el;
}

let ids = 0;

// labeled returns a label and the control it names.
export function labeled(text: string, control: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement): [HTMLLabelElement, typeof control] {
  control.id ||= `dg-field-${++ids}`;
  return [h('label', { for: control.id }, text), control];
}

// Field is a field of a dialog: a text input, a text area or a choice of options ([value, text]).
export interface Field {
  name: string;
  label: string;
  value?: string;
  type?: 'text' | 'textarea' | 'select';
  options?: [string, string][];
  help?: string;
  required?: boolean;
}

const dialog = () => document.getElementById('dialog') as HTMLDialogElement;

// ask shows a dialog of fields and resolves with their values when it is submitted, or with
// undefined when it is cancelled.
export function ask(title: string, fields: Field[], submit = 'OK'): Promise<Record<string, string> | undefined> {
  const d = dialog();
  const controls = fields.map((f) => {
    const c =
      f.type === 'select'
        ? h('select', { name: f.name }, ...(f.options ?? []).map(([v, t]) => h('option', { value: v, selected: v === f.value }, t)))
        : f.type === 'textarea'
          ? h('textarea', { name: f.name, rows: 4, value: f.value ?? '' })
          : h('input', { name: f.name, type: 'text', value: f.value ?? '', required: f.required, autocomplete: 'off', spellcheck: 'false' });
    return { f, c };
  });
  const form = h(
    'form',
    { method: 'dialog' },
    heading(d, title),
    ...controls.flatMap(({ f, c }) => [...labeled(f.label, c), f.help ? h('small', {}, f.help) : null]),
    // OK comes first, so Enter in a field submits with it; the buttons are drawn in reverse.
    h('div', { class: 'buttons' }, h('button', { type: 'submit', value: 'ok' }, submit), h('button', { type: 'submit', value: 'cancel', formnovalidate: true }, 'Cancel')),
  );
  d.replaceChildren(form);
  return open(d, () => Object.fromEntries(controls.map(({ f, c }) => [f.name, c.value])));
}

// choose shows a dialog that lists items, filtered by what is typed, and resolves with the value
// of the one picked, or undefined.
export function choose(title: string, label: string, items: { value: string; text: string }[]): Promise<string | undefined> {
  const d = dialog();
  const search = h('input', { type: 'search', autocomplete: 'off' });
  const list = h('div', { class: 'choices', role: 'listbox', 'aria-label': title });
  let picked: string | undefined;
  const fill = () => {
    const q = search.value.toLowerCase();
    const shown = items.filter((i) => i.text.toLowerCase().includes(q)).slice(0, 100);
    list.replaceChildren(
      ...shown.map((i) =>
        h(
          'button',
          {
            type: 'submit',
            value: 'ok',
            role: 'option',
            'data-value': i.value,
            onclick: () => {
              picked = i.value;
            },
          },
          i.text,
        ),
      ),
      ...(shown.length ? [] : [h('small', {}, 'Nothing matches.')]),
    );
  };
  search.addEventListener('input', fill);
  search.addEventListener('keydown', (ev) => {
    if (ev.key !== 'Enter') return;
    ev.preventDefault();
    const first = list.querySelector('button');
    if (first) {
      picked = first.dataset.value;
      d.close('ok');
    }
  });
  fill();
  const form = h('form', { method: 'dialog' }, heading(d, title), ...labeled(label, search), list, h('div', { class: 'buttons' }, h('button', { type: 'submit', value: 'cancel' }, 'Cancel')));
  d.replaceChildren(form);
  return open(d, () => picked);
}

// heading returns the title of dialog d, which names it.
function heading(d: HTMLDialogElement, title: string): HTMLHeadingElement {
  d.setAttribute('aria-labelledby', 'dg-dialog-title');
  return h('h2', { id: 'dg-dialog-title' }, title);
}

function open<T>(d: HTMLDialogElement, result: () => T): Promise<T | undefined> {
  d.returnValue = '';
  d.showModal();
  (d.querySelector('input, select, textarea') as HTMLElement | null)?.focus();
  return new Promise((resolve) => {
    d.addEventListener('close', () => resolve(d.returnValue === 'ok' ? result() : undefined), { once: true });
  });
}

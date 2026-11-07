import { viewLabelOp, type Scene } from '../edit/edit';
import type { Point } from '../geometry/geometry';
import { ChevronRight, X } from 'lucide';
import { paletteGroups, type PaletteCategory } from '../edit/palette';
import type { KindInfo, ModelInfo, ModelNode, Pictures } from '../protocol/client';
import type { Op } from '../protocol/ops';
import type { Compartment } from '../protocol/view';
import type { ActionHost, Actions, Selection } from './actions';
import { h, icon, labeled } from './dialog';
import { kindPicture } from './markup';

// DRAG starts dragging a kind out of the palette after the pointer moved this many pixels.
const DRAG = 3;

// palette lists the kinds of the loaded packs, each with a picture of its shape in its default
// style, by pack and category (FR-20); a closed category draws nothing until it opens. A kind
// dragged onto the canvas, or into a container, creates a node there (drop); one chosen by click
// or keyboard creates it in the middle of what the canvas shows (pick).
export function palette(root: HTMLElement, pick: (kind: string) => void, drop: (kind: string, client: Point) => void) {
  const list = root.querySelector('#kinds') as HTMLElement;
  const filter = root.querySelector('#palette-filter') as HTMLInputElement;
  let shown: ModelInfo | undefined;
  let pictures: Pictures = { kinds: {}, shapes: {} };
  let enabled = false;
  const opened = new Map<string, boolean>();
  const entry = (k: KindInfo) => {
    const b = h('button', { type: 'button', 'data-kind': k.name, disabled: !enabled, 'data-tip': `Drag onto the canvas to create a ${k.name}, or click to add it in the middle` }, picture(k, pictures), h('span', {}, k.short ?? k.name));
    b.addEventListener('click', () => {
      if (!b.dataset.dragged) pick(k.name);
      delete b.dataset.dragged;
    });
    b.addEventListener('pointerdown', (ev) => dragKind(b, k.name, ev));
    return b;
  };
  // category shows a category that opens and closes; what it opens or closes under a filter lasts
  // only as long as the filter.
  const category = (c: PaletteCategory, filtered: boolean) => {
    const d = h('details', { open: c.open }, h('summary', {}, icon(ChevronRight), h('span', {}, c.title), h('span', { class: 'count' }, String(c.kinds.length))));
    if (c.open) d.append(...c.kinds.map(entry));
    d.addEventListener('toggle', () => {
      if (d.open && d.childElementCount === 1) d.append(...c.kinds.map(entry));
      if (!filtered) opened.set(c.key, d.open);
    });
    return d;
  };
  const fill = () => {
    const q = filter.value;
    const packs = shown ? paletteGroups(shown, q, opened) : [];
    list.replaceChildren(...packs.flatMap((p) => [h('h3', {}, p.title), ...p.categories.flatMap((c): Node[] => (c.title ? [category(c, !!q)] : c.kinds.map(entry)))]));
  };
  const dragKind = (b: HTMLButtonElement, kind: string, ev: PointerEvent) => {
    if (ev.button !== 0 || b.disabled) return;
    const start = { x: ev.clientX, y: ev.clientY };
    let ghost: HTMLElement | undefined;
    b.setPointerCapture(ev.pointerId);
    const move = (e: PointerEvent) => {
      if (!ghost && Math.hypot(e.clientX - start.x, e.clientY - start.y) < DRAG) return;
      ghost ??= document.body.appendChild(h('div', { class: 'dg-ghost-kind' }, kind));
      ghost.style.left = `${e.clientX + 8}px`;
      ghost.style.top = `${e.clientY + 8}px`;
    };
    const up = (e: PointerEvent) => {
      b.removeEventListener('pointermove', move);
      b.removeEventListener('pointerup', up);
      b.removeEventListener('pointercancel', up);
      if (!ghost) return;
      ghost.remove();
      b.dataset.dragged = '1';
      if (e.type === 'pointerup') drop(kind, { x: e.clientX, y: e.clientY });
    };
    b.addEventListener('pointermove', move);
    b.addEventListener('pointerup', up);
    b.addEventListener('pointercancel', up);
  };
  filter.addEventListener('input', fill);
  return {
    update(next: ModelInfo | undefined, pics: Pictures, on: boolean) {
      const changed = next !== shown || pics !== pictures;
      if (on === enabled && !changed) return;
      enabled = on;
      shown = next;
      pictures = pics;
      if (changed) fill();
      else for (const b of list.querySelectorAll('button')) b.disabled = !on;
    },
  };
}

// picture is the palette's picture of kind k.
function picture(k: KindInfo, p: Pictures): Element | null {
  if (!k.style) return null;
  return new DOMParser().parseFromString(kindPicture({ ...k.style, ...p.kinds[k.name] }, p.shapes), 'image/svg+xml').documentElement;
}

// inspector shows the selection (FR-11): a node's ID, label, kind, tags, properties typed by its
// kind, ports and rows; an edge's kind, labels and tags; a decoration's fields. Each change is
// sent as one operation. Elements in read-only files show a lock and cannot change.
export function inspector(root: HTMLElement, host: ActionHost, act: Actions) {
  let shown = '';
  let next: { s: Scene | undefined; sel: Selection; locked: boolean } | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const send = (op: Op) => host.editable() && host.send(op);

  const field = (text: string, control: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement, commit: (v: string) => void) => {
    control.addEventListener('change', () => commit(control.value));
    if (control instanceof HTMLInputElement) control.addEventListener('keydown', (ev) => ev.key === 'Enter' && control.blur());
    return labeled(text, control);
  };

  // head titles the panel with what is selected and its kind, its ID beneath.
  const head = (title: string, kind: string, id: string, rename?: () => void) => [
    h('h2', {}, title.split('\n')[0]),
    h('p', { class: 'kind' }, kind),
    h('div', { class: 'identity' }, h('code', { class: 'id' }, id), rename ? h('button', { type: 'button', onclick: rename }, 'Rename ID…') : null),
  ];

  // tagsField edits tags as chips, completed from the project's tags.
  const tagsField = (s: Scene, tags: string[], commit: (tags: string[]) => void) => {
    const input = h('input', { list: 'dg-tags', placeholder: 'Add a tag' });
    const add = () => {
      const added = words(input.value).filter((t) => !tags.includes(t));
      input.value = '';
      if (added.length) commit([...tags, ...added]);
    };
    input.addEventListener('keydown', (ev) => {
      if (ev.key !== 'Enter') return;
      ev.preventDefault();
      add();
    });
    const chips = h(
      'div',
      { class: 'chips' },
      ...tags.map((t) => h('span', { class: 'chip' }, t, h('button', { type: 'button', 'aria-label': `Remove tag ${t}`, onclick: () => commit(tags.filter((x) => x !== t)) }, icon(X)))),
    );
    const [label, control] = field('Tags', input, add);
    return [label, chips, control, h('datalist', { id: 'dg-tags' }, ...s.model.tags.map((t) => h('option', { value: t })))];
  };

  // viewLabelField edits the label this view draws the node with; emptied, the view draws the
  // model's again.
  const viewLabelField = (s: Scene, n: ModelNode) => {
    const own = s.view.nodes.find((x) => x.id === n.id);
    if (!own) return [];
    const value = own.view_label ? own.label : '';
    return field('Label in this view', h('textarea', { rows: 2, value, placeholder: n.label }), (v) => v !== value && send(viewLabelOp(s, n.id, v)));
  };

  const nodeView = (s: Scene, n: ModelNode) => {
    const kinds = s.model.kinds;
    const kind = kinds.find((k) => k.name === n.kind);
    const ro = !!n.read_only;
    const out: (Node | null)[] = [
      ...head(n.label || n.id, n.kind, n.id, () => act.rename('node', n.id)),
      ro ? h('p', { class: 'lock' }, 'Read-only: defined in a read-only file, so only its layout can change.') : null,
      h('h3', {}, 'Text'),
      ...field('Label', h('textarea', { rows: 2, value: n.label }), (v) => v !== n.label && send({ op: 'set_label', id: n.id, label: v })),
      ...viewLabelField(s, n),
      h('h3', {}, 'Kind and tags'),
      ...field('Kind', h('select', {}, ...kinds.map((k) => h('option', { value: k.name, selected: k.name === n.kind }, k.name))), (v) => send({ op: 'set_kind', id: n.id, kind: v })),
      ...tagsField(s, n.tags ?? [], (tags) => send({ op: 'set_tags', id: n.id, tags })),
    ];
    const props = kind?.properties ?? [];
    if (props.length) {
      out.push(h('h3', {}, 'Properties'));
      const values = { ...n.props };
      const commit = () => send({ op: 'set_props', id: n.id, props: values });
      for (const p of props) {
        const def = p.default === undefined ? '' : String(p.default);
        if (p.type === 'bool') {
          const sel = h('select', {}, h('option', { value: '' }, def ? `default (${def})` : 'unset'), h('option', { value: 'true', selected: values[p.name] === true }, 'true'), h('option', { value: 'false', selected: values[p.name] === false }, 'false'));
          out.push(...field(p.name, sel, (v) => (v ? (values[p.name] = v === 'true') : delete values[p.name], commit())));
          continue;
        }
        const input = h('input', { type: p.type === 'number' ? 'number' : 'text', value: values[p.name] === undefined ? '' : String(values[p.name]), placeholder: def && `default ${def}` });
        out.push(
          ...field(p.name, input, (v) => {
            if (v === '') delete values[p.name];
            else values[p.name] = p.type === 'number' ? Number(v) : v;
            commit();
          }),
        );
      }
    }
    out.push(h('h3', {}, 'Ports'));
    const kindPorts = kind?.ports ?? [];
    if (kindPorts.length) out.push(h('p', { class: 'note' }, `From the kind: ${kindPorts.join(', ')}`));
    for (const p of n.ports ?? []) {
      out.push(h('div', { class: 'row' }, h('span', {}, `${p.name}${p.side ? ` (${p.side})` : ''}`), h('button', { type: 'button', 'aria-label': `Remove port ${p.name}`, onclick: () => send({ op: 'remove_port', id: n.id, name: p.name }) }, 'Remove')));
    }
    out.push(h('div', { class: 'actions' }, h('button', { type: 'button', onclick: () => act.addPort(n.id) }, 'Add port…')));
    if (kind?.rows) out.push(...rowsEditor(n, (compartments) => send({ op: 'set_rows', id: n.id, compartments })));
    out.push(
      h(
        'div',
        { class: 'actions last' },
        h('button', { type: 'button', onclick: () => act.hide([n.id]) }, 'Hide from view'),
        h('button', { type: 'button', class: 'danger', onclick: () => act.remove({ nodes: [n.id] }) }, 'Delete'),
      ),
    );
    return wrap(out, ro, ['Hide from view']);
  };

  const edgeView = (s: Scene, id: string) => {
    const ve = s.view.edges.find((e) => e.id === id);
    const e = s.model.edges.find((x) => x.id === id);
    if (!e) return [...head('Lifted edge', ve?.kind ?? '', id), h('p', { class: 'note' }, `Stands for ${(ve?.edges ?? []).join(', ')}; edit those in a view that draws them.`)];
    const ro = !!e.read_only;
    const labels = { ...e.labels };
    const label = (pos: 'start' | 'mid' | 'end', text: string) =>
      field(text, h('input', { value: labels[pos] ?? '' }), (v) => {
        if (v) labels[pos] = v;
        else delete labels[pos];
        send({ op: 'set_edge_labels', id, ...(Object.keys(labels).length ? { labels } : {}) });
      });
    return wrap(
      [
        ...head(e.labels.mid || `${e.from.node} → ${e.to.node}`, `${e.kind} edge`, id, () => act.rename('edge', id)),
        ro ? h('p', { class: 'lock' }, 'Read-only: defined in a read-only file, so only its hints can change.') : null,
        h('p', { class: 'note' }, `${endText(e.from)} → ${endText(e.to)}`),
        h('h3', {}, 'Text'),
        ...label('mid', 'Label'),
        ...label('start', 'Start label'),
        ...label('end', 'End label'),
        h('h3', {}, 'Kind and tags'),
        ...field('Kind', h('select', {}, ...s.model.edge_kinds.map((k) => h('option', { value: k, selected: k === e.kind }, k))), (v) => send({ op: 'set_kind', type: 'edge', id, kind: v })),
        ...tagsField(s, e.tags ?? [], (tags) => send({ op: 'set_tags', type: 'edge', id, tags })),
        h('div', { class: 'actions last' }, h('button', { type: 'button', class: 'danger', onclick: () => act.remove({ nodes: [], edge: id }) }, 'Delete')),
      ],
      ro,
    );
  };

  const decorationView = (s: Scene, id: string) => {
    const d = s.view.decorations?.find((x) => x.id === id);
    if (!d) return [];
    const view = s.view.view;
    const out: (Node | null)[] = [...head(d.text || d.type, `${d.type} decoration`, id), d.attach ? h('p', { class: 'note' }, `Attached to ${d.attach}`) : null];
    if (d.type === 'text') out.push(h('h3', {}, 'Text'), ...field('Text', h('textarea', { rows: 3, value: d.text ?? '' }), (v) => v && send({ op: 'update_decoration', view, id, decoration: { text: v } })));
    const promotable = !d.attach && d.type !== 'arrow' && d.type !== 'freehand';
    out.push(
      h(
        'div',
        { class: 'actions' },
        d.attach ? h('button', { type: 'button', onclick: () => act.detach(id) }, 'Detach') : h('button', { type: 'button', onclick: () => act.attach(id) }, 'Attach to node…'),
        promotable ? h('button', { type: 'button', onclick: () => act.promote(id) }, 'Promote to node…') : null,
      ),
      h('div', { class: 'actions last' }, h('button', { type: 'button', class: 'danger', onclick: () => act.remove({ nodes: [], decoration: id }) }, 'Delete')),
    );
    return out;
  };

  return {
    // show shows sel, with its controls disabled when locked. It draws once the event in
    // progress is over, so a field committed by Tab gives the focus to the next one first, and
    // the control that has the focus keeps it in the new panel.
    show(s: Scene | undefined, sel: Selection, locked: boolean) {
      next = { s, sel, locked };
      if (!timer) timer = setTimeout(render);
    },
  };

  function render() {
    timer = undefined;
    const { s, sel, locked } = next!;
    const n = sel.nodes.length === 1 ? s?.model.nodes.find((x) => x.id === sel.nodes[0]) : undefined;
    const own = n && s?.view.nodes.find((x) => x.id === n.id);
    const key = JSON.stringify([sel, locked, n, own?.view_label && own.label, sel.edge && s?.model.edges.find((e) => e.id === sel.edge), sel.decoration && s?.view.decorations?.find((d) => d.id === sel.decoration), s?.view.view]);
    if (key === shown) return;
    const same = shown && JSON.stringify(sel) === JSON.stringify(JSON.parse(shown)[0]);
    shown = key;
    const active = root.contains(document.activeElement) ? (document.activeElement as HTMLElement).dataset.field : undefined;
    let body: (Node | null)[];
    if (!s) body = [];
    else if (n) body = nodeView(s, n);
    else if (sel.edge) body = edgeView(s, sel.edge);
    else if (sel.decoration) body = decorationView(s, sel.decoration);
    else if (sel.nodes.length > 1)
      body = [
        h('h2', {}, `${sel.nodes.length} nodes`),
        h('div', { class: 'actions last' }, h('button', { type: 'button', onclick: () => act.hide(sel.nodes) }, 'Hide from view'), h('button', { type: 'button', class: 'danger', onclick: () => act.remove(sel) }, 'Delete')),
      ];
    else body = [h('h2', {}, 'Nothing selected'), h('p', { class: 'note' }, 'Select a node, edge or decoration to edit it. Double-click a node to edit its label.')];
    const fs = h('fieldset', { disabled: locked }, ...body.filter((x): x is Node => !!x));
    const seen = new Map<string, number>();
    for (const c of fs.querySelectorAll<HTMLElement>('input, select, textarea, button')) {
      const name = c.getAttribute('aria-label') || (c.id && fs.querySelector(`label[for="${c.id}"]`)?.textContent) || c.textContent;
      const base = `${c.tagName}:${name}`;
      const i = seen.get(base) ?? 0;
      seen.set(base, i + 1);
      c.dataset.field = `${base}:${i}`;
    }
    root.replaceChildren(fs);
    if (same && active) fs.querySelector<HTMLElement>(`[data-field="${CSS.escape(active)}"]`)?.focus();
  }
}

// wrap disables the controls of a read-only element's panel, except the buttons named keep.
function wrap(nodes: (Node | null)[], readOnly: boolean, keep: string[] = []): (Node | null)[] {
  if (readOnly) {
    for (const n of nodes) {
      if (!(n instanceof HTMLElement)) continue;
      const controls = n.matches('input, select, textarea, button') ? [n] : [...n.querySelectorAll<HTMLElement>('input, select, textarea, button')];
      for (const c of controls) if (!keep.includes(c.textContent ?? '')) c.setAttribute('disabled', '');
    }
  }
  return nodes;
}

// rowsEditor edits the rows of a table or class node: compartments of rows with a name, type
// and key, sent together by Apply.
function rowsEditor(n: ModelNode, apply: (compartments: { name?: string; rows: { name: string; type?: string; key?: string }[] }[]) => void): Node[] {
  const comps: Compartment[] = structuredClone(n.compartments?.length ? n.compartments : [{ name: '', rows: [] }]);
  const box = h('div', { class: 'rows' });
  const draw = () => {
    const named = comps.length > 1 || !!comps[0]?.name;
    box.replaceChildren(
      ...comps.flatMap((c, ci): Node[] => [
        named
          ? h(
              'div',
              { class: 'row' },
              h('input', { value: c.name, placeholder: 'compartment', 'aria-label': 'Compartment name', oninput: (ev: Event) => (c.name = (ev.target as HTMLInputElement).value) }),
              h('button', { type: 'button', 'aria-label': `Remove compartment ${c.name}`, onclick: () => (comps.splice(ci, 1), draw()) }, '×'),
            )
          : document.createTextNode(''),
        ...c.rows.map((r, ri) =>
          h(
            'div',
            { class: 'row' },
            h('input', { value: r.key ?? '', placeholder: 'key', size: 3, 'aria-label': 'Key', oninput: (ev: Event) => (r.key = (ev.target as HTMLInputElement).value) }),
            h('input', { value: r.name, placeholder: 'name', 'aria-label': 'Row name', oninput: (ev: Event) => (r.name = (ev.target as HTMLInputElement).value) }),
            h('input', { value: r.type ?? '', placeholder: 'type', 'aria-label': 'Type', oninput: (ev: Event) => (r.type = (ev.target as HTMLInputElement).value) }),
            h('button', { type: 'button', 'aria-label': `Remove row ${r.name}`, onclick: () => (c.rows.splice(ri, 1), draw()) }, '×'),
          ),
        ),
        h('button', { type: 'button', onclick: () => (c.rows.push({ name: '' }), draw(), (box.querySelectorAll<HTMLInputElement>('input[aria-label="Row name"]')[rowIndex(comps, ci)] ?? null)?.focus()) }, named ? `Add row to ${c.name || 'compartment'}` : 'Add row'),
      ]),
    );
  };
  draw();
  const result = () =>
    comps
      .filter((c) => c.name || comps.length === 1)
      .map((c) => ({
        ...(c.name ? { name: c.name } : {}),
        rows: c.rows.filter((r) => r.name).map((r) => ({ name: r.name, ...(r.type ? { type: r.type } : {}), ...(r.key ? { key: r.key } : {}) })),
      }))
      .filter((c) => c.name || c.rows.length);
  return [
    h('h3', {}, 'Rows'),
    box,
    h(
      'div',
      { class: 'actions' },
      h(
        'button',
        {
          type: 'button',
          onclick: () => {
            if (!comps[0].name) comps[0].name = 'compartment1';
            comps.push({ name: `compartment${comps.length + 1}`, rows: [] });
            draw();
          },
        },
        'Add compartment',
      ),
      h('button', { type: 'button', onclick: () => apply(result()) }, 'Apply rows'),
    ),
  ];
}

// rowIndex is the index, among all rows, of the last row of compartment ci.
function rowIndex(comps: Compartment[], ci: number): number {
  return comps.slice(0, ci + 1).reduce((a, c) => a + c.rows.length, 0) - 1;
}

function words(v: string): string[] {
  return v.split(/[\s,]+/).filter(Boolean);
}

function endText(e: { node: string; port?: string; row?: string }): string {
  return e.node + (e.port ? `.port.${e.port}` : e.row ? `.row.${e.row}` : '');
}

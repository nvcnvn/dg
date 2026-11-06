import { Check } from 'lucide';
import { movable, unpinOps, type Align, type Scene } from '../edit/edit';
import type { Point } from '../geometry/geometry';
import type { Actions, Selection } from './actions';
import { h, icon } from './dialog';

// Commands are what the context menu and the keys do on the canvas (canvas/interact.ts).
export interface Commands {
  scene(): Scene | undefined;
  editable(): boolean;
  selection(): Selection;
  select(sel: Selection): void;
  hasClip(): boolean;
  copy(): void;
  // paste pastes at p, else at the pointer.
  paste(p?: Point): void;
  remove(recursive?: boolean): void;
  editLabel(id: string): void;
  editEdgeLabel(id: string): void;
  unpin(): void;
  arrange(how: Align | 'x' | 'y'): void;
  resetRoute(edge: string): void;
  connecting(on?: boolean): boolean;
  // cancel cancels the gesture in progress, and reports whether there was one.
  cancel(): boolean;
  zoom(): number;
  setZoom(z: number): void;
  fit(): void;
  zoomToSelection(): void;
  help(): void;
  center(): Point;
  pagePoint(p: Point): Point;
  selectView(id: string): void;
  space(down: boolean): void;
}

// Entry is an item of a menu, a separator when its label is empty, with its shortcut on the
// right; danger colors a destructive one. A disabled item shows why it is disabled.
export interface Entry {
  label: string;
  run: () => void;
  on: boolean;
  why?: string;
  keys?: string;
  danger?: boolean;
  // checked marks the current choice of a menu that holds one.
  checked?: boolean;
  view?: string;
}

const SEP: Entry = { label: '', run: () => {}, on: true };
const openers = new Map<HTMLElement, Element | undefined>();
const GAP = 4;

// showMenu shows entries in menu at the client point (x, y), kept inside the window, with the
// focus on its first item that can be chosen; opener is the control that opened it, which gets
// the focus back on Escape and closes it when clicked again.
export function showMenu(menu: HTMLElement, entries: Entry[], x: number, y: number, opener?: Element) {
  menu.replaceChildren(
    ...entries.map((e) => {
      if (!e.label) return document.createElement('hr');
      const radio = e.checked !== undefined;
      return h(
        'button',
        {
          type: 'button',
          class: e.danger ? 'item danger' : 'item',
          role: radio ? 'menuitemradio' : 'menuitem',
          'aria-checked': radio ? String(e.checked) : undefined,
          'data-view': e.view,
          disabled: !e.on,
          onclick: () => {
            closeMenu(menu);
            e.run();
          },
        },
        e.checked ? icon(Check) : null,
        h('span', { class: 'text' }, e.label),
        e.keys ? h('kbd', { 'aria-hidden': 'true' }, e.keys) : null,
        !e.on && e.why ? h('small', {}, e.why) : null,
      );
    }),
  );
  menu.hidden = false;
  menu.style.left = `${Math.max(GAP, Math.min(x, window.innerWidth - menu.offsetWidth - GAP))}px`;
  menu.style.top = `${Math.max(GAP, Math.min(y, window.innerHeight - menu.offsetHeight - GAP))}px`;
  openers.set(menu, opener);
  menu.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
}

export function closeMenu(menu: HTMLElement) {
  if (menu.hidden) return;
  menu.hidden = true;
  menu.replaceChildren();
  openers.delete(menu);
}

// openerOf is the control that opened menu, while it is open.
export const openerOf = (menu: HTMLElement) => openers.get(menu);

// bindMenu makes the arrow keys, Home and End move through menu, Escape close it, and a click
// outside it close it.
export function bindMenu(menu: HTMLElement) {
  menu.addEventListener('keydown', (ev) => {
    const items = [...menu.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')];
    const i = items.indexOf(document.activeElement as HTMLButtonElement);
    if (ev.key === 'ArrowDown') items[(i + 1) % items.length]?.focus();
    else if (ev.key === 'ArrowUp') items[(i - 1 + items.length) % items.length]?.focus();
    else if (ev.key === 'Home') items[0]?.focus();
    else if (ev.key === 'End') items[items.length - 1]?.focus();
    else if (ev.key === 'Escape') {
      const opener = openers.get(menu);
      closeMenu(menu);
      if (opener instanceof HTMLElement) opener.focus();
      else window.focus();
    } else return;
    ev.preventDefault();
    ev.stopPropagation();
  });
  document.addEventListener('click', (ev) => {
    const t = ev.target as Node;
    if (!menu.contains(t) && !openers.get(menu)?.contains(t)) closeMenu(menu);
  });
}
const READ_ONLY = 'Defined in a read-only file';
const ZOOMS = [0.25, 0.33, 0.5, 0.67, 0.75, 0.9, 1, 1.1, 1.25, 1.5, 2, 3, 4];

// UI is where the canvas's keys do not apply: controls, which use the keys themselves, and the
// toolbar, panels, menus, dialogs and toast around the canvas.
const UI = 'button, input, select, textarea, a, dialog, #bar, #palette, #inspector, #context-menu, #drill-menu, #tool-menu, #overflow-menu, #toast, #text';

// menuAndKeys runs the context menu and the editor's keys (FR-11).
export function menuAndKeys(menu: HTMLElement, c: Commands, act: Actions) {
  const item = (label: string, run: () => void, on: boolean, why?: string, keys?: string): Entry => ({ label, run, on, why: on ? undefined : why, keys });
  const danger = (label: string, run: () => void, on: boolean, why?: string, keys?: string): Entry => ({ ...item(label, run, on, why, keys), danger: true });

  // entries are the context menu's items for the selection, or for the canvas at p.
  const entries = (s: Scene, p: Point): Entry[] => {
    const sel = c.selection();
    const ids = sel.nodes;
    const n = movable(s, ids).length;
    const readOnly = (id: string) => !!s.model.nodes.find((m) => m.id === id)?.read_only;
    const ro = ids.some(readOnly);
    if (sel.decoration) {
      const d = s.view.decorations?.find((x) => x.id === sel.decoration);
      const id = sel.decoration;
      const why = d?.attach ? 'An attached decoration has no position of its own' : 'An arrow or a freehand line has no box';
      return [
        item('Promote to node…', () => act.promote(id), !!d && !d.attach && d.type !== 'arrow' && d.type !== 'freehand', why),
        d?.attach ? item('Detach from node', () => act.detach(id), true) : item('Attach to node…', () => act.attach(id), true),
        SEP,
        danger('Delete decoration', () => c.remove(), true, undefined, 'Del'),
      ];
    }
    if (sel.edge) {
      const id = sel.edge;
      const ve = s.view.edges.find((e) => e.id === id);
      const me = s.model.edges.find((e) => e.id === id);
      const why = me ? READ_ONLY : 'A lifted edge stands for edges inside a collapsed container';
      const ok = !!me && !me.read_only;
      return [
        item('Edit label', () => c.editEdgeLabel(id), ok, why, 'F2'),
        item('Rename ID…', () => act.rename('edge', id), ok, why),
        item('Reset route', () => c.resetRoute(id), !!ve?.hint),
        SEP,
        danger('Delete edge', () => c.remove(), ok, why, 'Del'),
      ];
    }
    if (!ids.length) {
      return [
        item('Paste here', () => c.paste(p), c.hasClip(), undefined, 'Ctrl/⌘+V'),
        item('Add node…', () => act.addNode(), s.model.nodes.some((m) => !s.nodes.has(m.id))),
        item('Add text here', () => act.addDecoration('text', p), true),
      ];
    }
    const one = ids.length === 1 ? ids[0] : undefined;
    const drill = one ? (s.view.nodes.find((x) => x.id === one)?.drill ?? []) : [];
    const container = ids.some((id) => s.model.nodes.some((m) => m.parent === id));
    return [
      item('Edit label', () => c.editLabel(one!), !!one && !ro, one && READ_ONLY, 'F2'),
      item('Copy', () => c.copy(), true, undefined, 'Ctrl/⌘+C'),
      item('Paste', () => c.paste(), c.hasClip(), undefined, 'Ctrl/⌘+V'),
      item('Hide from view', () => act.hide(ids), true),
      item('Rename ID…', () => act.rename('node', one!), !!one && !ro, one && READ_ONLY),
      item('Add port…', () => act.addPort(one!), !!one && !ro, one && READ_ONLY),
      ...drill.map((v) => item(`Open view ${v}`, () => c.selectView(v), true)),
      SEP,
      item('Unpin', () => c.unpin(), unpinOps(s, ids).length > 0, undefined, 'U'),
      ...arrangeItems(c.arrange, n, true),
      SEP,
      danger('Delete from model', () => c.remove(), !ro, READ_ONLY, 'Del'),
      ...(container ? [danger('Delete with contents', () => c.remove(true), !ro, READ_ONLY)] : []),
    ];
  };

  const close = () => closeMenu(menu);

  // open shows the context menu at the client point (x, y) for the drawing's point p.
  const open = (x: number, y: number, p: Point) => {
    const s = c.scene();
    if (!s || !c.editable()) return;
    const list = entries(s, p).filter((e) => !e.label || e.on || e.why);
    const shown = list.filter((e, i) => e.label || (i > 0 && list[i - 1].label && list.slice(i + 1).some((x) => x.label)));
    if (shown.some((e) => e.label && e.on)) showMenu(menu, shown, x, y);
  };

  bindMenu(menu);

  document.addEventListener('keydown', (ev) => {
    if ((ev.target as Element).closest?.(UI)) return;
    if (ev.key === ' ') c.space(true);
    const mod = ev.ctrlKey || ev.metaKey;
    const key = ev.key.toLowerCase();
    const sel = c.selection();
    if (ev.key === 'Escape') {
      if (!c.cancel()) {
        if (c.connecting()) c.connecting(false);
        else c.select({ nodes: [] });
      }
    } else if (mod && key === 'a') {
      const s = c.scene();
      if (s) c.select({ nodes: s.drawing.nodes.map((n) => n.id) });
    } else if (mod && key === 'c') {
      c.copy();
    } else if (mod && key === 'v') {
      c.paste();
    } else if (!mod && (ev.key === 'Delete' || ev.key === 'Backspace')) {
      if (sel.nodes.length || sel.edge || sel.decoration) c.remove();
    } else if (!mod && (ev.key === 'F2' || ev.key === 'Enter')) {
      if (sel.nodes.length === 1) c.editLabel(sel.nodes[0]);
      else if (sel.edge) c.editEdgeLabel(sel.edge);
      else return;
    } else if ((ev.shiftKey && ev.key === 'F10') || ev.key === 'ContextMenu') {
      const s = c.scene();
      const b = s && (sel.nodes.length ? s.nodes.get(sel.nodes[0])?.box : sel.decoration ? s.drawing.decorations.find((x) => x.id === sel.decoration)?.box : undefined);
      const p = b ? { x: b.x + b.w / 2, y: b.y + b.h / 2 } : c.center();
      const q = c.pagePoint(p);
      open(q.x, q.y, p);
    } else if (!mod && ev.shiftKey && ev.code === 'Digit1') {
      c.fit();
    } else if (!mod && ev.shiftKey && ev.code === 'Digit2') {
      c.zoomToSelection();
    } else if (!mod && ev.key === '?') {
      c.help();
    } else if (!mod && key === 'c') {
      c.connecting(!c.connecting());
    } else if (!mod && key === 'u') {
      c.unpin();
    } else if (!mod && (ev.key === '+' || ev.key === '=')) {
      c.setZoom(step(c.zoom(), 1));
    } else if (!mod && ev.key === '-') {
      c.setZoom(step(c.zoom(), -1));
    } else if (!mod && ev.key === '0') {
      c.setZoom(1);
    } else return;
    ev.preventDefault();
  });
  document.addEventListener('keyup', (ev) => {
    if (ev.key === ' ') c.space(false);
  });

  return { open, close };
}

// arrangeItems are the align and distribute actions for n nodes that can move, which disabled
// are left out of the context menu and explained in the toolbar's menu.
export function arrangeItems(arrange: (how: Align | 'x' | 'y') => void, n: number, quiet = false): Entry[] {
  const item = (label: string, how: Align | 'x' | 'y', need: number): Entry => ({ label, run: () => arrange(how), on: n >= need, why: quiet ? undefined : need === 2 ? 'Select two nodes or more' : 'Select three nodes or more' });
  return [
    item('Align left', 'left', 2),
    item('Align center', 'center', 2),
    item('Align right', 'right', 2),
    item('Align top', 'top', 2),
    item('Align middle', 'middle', 2),
    item('Align bottom', 'bottom', 2),
    item('Distribute horizontally', 'x', 3),
    item('Distribute vertically', 'y', 3),
  ];
}

function step(z: number, dir: 1 | -1): number {
  if (dir > 0) return ZOOMS.find((v) => v > z + 1e-9) ?? z;
  return [...ZOOMS].reverse().find((v) => v < z - 1e-9) ?? z;
}

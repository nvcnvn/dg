import { movable, unpinOps, type Align, type Scene } from '../edit/edit';
import type { Point } from '../geometry/geometry';
import type { Actions, Selection } from './actions';

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
  center(): Point;
  pagePoint(p: Point): Point;
  selectView(id: string): void;
  space(down: boolean): void;
}

// Entry is an item of the context menu, a separator when its label is empty. A disabled item
// is listed when why says why it is disabled, and left out otherwise.
type Entry = { label: string; run: () => void; on: boolean; why?: string };

const SEP: Entry = { label: '', run: () => {}, on: true };
const READ_ONLY = 'Defined in a read-only file';
const ZOOMS = [0.25, 0.33, 0.5, 0.67, 0.75, 0.9, 1, 1.1, 1.25, 1.5, 2, 3, 4];

// UI is where the canvas's keys do not apply: controls, which use the keys themselves, and the
// toolbar, panels, menus, dialogs and toast around the canvas.
const UI = 'button, input, select, textarea, a, dialog, #bar, #palette, #inspector, #context-menu, #drill-menu, #toast';

// menuAndKeys runs the context menu and the editor's keys (FR-11).
export function menuAndKeys(menu: HTMLElement, c: Commands, act: Actions) {
  const item = (label: string, run: () => void, on: boolean, why?: string): Entry => ({ label, run, on, why: on ? undefined : why });

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
        item('Delete decoration', () => c.remove(), true),
      ];
    }
    if (sel.edge) {
      const id = sel.edge;
      const ve = s.view.edges.find((e) => e.id === id);
      const me = s.model.edges.find((e) => e.id === id);
      const why = me ? READ_ONLY : 'A lifted edge stands for edges inside a collapsed container';
      const ok = !!me && !me.read_only;
      return [
        item('Edit label', () => c.editEdgeLabel(id), ok, why),
        item('Rename ID…', () => act.rename('edge', id), ok, why),
        item('Delete edge', () => c.remove(), ok, why),
        SEP,
        item('Reset route', () => c.resetRoute(id), !!ve?.hint),
      ];
    }
    if (!ids.length) {
      return [
        item('Paste here', () => c.paste(p), c.hasClip()),
        item('Add node…', () => act.addNode(), s.model.nodes.some((m) => !s.nodes.has(m.id))),
        item('Add text here', () => act.addDecoration('text', p), true),
      ];
    }
    const one = ids.length === 1 ? ids[0] : undefined;
    const drill = one ? (s.view.nodes.find((x) => x.id === one)?.drill ?? []) : [];
    const container = ids.some((id) => s.model.nodes.some((m) => m.parent === id));
    return [
      item('Edit label', () => c.editLabel(one!), !!one && !ro, one && READ_ONLY),
      item('Copy', () => c.copy(), true),
      item('Paste', () => c.paste(), c.hasClip()),
      SEP,
      item('Hide from view', () => act.hide(ids), true),
      item('Delete from model', () => c.remove(), !ro, READ_ONLY),
      ...(container ? [item('Delete with contents', () => c.remove(true), !ro, READ_ONLY)] : []),
      item('Rename ID…', () => act.rename('node', one!), !!one && !ro, one && READ_ONLY),
      item('Add port…', () => act.addPort(one!), !!one && !ro, one && READ_ONLY),
      ...drill.map((v) => item(`Open view ${v}`, () => c.selectView(v), true)),
      SEP,
      item('Unpin', () => c.unpin(), unpinOps(s, ids).length > 0),
      item('Align left', () => c.arrange('left'), n > 1),
      item('Align center', () => c.arrange('center'), n > 1),
      item('Align right', () => c.arrange('right'), n > 1),
      item('Align top', () => c.arrange('top'), n > 1),
      item('Align middle', () => c.arrange('middle'), n > 1),
      item('Align bottom', () => c.arrange('bottom'), n > 1),
      item('Distribute horizontally', () => c.arrange('x'), n > 2),
      item('Distribute vertically', () => c.arrange('y'), n > 2),
    ];
  };

  const close = () => {
    if (menu.hidden) return;
    menu.hidden = true;
    menu.replaceChildren();
  };

  // open shows the context menu at the page point (x, y) for the drawing's point p, with the
  // focus on its first item; arrow keys move between items and Escape closes it.
  const open = (x: number, y: number, p: Point) => {
    const s = c.scene();
    if (!s || !c.editable()) return;
    const list = entries(s, p).filter((e) => !e.label || e.on || e.why);
    const shown = list.filter((e, i) => e.label || (i > 0 && list[i - 1].label && list.slice(i + 1).some((x) => x.label)));
    if (!shown.some((e) => e.label && e.on)) return;
    menu.replaceChildren(
      ...shown.map((e) => {
        if (!e.label) return document.createElement('hr');
        const b = document.createElement('button');
        b.type = 'button';
        b.textContent = e.label;
        b.setAttribute('role', 'menuitem');
        b.disabled = !e.on;
        if (e.why) b.title = e.why;
        b.addEventListener('click', () => {
          close();
          e.run();
        });
        return b;
      }),
    );
    menu.style.left = `${x}px`;
    menu.style.top = `${y}px`;
    menu.hidden = false;
    menu.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
  };

  menu.addEventListener('keydown', (ev) => {
    const items = [...menu.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')];
    const i = items.indexOf(document.activeElement as HTMLButtonElement);
    if (ev.key === 'ArrowDown') items[(i + 1) % items.length]?.focus();
    else if (ev.key === 'ArrowUp') items[(i - 1 + items.length) % items.length]?.focus();
    else if (ev.key === 'Escape') {
      close();
      window.focus();
    } else return;
    ev.preventDefault();
    ev.stopPropagation();
  });

  document.addEventListener('click', (ev) => {
    if (!menu.contains(ev.target as Node)) close();
  });

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

function step(z: number, dir: 1 | -1): number {
  if (dir > 0) return ZOOMS.find((v) => v > z + 1e-9) ?? z;
  return [...ZOOMS].reverse().find((v) => v < z - 1e-9) ?? z;
}

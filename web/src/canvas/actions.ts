import type { Scene } from '../edit/edit';
import { decorationOp, deleteOps, hideOps, selectors, slug, type DecorationType } from '../edit/model';
import { center, roundTo, type Point } from '../geometry/geometry';
import type { Result } from '../protocol/client';
import type { Op } from '../protocol/ops';
import { ask, choose } from './dialog';

// Selection is what the editor has selected: nodes, or an edge, or a decoration.
export interface Selection {
  nodes: string[];
  edge?: string;
  decoration?: string;
}

// ActionHost is what the model actions need from the editor.
export interface ActionHost {
  scene(): Scene | undefined;
  editable(): boolean;
  // send sends an operation and calls then with its results once the drawing it brings is shown.
  send(op: Op, then?: (results: Result[]) => void): boolean;
  select(sel: Selection): void;
  selectView(id: string): void;
  status(message: string): void;
}

export type Actions = ReturnType<typeof actions>;

type PortSide = NonNullable<Extract<Op, { op: 'add_port' }>['side']>;

const SIDES: [PortSide | '', string][] = [
  ['', 'default of the kind'],
  ['top', 'top'],
  ['right', 'right'],
  ['bottom', 'bottom'],
  ['left', 'left'],
  ['auto', 'auto'],
];

// actions are the model edits the context menu, the keyboard, the toolbar and the inspector
// share; those that need names ask for them in a dialog. Each sends one operation or batch.
export function actions(host: ActionHost) {
  const s = () => host.scene();
  const view = () => s()?.view.view ?? '';
  // pick selects what the results name, of results of op.
  const pick = (op: Op['op'], as: 'node' | 'edge' | 'decoration') => (results: Result[]) => {
    const ids = results.filter((r) => r.op === op && r.id).map((r) => r.id!);
    if (!ids.length) return;
    host.select(as === 'node' ? { nodes: ids } : as === 'edge' ? { nodes: [], edge: ids[0] } : { nodes: [], decoration: ids[0] });
  };

  return {
    pick,

    async rename(type: 'node' | 'edge', old: string) {
      const v = await ask(`Rename ${type} ${old}`, [{ name: 'id', label: 'New ID', value: old, required: true, help: 'Every reference is renamed: model files, views.hcl and layout files.' }], 'Rename');
      if (!v || v.id === old || !host.editable()) return;
      host.send({ op: 'rename_id', type, old, new: v.id }, type === 'node' ? pick('rename_id', 'node') : pick('rename_id', 'edge'));
    },

    async addPort(id: string) {
      const v = await ask(`Add a port to ${id}`, [
        { name: 'name', label: 'Name', required: true },
        { name: 'side', label: 'Side', type: 'select', options: SIDES },
      ]);
      if (!v || !v.name || !host.editable()) return;
      host.send({ op: 'add_port', id, name: v.name, ...(v.side ? { side: v.side as PortSide } : {}) });
    },

    // remove deletes the selection: nodes from the model (recursive also what they hold), an edge
    // from the model, or a decoration from the view.
    remove(sel: Selection, recursive = false) {
      const op: Op | undefined = sel.edge
        ? { op: 'disconnect', id: sel.edge }
        : sel.decoration
          ? { op: 'delete_decoration', view: view(), id: sel.decoration }
          : deleteOps(sel.nodes, recursive);
      if (op) host.send(op);
    },

    hide(ids: string[]) {
      const sc = s();
      const op = sc && hideOps(sc, ids);
      if (op) host.send(op);
    },

    // addNode makes the view show a node of the model it does not draw.
    async addNode() {
      const sc = s();
      if (!sc) return;
      const items = sc.model.nodes.filter((n) => !sc.nodes.has(n.id)).map((n) => ({ value: n.id, text: n.label === n.id ? n.id : `${n.label} (${n.id})` }));
      const id = await choose('Add a node to the view', 'Search by ID or label', items);
      if (id && host.editable()) host.send({ op: 'show_node', view: view(), id }, pick('show_node', 'node'));
    },

    async newView(include: string[]) {
      const v = await ask(
        'New view',
        [
          { name: 'title', label: 'Title' },
          { name: 'id', label: 'ID', help: 'A simple ID; made from the title when empty.' },
          { name: 'include', label: 'Include', value: include.join(' ') || 'all', help: 'Selectors: all, node.<id>, kind.<kind>, tag.<tag>, children.<id>, descendants.<id>.' },
          { name: 'exclude', label: 'Exclude' },
          { name: 'collapse', label: 'Collapse', help: 'Containers drawn as one box.' },
          { name: 'overlays', label: 'Overlays', type: 'textarea', help: 'One per line: <id>: <selectors> | <label>' },
        ],
        'Create',
      );
      if (!v || !host.editable()) return;
      const id = v.id.trim() || slug(v.title) || 'view';
      const overlays = v.overlays
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean)
        .map((line) => {
          const [head, label] = line.split('|').map((x) => x.trim());
          const [oid, members] = head.split(/:(.*)/s).map((x) => x.trim());
          return { id: oid, members: selectors(members ?? ''), ...(label ? { label } : {}) };
        });
      const op: Op = {
        op: 'create_view',
        id,
        ...(v.title.trim() ? { title: v.title.trim() } : {}),
        include: selectors(v.include),
        ...(selectors(v.exclude).length ? { exclude: selectors(v.exclude) } : {}),
        ...(selectors(v.collapse).length ? { collapse: selectors(v.collapse) } : {}),
        ...(overlays.length ? { overlays } : {}),
      };
      host.send(op, () => host.selectView(id));
    },

    // addDecoration adds a decoration of type at p, attached to node attach if set.
    async addDecoration(type: DecorationType, p: Point, attach?: string) {
      const sc = s();
      if (!sc) return;
      let text = '';
      if (type === 'text' || type === 'image') {
        const v = await ask(type === 'text' ? 'Add a text' : 'Add an image', [type === 'text' ? { name: 'text', label: 'Text', type: 'textarea', required: true } : { name: 'text', label: 'SVG file', required: true, help: 'A path in the project, such as icons/logo.svg.' }], 'Add');
        if (!v?.text) return;
        text = v.text;
      }
      if (host.editable()) host.send(decorationOp(sc, type, p, attach, text), pick('add_decoration', 'decoration'));
    },

    async promote(id: string) {
      const sc = s();
      const d = sc?.view.decorations?.find((x) => x.id === id);
      if (!sc || !d) return;
      const kinds = sc.model.kinds.map((k): [string, string] => [k.name, k.name]);
      const v = await ask(`Promote ${id} to a node`, [
        { name: 'kind', label: 'Kind', type: 'select', options: kinds, value: d.type === 'text' ? 'note' : 'node' },
        { name: 'label', label: 'Label', value: d.text ?? '' },
      ], 'Promote');
      if (!v || !host.editable()) return;
      host.send({ op: 'promote_decoration', view: view(), id, kind: v.kind, ...(v.label && v.label !== d.text ? { label: v.label } : {}) }, pick('promote_decoration', 'node'));
    },

    // attach attaches decoration id to a node, keeping where it is drawn.
    async attach(id: string) {
      const sc = s();
      const d = sc?.drawing.decorations.find((x) => x.id === id);
      if (!sc || !d) return;
      const target = await choose(`Attach ${id} to a node`, 'Search by ID', sc.drawing.nodes.map((n) => ({ value: n.id, text: n.id })));
      const nb = target && sc.nodes.get(target)?.box;
      if (!nb || !host.editable()) return;
      const at = d.type === 'arrow' || d.type === 'freehand' ? d.origin : center(d.box);
      const c = center(nb);
      const g = sc.view.grid;
      host.send({ op: 'update_decoration', view: view(), id, decoration: { attach: target, dx: roundTo(at.x - c.x, g), dy: roundTo(at.y - c.y, g) }, unset: ['in', 'x', 'y'] });
    },

    // detach detaches decoration id from its node, keeping where it is drawn.
    detach(id: string) {
      const sc = s();
      const d = sc?.drawing.decorations.find((x) => x.id === id);
      if (!sc || !d) return;
      const at = d.type === 'arrow' || d.type === 'freehand' ? d.origin : d.box;
      const g = sc.view.grid;
      host.send({ op: 'update_decoration', view: view(), id, decoration: { x: roundTo(at.x, g), y: roundTo(at.y, g) }, unset: ['attach', 'dx', 'dy'] });
    },

  };
}

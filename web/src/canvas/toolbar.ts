import { Cable, CircleHelp, Download, Ellipsis, LayoutTemplate, Maximize, PanelLeft, PanelRight, PinOff, Redo2, Shapes, SquarePlus, AlignCenterHorizontal, Undo2, ZoomIn, ZoomOut, type IconNode } from 'lucide';
import { h, icon } from './dialog';

// Tool is a button of the toolbar: an icon, unless it shows its label, with a tooltip that names
// the action and its shortcut.
interface Tool {
  id: string;
  label: string;
  // text is what a tool that shows its label shows, when it differs from its name.
  text?: string;
  icon?: IconNode;
  keys?: string;
  // aria is the shortcut as aria-keyshortcuts writes it.
  aria?: string;
  // tip is the action when it is not the label.
  tip?: string;
  menu?: boolean;
  toggle?: boolean;
  labeled?: boolean;
}

const VIEW_TOOLS: Tool[] = [{ id: 'new-view', label: 'New view…', icon: LayoutTemplate }];

const GROUPS: { name: string; tools: Tool[] }[] = [
  {
    name: 'History',
    tools: [
      { id: 'undo', label: 'Undo', icon: Undo2, keys: 'Ctrl/⌘+Z', aria: 'Control+Z Meta+Z' },
      { id: 'redo', label: 'Redo', icon: Redo2, keys: 'Ctrl/⌘+Shift+Z', aria: 'Control+Shift+Z Meta+Shift+Z' },
    ],
  },
  {
    name: 'Create',
    tools: [
      { id: 'add-node', label: 'Add node…', icon: SquarePlus, tip: 'Add a node of the model to the view' },
      { id: 'connect', label: 'Connect', icon: Cable, keys: 'C', aria: 'C', tip: 'Draw edges from nodes, ports and rows', toggle: true },
      { id: 'edge-kind', label: 'Edge: default', tip: 'Kind of new edges', menu: true, labeled: true },
      { id: 'add-decoration', label: 'Decoration…', icon: Shapes, tip: 'Add a decoration', menu: true },
    ],
  },
  {
    name: 'Arrange',
    tools: [
      { id: 'unpin', label: 'Unpin', icon: PinOff, keys: 'U', aria: 'U', tip: 'Unpin the selection' },
      { id: 'arrange', label: 'Arrange', icon: AlignCenterHorizontal, tip: 'Align and distribute', menu: true },
    ],
  },
  {
    name: 'Zoom',
    tools: [
      { id: 'zoom-out', label: 'Zoom out', icon: ZoomOut, keys: '-', aria: '-' },
      { id: 'zoom', label: 'Zoom level 100%', text: '100%', tip: 'Zoom level', menu: true, labeled: true },
      { id: 'zoom-in', label: 'Zoom in', icon: ZoomIn, keys: '+', aria: '+' },
      { id: 'fit', label: 'Fit', icon: Maximize, keys: 'Shift+1', aria: 'Shift+1', tip: 'Fit the view' },
    ],
  },
  {
    name: 'Panels',
    tools: [
      { id: 'show-palette', label: 'Palette', icon: PanelLeft, tip: 'Show the palette of kinds', toggle: true },
      { id: 'show-inspector', label: 'Inspector', icon: PanelRight, tip: 'Show the inspector', toggle: true },
    ],
  },
  { name: 'Export', tools: [{ id: 'save', label: 'Export', icon: Download, tip: 'Export the view', menu: true }] },
  { name: 'Help', tools: [{ id: 'help', label: 'Keyboard shortcuts', icon: CircleHelp, keys: '?', aria: 'Shift+?' }] },
];

// toolbar builds the toolbar in root, one row of groups (FR-20): what does not fit goes into the
// overflow menu at the end of the row, last group first. layout runs again when the row's width
// or content changes.
export function toolbar(root: HTMLElement, overflow: HTMLElement) {
  const tools = new Map<string, Tool>();
  const get = (id: string) => document.getElementById(id) as HTMLButtonElement;
  const button = (t: Tool) => {
    tools.set(t.id, t);
    const b = h(
      'button',
      {
        type: 'button',
        id: t.id,
        class: t.labeled ? 'tool labeled' : 'tool',
        'aria-label': t.label,
        'aria-keyshortcuts': t.aria,
        'aria-haspopup': t.menu ? 'menu' : undefined,
        'aria-pressed': t.toggle ? 'false' : undefined,
        'data-tip': tipOf(t),
      },
      t.icon ? icon(t.icon) : null,
      h('span', { class: 'tool-text' }, t.text ?? t.label),
    );
    return b;
  };

  const views = h('select', { id: 'views', 'aria-label': 'View' });
  const crumbs = h('nav', { id: 'crumbs', 'aria-label': 'Breadcrumbs', hidden: true });
  const viewGroup = h('div', { class: 'group view', role: 'group', 'aria-label': 'View' }, views, crumbs, ...VIEW_TOOLS.map(button));
  const groups = GROUPS.map((g) => h('div', { class: 'group', role: 'group', 'aria-label': g.name }, ...g.tools.map(button)));
  const more = h('button', { type: 'button', id: 'more', class: 'tool', 'aria-label': 'More tools', 'aria-haspopup': 'true', 'aria-expanded': 'false', 'data-tip': 'More tools', hidden: true }, icon(Ellipsis));
  root.replaceChildren(viewGroup, ...groups, more);

  const fits = () => root.scrollWidth <= root.clientWidth;
  // laid is what the row was last laid out for. Moving a group loses the focus inside it, so
  // nothing moves until the width or the row's text changes.
  let laid = '';
  const layout = () => {
    const now = [root.clientWidth, views.selectedOptions[0]?.text, crumbs.textContent, get('edge-kind').textContent].join('\n');
    if (now === laid) return;
    laid = now;
    for (const g of groups) if (g.parentElement !== root) root.insertBefore(g, more);
    more.hidden = true;
    if (fits()) {
      if (!overflow.hidden) closeOverflow();
      return;
    }
    more.hidden = false;
    for (const g of [...groups].reverse()) {
      overflow.prepend(g);
      if (fits()) break;
    }
  };
  const closeOverflow = () => {
    overflow.hidden = true;
    more.setAttribute('aria-expanded', 'false');
  };
  const openOverflow = () => {
    const r = more.getBoundingClientRect();
    overflow.hidden = false;
    overflow.style.top = `${r.bottom + 4}px`;
    overflow.style.left = `${Math.max(4, r.right - overflow.offsetWidth)}px`;
    more.setAttribute('aria-expanded', 'true');
    overflow.querySelector<HTMLElement>('button')?.focus();
  };
  more.addEventListener('click', () => (overflow.hidden ? openOverflow() : closeOverflow()));
  overflow.addEventListener('click', (ev) => {
    if ((ev.target as Element).closest('button')) closeOverflow();
  });
  overflow.addEventListener('keydown', (ev) => {
    if (ev.key !== 'Escape') return;
    closeOverflow();
    more.focus();
    ev.stopPropagation();
  });
  document.addEventListener('click', (ev) => {
    if (!overflow.hidden && !overflow.contains(ev.target as Node) && !more.contains(ev.target as Node)) closeOverflow();
  });
  new ResizeObserver(layout).observe(root);

  return {
    views,
    crumbs,
    layout,
    button: get,
    // click runs f when the tool is clicked while enabled.
    click(id: string, f: () => void) {
      get(id).addEventListener('click', () => {
        if (get(id).getAttribute('aria-disabled') !== 'true') f();
      });
    },
    // enable enables a tool; while disabled, its tooltip says what enables it.
    enable(id: string, on: boolean, why: string) {
      const b = get(id);
      b.setAttribute('aria-disabled', String(!on));
      b.dataset.tip = on ? tipOf(tools.get(id)!) : why;
    },
    // setText shows text on a tool that shows its label; name names the tool with it.
    setText(id: string, text: string, name = text) {
      const b = get(id);
      b.querySelector('.tool-text')!.textContent = text;
      b.setAttribute('aria-label', name);
    },
  };
}

function tipOf(t: Tool): string {
  const name = t.tip ?? t.label.replace('…', '');
  return t.keys ? `${name} (${t.keys})` : name;
}

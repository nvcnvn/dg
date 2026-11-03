import { dia } from '@joint/core';
import type { Diagnostic, ServerMessage, ViewInfo } from '../protocol/client';
import { connect } from '../protocol/client';
import type { ResolvedView } from '../protocol/view';
import { checkStyle, type Drawing } from './export';
import { defsMarkup, edgeMarkup, flowCSS, flowPeriods, nodeMarkup, shiftNode } from './markup';

const UNPINNED_OPACITY = 0.55;

const Markup = dia.Element.define('dg.Markup', {});
const namespace = { dg: { Markup } };

// startEditor runs the read-only editor: it draws each snapshot the server sends on a JointJS
// paper, with the markup the SVG export uses, fading unpinned nodes in a view with pinned ones,
// and lists diagnostics. While the project has
// errors the last good drawing stays.
export function startEditor(render: (view: ResolvedView) => Promise<{ drawing: Drawing; family: string }>) {
  const select = document.getElementById('views') as HTMLSelectElement;
  const status = document.getElementById('status')!;
  const hint = document.getElementById('hint')!;
  const list = document.getElementById('diagnostics')!;
  const graph = new dia.Graph({}, { cellNamespace: namespace });
  const paper = new dia.Paper({
    el: document.getElementById('paper')!,
    model: graph,
    cellViewNamespace: namespace,
    interactive: false,
    async: false,
    width: 1,
    height: 1,
  });

  const flowStyle = document.head.appendChild(document.createElement('style'));
  let serverDiags: Diagnostic[] = [];
  let tooSmall: string[] = [];
  let pending: ResolvedView | undefined;
  let drawing = false;

  const showDiagnostics = () => {
    const all = [...serverDiags, ...tooSmall.map((id): Diagnostic => ({ severity: 'warning', summary: 'Container too small', detail: `Node "${id}" has a pinned size too small for its children.` }))];
    list.replaceChildren(
      ...all.map((d) => {
        const li = document.createElement('li');
        li.className = d.severity;
        li.textContent = (d.file ? `${d.file}:${d.line}:${d.column}: ` : '') + `${d.severity}: ${d.summary}` + (d.detail ? `: ${d.detail}` : '');
        return li;
      }),
    );
    list.hidden = all.length === 0;
    const errors = serverDiags.some((d) => d.severity === 'error');
    document.body.classList.toggle('stale', errors);
    status.textContent = errors ? 'The project has errors; showing the last good drawing (read-only).' : '';
  };

  const show = (d: Drawing, family: string) => {
    paper.svg.setAttribute('font-family', `${family}, sans-serif`);
    paper.defs.querySelectorAll('[data-dg]').forEach((el) => el.remove());
    const defs = new DOMParser().parseFromString(`<svg xmlns="http://www.w3.org/2000/svg">${defsMarkup(d.nodes, d).join('')}</svg>`, 'image/svg+xml');
    for (const el of [...defs.documentElement.children]) {
      el.setAttribute('data-dg', '');
      paper.defs.appendChild(document.importNode(el, true));
    }
    // In a view with nothing pinned every node is unpinned, so fading tells nothing; a hint says
    // the layout is not saved instead (spec 7.4).
    const anyPinned = d.nodes.some((n) => n.pinned);
    hint.hidden = anyPinned;
    const nodes = d.nodes.map(
      (n, i) =>
        new Markup({
          id: 'n:' + n.id,
          position: { x: n.box.x, y: n.box.y },
          size: { width: n.box.w, height: n.box.h },
          markup: nodeMarkup(shiftNode(n, -n.box.x, -n.box.y), d, d).join(''),
          attrs: { root: { opacity: n.pinned || !anyPinned ? 1 : UNPINNED_OPACITY } },
          z: i,
        }),
    );
    const periods = flowPeriods(d.edges);
    const edges = d.edges.map(
      (e, i) => new Markup({ id: 'e:' + e.id, position: { x: 0, y: 0 }, size: { width: 1, height: 1 }, markup: edgeMarkup(e, d, periods).join(''), z: nodes.length + i }),
    );
    flowStyle.textContent = flowCSS(periods);
    checkStyle(flowStyle);
    graph.resetCells([...nodes, ...edges]);
    paper.setDimensions(d.bounds.w, d.bounds.h);
    paper.translate(-d.bounds.x, -d.bounds.y);
    paper.el.dataset.view = d.view;
  };

  const drawLatest = async () => {
    if (drawing) return;
    drawing = true;
    try {
      while (pending) {
        const view = pending;
        pending = undefined;
        try {
          const { drawing: d, family } = await render(view);
          show(d, family);
          tooSmall = d.nodes.filter((n) => n.tooSmall).map((n) => n.id);
        } catch (err) {
          status.textContent = `Drawing failed: ${err}`;
        }
        showDiagnostics();
      }
    } finally {
      drawing = false;
    }
  };

  const showViews = (views: ViewInfo[], current: string) => {
    select.replaceChildren(
      ...views.map((v) => {
        const o = document.createElement('option');
        o.value = v.id;
        o.textContent = v.title ? `${v.title} (${v.id})` : v.id;
        return o;
      }),
    );
    select.value = current;
  };

  const conn = connect(
    (m: ServerMessage) => {
      switch (m.type) {
        case 'snapshot':
          showViews(m.views, m.resolved.view);
          pending = m.resolved;
          void drawLatest();
          break;
        case 'diagnostics':
          serverDiags = m.diagnostics;
          showDiagnostics();
          break;
        case 'error':
          status.textContent = m.message;
          break;
      }
    },
    (open) => document.body.classList.toggle('disconnected', !open),
  );
  select.addEventListener('change', () => conn.selectView(select.value));
}

import '@fontsource/inter/400.css';
import ELK from 'elkjs/lib/elk-api.js';
import elkWorkerUrl from 'elkjs/lib/elk-worker.min.js?url';
import libavoidUrl from '../node_modules/libavoid-js/dist/libavoid.wasm?url';
import { routeAvoid } from './canvas/avoid';
import { toJSON, toSVG } from './canvas/export';
import { canvasMeasure, loadFont } from './canvas/measure';
import type { ResolvedView } from './protocol/view';
import { draw } from './render';

declare global {
  interface Window {
    dg?: { render(view: ResolvedView): Promise<{ svg: string; json: string }> };
  }
}

const blocked: string[] = [];
document.addEventListener('securitypolicyviolation', (e) => blocked.push(e.blockedURI));

window.dg = {
  async render(view) {
    await loadFont(view.nodes.map((n) => n.label).concat(view.edges.map((e) => e.labels.mid ?? '')));
    const elk = new ELK({ workerUrl: elkWorkerUrl });
    try {
      const drawing = await draw(view, canvasMeasure(), elk, (boxes, ids, obstacles, edges) =>
        routeAvoid(boxes, ids, obstacles, edges, libavoidUrl),
      );
      if (blocked.length) throw new Error('blocked requests: ' + blocked.join(', '));
      return { svg: toSVG(drawing), json: toJSON(drawing) };
    } finally {
      elk.terminateWorker();
    }
  },
};

import '@fontsource/inter/400.css';
import '@fontsource/inter/700.css';
import './app.css';
import ELK from 'elkjs/lib/elk-api.js';
import elkWorkerUrl from 'elkjs/lib/elk-worker.min.js?url';
import libavoidUrl from '../node_modules/libavoid-js/dist/libavoid.wasm?url';
import { avoidRouter } from './canvas/avoid';
import { startEditor } from './canvas/editor';
import { toJSON, toSVG, type Drawing } from './canvas/export';
import { canvasMeasure, loadFonts } from './canvas/measure';
import type { Elk } from './place/elk';
import type { ResolvedView } from './protocol/view';
import { draw } from './render';

declare global {
  interface Window {
    dg?: { render(view: ResolvedView): Promise<{ svg: string; json: string; too_small: string[] }> };
  }
}

const router = avoidRouter(libavoidUrl);

async function render(view: ResolvedView, elk: Elk): Promise<{ drawing: Drawing; family: string }> {
  const font = await loadFonts(view);
  return { drawing: await draw(view, canvasMeasure(font.family), font.name, elk, router), family: font.family };
}

if (new URLSearchParams(location.search).has('export')) {
  const blocked: string[] = [];
  document.addEventListener('securitypolicyviolation', (e) => blocked.push(e.blockedURI));
  window.dg = {
    async render(view) {
      const elk = new ELK({ workerUrl: elkWorkerUrl });
      try {
        const { drawing } = await render(view, elk);
        if (blocked.length) throw new Error('blocked requests: ' + blocked.join(', '));
        return { svg: toSVG(drawing), json: toJSON(drawing), too_small: drawing.nodes.filter((n) => n.tooSmall).map((n) => n.id) };
      } finally {
        elk.terminateWorker();
      }
    },
  };
} else {
  const elk = new ELK({ workerUrl: elkWorkerUrl });
  startEditor((view) => render(view, elk));
}

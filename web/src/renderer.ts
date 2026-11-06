import ELK from 'elkjs/lib/elk-api.js';
import elkWorkerUrl from 'elkjs/lib/elk-worker.min.js?url';
import libavoidUrl from '../node_modules/libavoid-js/dist/libavoid.wasm?url';
import { avoidRouter } from './canvas/avoid';
import type { Drawing } from './canvas/export';
import { canvasMeasure, loadFonts } from './canvas/measure';
import type { Elk } from './place/elk';
import type { ResolvedView } from './protocol/view';
import { draw } from './render';

const router = avoidRouter(libavoidUrl);

// newElk starts the layered placement in a worker.
export const newElk = () => new ELK({ workerUrl: elkWorkerUrl });

// render draws view with the bundled font or the project's.
export async function render(view: ResolvedView, elk: Elk): Promise<{ drawing: Drawing; family: string }> {
  const font = await loadFonts(view);
  return { drawing: await draw(view, canvasMeasure(font.family), font.name, elk, router), family: font.family };
}

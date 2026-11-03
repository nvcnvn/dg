import { expect, it } from 'vitest';
import { avoidRouter } from './canvas/avoid';
import { edge, node, text } from './fixtures';
import type { ResolvedView, ViewEdge, ViewNode } from './protocol/view';
import { draw } from './render';

const wasm = decodeURIComponent(new URL('../node_modules/libavoid-js/dist/libavoid.wasm', import.meta.url).pathname);
const noElk = { layout: () => Promise.reject(new Error('elk is not used when a node is pinned')) };

// bigView is n nodes in n/20 containers, one pinned so the spiral placer runs, and 2n edges
// between random nodes from a fixed seed.
function bigView(n: number): ResolvedView {
  let seed = 1;
  const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
  const groups = Math.max(1, Math.floor(n / 20));
  const id = (i: number) => 'n' + String(i).padStart(3, '0');
  const nodes: ViewNode[] = [];
  for (let g = 0; g < groups; g++) nodes.push(node('g' + g, { container: true, pin: g === 0 ? { x: 0, y: 0 } : undefined }, { padding: 20 }));
  for (let i = 0; i < n; i++) nodes.push(node(id(i), { frame: 'g' + (i % groups) }));
  const edges: ViewEdge[] = [];
  for (let j = 0; j < 2 * n; j++) {
    const a = Math.floor(rnd() * n);
    const b = (Math.floor(rnd() * n) + (a === 0 ? 1 : 0)) % n;
    edges.push(edge('e' + String(j).padStart(3, '0'), id(a), id(b === a ? (b + 1) % n : b)));
  }
  return { view: 'big', grid: 10, nodes, edges };
}

// The drag budget of FR-8 is M10's; this bounds a whole draw so routing cannot blow up again.
// Nearly all of it is libavoid's own routing of this dense graph: about 3.3 seconds on an M-series
// Mac, where a pin or nudging option that makes libavoid slow costs minutes.
it('draws 200 nodes and 400 edges within 8 seconds', async () => {
  const view = bigView(200);
  const start = performance.now();
  const d = await draw(view, text, 'Inter', noElk, avoidRouter(wasm));
  const ms = performance.now() - start;
  console.log(`draw 200/400: ${Math.round(ms)} ms`);
  expect(d.edges).toHaveLength(400);
  expect(ms).toBeLessThan(8000);
}, 120000);

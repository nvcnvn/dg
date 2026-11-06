import '@fontsource/inter/400.css';
import '@fontsource/inter/700.css';
import './theme.css';
import './app.css';
import { startEditor } from './canvas/editor';
import { mountSVG, picture, setClock, toJSON, toSVG } from './canvas/export';
import { followHost } from './canvas/host';
import type { ResolvedView } from './protocol/view';
import { newElk, render } from './renderer';

declare global {
  interface Window {
    dg?: {
      render(view: ResolvedView): Promise<{ svg: string; json: string; too_small: string[]; overflowing: string[] }>;
      setClock(ms: number): Promise<void>;
    };
  }
}

if (new URLSearchParams(location.search).has('export')) {
  const blocked: string[] = [];
  document.addEventListener('securitypolicyviolation', (e) => blocked.push(e.blockedURI));
  window.dg = {
    async render(view) {
      const elk = newElk();
      try {
        const { drawing: d, family } = await render(view, elk);
        const drawing = picture(d);
        const svg = toSVG(drawing);
        await mountSVG(svg, family);
        if (blocked.length) throw new Error('blocked requests: ' + blocked.join(', '));
        return { svg, json: toJSON(drawing), too_small: drawing.nodes.filter((n) => n.tooSmall).map((n) => n.id), overflowing: drawing.nodes.filter((n) => n.overflows).map((n) => n.id) };
      } finally {
        elk.terminateWorker();
      }
    },
    setClock,
  };
} else {
  followHost();
  const elk = newElk();
  startEditor((view) => render(view, elk));
}

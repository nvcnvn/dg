import { describe, expect, it } from 'vitest';
import { marker, markerLength } from './markers';

const right = [{ x: 0, y: 0 }, { x: 100, y: 0 }];
const down = [{ x: 0, y: 0 }, { x: 0, y: 100 }];

describe('marker', () => {
  it('points an arrow along the last segment', () => {
    expect(marker('arrow', right)).toEqual([{ line: [{ x: 90, y: 5 }, { x: 100, y: 0 }, { x: 90, y: -5 }] }]);
    expect(marker('arrow', down)).toEqual([{ line: [{ x: -5, y: 90 }, { x: 0, y: 100 }, { x: 5, y: 90 }] }]);
  });
  it('orients by the last segment of a bent route, skipping repeated points', () => {
    expect(marker('triangle', [{ x: 0, y: 0 }, { x: 50, y: 0 }, { x: 50, y: 40 }, { x: 50, y: 40 }])).toEqual([
      { polygon: [{ x: 50, y: 40 }, { x: 44, y: 28 }, { x: 56, y: 28 }], filled: true },
    ]);
  });
  it('spreads a crow foot at the end and puts the bar and ring behind it', () => {
    const [foot, ring] = marker('crow-zero-many', right);
    expect(foot).toEqual({ line: [{ x: 100, y: 7 }, { x: 88, y: 0 }, { x: 100, y: -7 }] });
    expect(ring).toEqual({ circle: { x: 81, y: 0 }, r: 5, filled: false });
    expect(marker('crow-one', right)).toHaveLength(2);
  });
  it.each(['none', 'arrow', 'triangle', 'triangle-open', 'diamond', 'diamond-open', 'circle', 'circle-open', 'crow-one', 'crow-zero-one', 'crow-many', 'crow-one-many', 'crow-zero-many'])(
    '%s stays within its length',
    (name) => {
      for (const p of marker(name, right)) {
        const pts = 'circle' in p ? [{ x: p.circle.x - p.r, y: 0 }] : 'line' in p ? p.line : p.polygon;
        for (const q of pts) expect(100 - q.x).toBeLessThanOrEqual(markerLength(name));
      }
    },
  );
  it('draws nothing on a route without length', () => {
    expect(marker('arrow', [{ x: 1, y: 1 }, { x: 1, y: 1 }])).toEqual([]);
  });
});

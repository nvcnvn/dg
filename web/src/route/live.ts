import { center, cmp, type Box, type Point } from '../geometry/geometry';
import type { ResolvedView, Side } from '../protocol/view';
import { join, nearestSide, STUB, step, type RouteRequest } from './edges';
import { clip } from './route';

// Routes drawn live while nodes are dragged (spec 7.5): only the edges with one end on a
// moving node are routed again, by a router that keeps its obstacles between frames. An end at a
// node without a port, row or side hint is routed from the node's center and clipped to its box;
// any other end keeps the point and side it was drawn at, moving with its node. The routes are
// previews: the drawing after the drop routes every edge by section 7.5.

export interface LiveEnd {
  node: string;
  at: Point;
  dir: Side | 'all';
  moves: boolean;
}

export interface LiveEdge {
  id: string;
  routing: 'orthogonal' | 'polyline' | 'straight';
  from: LiveEnd;
  to: LiveEnd;
  via: Point[];
}

// liveEdges returns the edges of view with one end on a node of moved, in ID order, given the
// nodes' boxes and the edges' drawn routes.
export function liveEdges(view: ResolvedView, boxes: Map<string, Box>, routes: Map<string, Point[]>, moved: Set<string>): LiveEdge[] {
  const containers = new Set(view.nodes.filter((n) => n.container).map((n) => n.id));
  return view.edges
    .filter((e) => moved.has(e.from.node) !== moved.has(e.to.node) && routes.get(e.id)!.length > 1)
    .sort((a, b) => cmp(a.id, b.id))
    .map((e) => {
      const pts = routes.get(e.id)!;
      const end = (i: 0 | 1): LiveEnd => {
        const v = i === 0 ? e.from : e.to;
        const b = boxes.get(v.node)!;
        const side = i === 0 ? e.hint?.from_side : e.hint?.to_side;
        if (!containers.has(v.node) && !v.port && !v.row && !side) return { node: v.node, at: center(b), dir: 'all', moves: moved.has(v.node) };
        const p = i === 0 ? pts[0] : pts[pts.length - 1];
        return { node: v.node, at: p, dir: nearestSide(b, p), moves: moved.has(v.node) };
      };
      return { id: e.id, routing: e.style.routing, from: end(0), to: end(1), via: (e.hint?.via ?? []).map(([x, y]) => ({ x, y })) };
    });
}

function shifted(e: LiveEnd, d: Point): Point {
  return e.moves ? { x: e.at.x + d.x, y: e.at.y + d.y } : e.at;
}

// liveRequest returns what the router routes for edge e with its moving end moved by d: an end
// with a direction from a free point a stub outside it.
export function liveRequest(e: LiveEdge, d: Point): RouteRequest {
  const end = (x: LiveEnd) => {
    const at = shifted(x, d);
    if (x.dir === 'all') return { at, obstacle: x.node };
    return { at: { x: at.x + STUB * step[x.dir].x, y: at.y + STUB * step[x.dir].y } };
  };
  return { id: e.id, type: e.routing === 'polyline' ? 'polyline' : 'orthogonal', from: end(e.from), to: end(e.to), via: e.via };
}

// liveRoute finishes the route raw of edge e moved by d: joined to its ends with a direction and
// clipped to the boxes of the others; a straight edge, or one without a route, is a line between
// its ends.
export function liveRoute(e: LiveEdge, raw: Point[] | undefined, d: Point, box: (id: string) => Box): Point[] {
  const from = { at: shifted(e.from, d), dir: e.from.dir };
  const to = { at: shifted(e.to, d), dir: e.to.dir };
  let pts = raw && raw.length > 1 && e.routing !== 'straight' ? [...raw] : [from.at, ...e.via, to.at];
  if (from.dir !== 'all') pts = [...join(from, pts[0]), ...pts];
  if (to.dir !== 'all') pts = [...pts, ...join(to, pts[pts.length - 1]).reverse()];
  const rect = (x: LiveEnd) => {
    if (x.dir !== 'all') return null;
    const b = box(x.node);
    return [
      { x: b.x, y: b.y },
      { x: b.x + b.w, y: b.y },
      { x: b.x + b.w, y: b.y + b.h },
      { x: b.x, y: b.y + b.h },
    ];
  };
  return clip(pts, rect(e.from), rect(e.to));
}

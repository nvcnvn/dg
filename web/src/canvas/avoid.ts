import { AvoidLib } from 'libavoid-js';
import { center, cmp, type Box, type Point } from '../geometry/geometry';
import { BUFFER, joinLegs, type RouteEnd, type RouteRequest, type Router } from '../route/edges';

interface Handle {
  readonly brand?: unknown;
}

interface Route {
  size(): number;
  get_ps(i: number): Point;
}

// Avoid is the part of the libavoid-js module dg uses, typed here because the package's own
// typings do not match the module (ConnEnd, destroy).
interface Avoid {
  OrthogonalRouting: number;
  PolyLineRouting: number;
  ConnType_Orthogonal: number;
  ConnType_PolyLine: number;
  ConnDirAll: number;
  shapeBufferDistance: number;
  idealNudgingDistance: number;
  nudgeOrthogonalTouchingColinearSegments: number;
  performUnifyingNudgingPreprocessingStep: number;
  nudgeSharedPathsWithCommonEndPoint: number;
  nudgeOrthogonalSegmentsConnectedToShapes: number;
  Router: new (flags: number) => Handle & {
    setRoutingParameter(p: number, v: number): void;
    setRoutingOption(o: number, v: boolean): void;
    processTransaction(): void;
    moveShape(shape: Handle, dx: number, dy: number): void;
  };
  Point: new (x: number, y: number) => Handle;
  Rectangle: new (topLeft: Handle, bottomRight: Handle) => Handle;
  ShapeRef: new (router: Handle, poly: Handle) => Handle;
  ShapeConnectionPin: new (shape: Handle, id: number, x: number, y: number, proportional: boolean, inside: number, dirs: number) => Handle & {
    setExclusive(v: boolean): void;
  };
  ConnEnd: { new (p: Handle): Handle; new (shape: Handle, pin: number): Handle };
  ConnRef: new (router: Handle, from: Handle, to: Handle) => Conn;
  destroy(o: unknown): void;
}

type Conn = Handle & {
  setRoutingType(t: number): void;
  displayRoute(): Route;
  setSourceEndpoint(e: Handle): void;
  setDestEndpoint(e: Handle): void;
};

const CENTER_PIN = 1;

// seesPin reports whether a leg to the free point to, from the pin at the center of from, can
// abort libavoid: on one of the pin's center lines, the point can see the pin, which the search
// for the target's directions takes for the target's own (see docs/tech-debts.md). How far it sees
// depends on the lines the sweep draws, so a leg to any such point runs the other way around.
function seesPin(from: Box, to: Point): boolean {
  const c = center(from);
  return (to.x === c.x) !== (to.y === c.y);
}

// pinToFree returns the free point the first leg of an orthogonal request goes to from a pin.
function pinToFree(e: RouteRequest): Point | undefined {
  if (e.type !== 'orthogonal' || e.from.obstacle === undefined) return undefined;
  return e.via[0] ?? (e.to.obstacle === undefined ? e.to.at : undefined);
}

// UNIFY_MAX_EDGES bounds the views libavoid's unifying nudging step runs for. The step keeps
// parallel segments of different edges apart, but its cost grows steeply: about 20 ms at 50
// edges, 3 seconds at 100 and minutes at 400.
const UNIFY_MAX_EDGES = 60;

let loaded: Promise<void> | undefined;

// current returns the libavoid module, or nothing while it loads or after an abort.
const current = () => (AvoidLib as unknown as { avoidLib?: Avoid }).avoidLib;

// avoidRouter returns a router that runs libavoid, loaded once from wasmUrl. Each call routes
// every request with a fresh libavoid router, obstacles and connectors added in sorted ID
// order so the result depends only on the input. An end at an obstacle connects to a pin at its
// center; any other end is a free point. libavoid binds no checkpoints, so a route through via
// points is the concatenation of one connector per leg.
//
// Pins with directions on box borders make libavoid's search about fifty times slower, so the
// caller gives an end with a direction as a free point outside the obstacle's buffer instead.
export function avoidRouter(wasmUrl: string): Router {
  return async (obstacles, edges) => {
    while (!current()) await (loaded ??= AvoidLib.load(wasmUrl));
    const r = session(obstacles, edges, (e) => {
      const to = pinToFree(e);
      return to !== undefined && seesPin(obstacles.get(e.from.obstacle!)!, to);
    });
    try {
      const routes = r.routes();
      r.destroy();
      return routes;
    } catch (err) {
      throw restart(err, edges);
    }
  };
}

// restart gives up on the libavoid module after an abort, which leaves it unusable, so that the
// next route loads it again, and returns the error to report.
function restart(err: unknown, edges: RouteRequest[]): Error {
  loaded = undefined;
  (AvoidLib as unknown as { avoidLib?: unknown }).avoidLib = undefined;
  const vias = edges.filter((e) => e.via.length).map((e) => `${e.id} via ${e.via.map((p) => `[${p.x},${p.y}]`).join(' ')}`);
  return new Error(`libavoid failed routing this view (${vias.length ? `edges with a via: ${vias.join(', ')}` : 'no edge has a via'}): ${err}`);
}

// LiveRouter keeps libavoid's obstacles and connectors between calls, so a drag routes only
// what moved (spec 7.5).
export interface LiveRouter {
  // route moves the obstacles of moving by d from where they were given, and the free ends
  // whose points changed, and routes the requests again; they are the requests given, in order,
  // with other points.
  route(requests: RouteRequest[], d: Point): Map<string, Point[]>;
  destroy(): void;
}

// liveRouter returns a live router for the obstacles and requests, or undefined while libavoid
// is not loaded. Obstacles and free ends move every frame, so each leg from a pin to a free point
// runs the other way around. After a failure, its own or one elsewhere that took the module with
// it, the router routes nothing, which draws the rest of the drag as lines; the drawing after the
// drop reports it.
export function liveRouter(obstacles: Map<string, Box>, moving: Set<string>, requests: RouteRequest[]): LiveRouter | undefined {
  if (!current()) return undefined;
  const r = session(obstacles, requests, (e) => pinToFree(e) !== undefined);
  let applied = { x: 0, y: 0 };
  let last = requests;
  let failed = false;
  const move = (next: RouteRequest[], d: Point) => {
    for (const id of [...moving].sort(cmp)) {
      const shape = r.shapes.get(id);
      if (shape) r.router.moveShape(shape, d.x - applied.x, d.y - applied.y);
    }
    applied = d;
    next.forEach((e, i) => {
      const legs = r.legs.get(e.id)!;
      const was = last[i];
      if (!e.from.obstacle && (e.from.at.x !== was.from.at.x || e.from.at.y !== was.from.at.y)) legs[0].setSourceEndpoint(r.end(e.from));
      if (!e.to.obstacle && (e.to.at.x !== was.to.at.x || e.to.at.y !== was.to.at.y)) {
        const leg = legs[legs.length - 1];
        if (r.reversed.has(leg)) leg.setSourceEndpoint(r.end(e.to));
        else leg.setDestEndpoint(r.end(e.to));
      }
    });
    last = next;
  };
  return {
    route(next, d) {
      failed ||= r.A !== current();
      if (failed) return new Map();
      try {
        move(next, d);
        return r.routes();
      } catch (err) {
        failed = true;
        console.error(restart(err, next));
        return new Map();
      }
    },
    destroy() {
      if (!failed && r.A === current()) r.destroy();
    },
  };
}

// session builds a libavoid router holding the obstacles and one connector per leg of each
// request, all added in sorted ID order so the result depends only on the input. The first leg of
// a request reverse holds for is routed from its far end.
function session(obstacles: Map<string, Box>, edges: RouteRequest[], reverse: (e: RouteRequest) => boolean) {
  const A = current()!;
  const polyline = edges.some((e) => e.type === 'polyline');
  const router = new A.Router(A.OrthogonalRouting | (polyline ? A.PolyLineRouting : 0));
  const temporaries: unknown[] = [];
  const tmp = <T>(o: T): T => {
    temporaries.push(o);
    return o;
  };
  router.setRoutingParameter(A.shapeBufferDistance, BUFFER);
  router.setRoutingParameter(A.idealNudgingDistance, 20);
  router.setRoutingOption(A.nudgeOrthogonalTouchingColinearSegments, false);
  router.setRoutingOption(A.performUnifyingNudgingPreprocessingStep, edges.length <= UNIFY_MAX_EDGES);
  router.setRoutingOption(A.nudgeSharedPathsWithCommonEndPoint, true);
  router.setRoutingOption(A.nudgeOrthogonalSegmentsConnectedToShapes, true);
  const point = (p: Point) => tmp(new A.Point(p.x, p.y));

  const shapes = new Map<string, Handle>();
  for (const id of [...obstacles.keys()].sort(cmp)) {
    const b = obstacles.get(id)!;
    const shape = new A.ShapeRef(router, tmp(new A.Rectangle(point(b), point({ x: b.x + b.w, y: b.y + b.h }))));
    new A.ShapeConnectionPin(shape, CENTER_PIN, 0.5, 0.5, true, 0, A.ConnDirAll).setExclusive(false);
    shapes.set(id, shape);
  }
  const end = (e: RouteEnd) => {
    const shape = e.obstacle !== undefined ? shapes.get(e.obstacle) : undefined;
    return tmp(shape ? new A.ConnEnd(shape, CENTER_PIN) : new A.ConnEnd(point(e.at)));
  };

  const legs = new Map<string, Conn[]>();
  const reversed = new Set<Conn>();
  for (const e of edges) {
    const stops = [end(e.from), ...e.via.map((p) => tmp(new A.ConnEnd(point(p)))), end(e.to)];
    const conns = [];
    for (let i = 0; i + 1 < stops.length; i++) {
      const back = i === 0 && reverse(e);
      const c = new A.ConnRef(router, stops[back ? 1 : i], stops[back ? 0 : i + 1]);
      if (back) reversed.add(c);
      c.setRoutingType(e.type === 'polyline' ? A.ConnType_PolyLine : A.ConnType_Orthogonal);
      conns.push(c);
    }
    legs.set(e.id, conns);
  }

  const routes = () => {
    router.processTransaction();
    const out = new Map<string, Point[]>();
    for (const e of edges) {
      const points = legs.get(e.id)!.map((c) => {
        const route = c.displayRoute();
        const pts = Array.from({ length: route.size() }, (_, i) => {
          const p = route.get_ps(i);
          return { x: p.x, y: p.y };
        });
        return reversed.has(c) ? pts.reverse() : pts;
      });
      out.set(e.id, joinLegs(points, e.via));
    }
    return out;
  };
  const destroy = () => {
    for (const o of temporaries) A.destroy(o);
    A.destroy(router);
  };
  return { A, router, shapes, legs, reversed, end, routes, destroy };
}

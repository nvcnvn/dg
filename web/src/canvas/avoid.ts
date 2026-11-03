import { AvoidLib } from 'libavoid-js';
import { cmp, type Box, type Point } from '../geometry/geometry';
import { BUFFER, type RouteEnd, type RouteRequest, type Router } from '../route/edges';

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

// UNIFY_MAX_EDGES bounds the views libavoid's unifying nudging step runs for. The step keeps
// parallel segments of different edges apart, but its cost grows steeply: about 20 ms at 50
// edges, 3 seconds at 100 and minutes at 400.
const UNIFY_MAX_EDGES = 60;

let loaded: Promise<void> | undefined;

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
    await (loaded ??= AvoidLib.load(wasmUrl));
    const r = session(obstacles, edges);
    try {
      return r.routes();
    } finally {
      r.destroy();
    }
  };
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
// is not loaded yet.
export function liveRouter(obstacles: Map<string, Box>, moving: Set<string>, requests: RouteRequest[]): LiveRouter | undefined {
  if (!loaded) return undefined;
  const r = session(obstacles, requests);
  let applied = { x: 0, y: 0 };
  let last = requests;
  return {
    route(next, d) {
      for (const id of [...moving].sort(cmp)) {
        const shape = r.shapes.get(id);
        if (shape) r.router.moveShape(shape, d.x - applied.x, d.y - applied.y);
      }
      applied = d;
      next.forEach((e, i) => {
        const legs = r.legs.get(e.id)!;
        const was = last[i];
        if (!e.from.obstacle && (e.from.at.x !== was.from.at.x || e.from.at.y !== was.from.at.y)) legs[0].setSourceEndpoint(r.end(e.from));
        if (!e.to.obstacle && (e.to.at.x !== was.to.at.x || e.to.at.y !== was.to.at.y)) legs[legs.length - 1].setDestEndpoint(r.end(e.to));
      });
      last = next;
      return r.routes();
    },
    destroy: r.destroy,
  };
}

// session builds a libavoid router holding the obstacles and one connector per leg of each
// request, all added in sorted ID order so the result depends only on the input.
function session(obstacles: Map<string, Box>, edges: RouteRequest[]) {
  const A = AvoidLib.getInstance() as unknown as Avoid;
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
  for (const e of edges) {
    const stops = [end(e.from), ...e.via.map((p) => tmp(new A.ConnEnd(point(p)))), end(e.to)];
    const conns = [];
    for (let i = 0; i + 1 < stops.length; i++) {
      const c = new A.ConnRef(router, stops[i], stops[i + 1]);
      c.setRoutingType(e.type === 'polyline' ? A.ConnType_PolyLine : A.ConnType_Orthogonal);
      conns.push(c);
    }
    legs.set(e.id, conns);
  }

  const routes = () => {
    router.processTransaction();
    const out = new Map<string, Point[]>();
    for (const e of edges) {
      const pts: Point[] = [];
      for (const c of legs.get(e.id)!) {
        const route = c.displayRoute();
        for (let i = pts.length ? 1 : 0; i < route.size(); i++) {
          const p = route.get_ps(i);
          pts.push({ x: p.x, y: p.y });
        }
      }
      out.set(e.id, pts);
    }
    return out;
  };
  const destroy = () => {
    for (const o of temporaries) A.destroy(o);
    A.destroy(router);
  };
  return { router, shapes, legs, end, routes, destroy };
}

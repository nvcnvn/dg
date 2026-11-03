import { dia, shapes } from '@joint/core';
import { initAvoidRouter } from '@joint/router-avoid';
import type { Box, Point } from '../geometry/geometry';

export interface RouteRequest {
  id: string;
  from: string;
  to: string;
}

// routeAvoid routes edges between boxes with libavoid. Only boxes listed in
// obstacles are avoided; an edge ending at any other box gets the fallback
// right-angle route. Routes run from box centers; clip them to the borders.
export async function routeAvoid(
  boxes: Map<string, Box>,
  ids: string[],
  obstacles: Set<string>,
  edges: RouteRequest[],
  libavoidUrl: string,
): Promise<Map<string, Point[]>> {
  const graph = new dia.Graph({}, { cellNamespace: shapes });
  graph.addCells(
    ids.map((id) => {
      const b = boxes.get(id)!;
      return new shapes.standard.Rectangle({ id, position: { x: b.x, y: b.y }, size: { width: b.w, height: b.h } });
    }),
  );
  graph.addCells(edges.map((e) => new shapes.standard.Link({ id: 'e:' + e.id, source: { id: e.from }, target: { id: e.to } })));

  const router = await initAvoidRouter(graph, {
    shapeBufferDistance: 10,
    idealNudgingDistance: 20,
    libavoidFilePath: libavoidUrl,
    trackElement: ({ element }) => obstacles.has(String(element.id)),
  });
  try {
    await router.routeAll();
  } finally {
    router.destroy();
  }

  const routes = new Map<string, Point[]>();
  for (const e of edges) {
    const link = graph.getCell('e:' + e.id) as dia.Link;
    const end = (def: dia.Link.EndJSON, box: Box): Point => {
      const args = (def.anchor?.args ?? {}) as { dx?: number; dy?: number };
      return { x: box.x + box.w / 2 + (args.dx ?? 0), y: box.y + box.h / 2 + (args.dy ?? 0) };
    };
    const vertices = (link.vertices() ?? []).map((v) => ({ x: v.x, y: v.y }));
    routes.set(e.id, [end(link.source(), boxes.get(e.from)!), ...vertices, end(link.target(), boxes.get(e.to)!)]);
  }
  return routes;
}

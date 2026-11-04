import type { NodeStyle, Policy, ResolvedView, ViewNode } from '../protocol/view';
import { ceilTo, type Axis, type NodeMeasure, type Point, type Size, type Stretched } from './geometry';

const ALIGN = { start: 0, center: 0.5, end: 1 };

// arrange places the children of a stack or grid container (spec 7.3), given in policy order
// with their sizes, relative to the container's content origin, and returns their positions
// and drawn sizes. A stack puts them one after another along its direction, gap apart; across
// it, they are cross wide, or with stretch exactly that wide, else aligned. A grid fills its
// columns row by row, each column as wide as its widest child and each row as tall as its
// tallest, gap apart, a child aligned in its cell both ways.
export function arrange(p: Policy, sizes: Size[], cross: number): { rel: Point[]; sizes: Size[] } {
  const f = ALIGN[p.align];
  if (p.layout === 'grid') {
    const cols = Math.max(1, p.columns);
    const colW: number[] = [];
    const rowH: number[] = [];
    sizes.forEach((s, i) => {
      colW[i % cols] = Math.max(colW[i % cols] ?? 0, s.w);
      rowH[Math.floor(i / cols)] = Math.max(rowH[Math.floor(i / cols)] ?? 0, s.h);
    });
    const starts = (list: number[]) => list.map((_, i) => list.slice(0, i).reduce((a, v) => a + v + p.gap, 0));
    const xs = starts(colW);
    const ys = starts(rowH);
    const rel = sizes.map((s, i) => {
      const c = i % cols;
      const r = Math.floor(i / cols);
      return { x: Math.round(xs[c] + (colW[c] - s.w) * f), y: Math.round(ys[r] + (rowH[r] - s.h) * f) };
    });
    return { rel, sizes };
  }
  const vertical = p.direction === 'vertical';
  const across = (s: Size) => (vertical ? s.w : s.h);
  const along = (s: Size) => (vertical ? s.h : s.w);
  const out = sizes.map((s) => (p.stretch ? (vertical ? { w: cross, h: s.h } : { w: s.w, h: cross }) : s));
  let at = 0;
  const rel = out.map((s) => {
    const off = Math.round((cross - across(s)) * f);
    const pos = vertical ? { x: off, y: at } : { x: at, y: off };
    at += along(s) + p.gap;
    return pos;
  });
  return { rel, sizes: out };
}

// arrangeFrame places the children of stack or grid container frame, whose sizes are known,
// setting their positions and drawn sizes. Across a stack they have the room inside the
// container, exactly when its size is set that way (pinned, or given in size when an outer stack
// stretched it), else at least the widest child. A child with a policy whose size the stretch
// changed is arranged again in its new size.
export function arrangeFrame(byId: Map<string, ViewNode>, measures: Map<string, NodeMeasure>, frame: string, rel: Map<string, Point>, sizes: Map<string, Size>, size?: Size) {
  const n = byId.get(frame)!;
  const p = n.policy!;
  const kids = p.order.map((id) => sizes.get(id)!);
  const vertical = p.direction === 'vertical';
  const m = measures.get(frame)!;
  const set = vertical ? (size?.w ?? n.pin?.w) : (size?.h ?? n.pin?.h);
  const room = (set ?? (vertical ? m.size.w : m.size.h)) - (vertical ? m.origin.x : m.origin.y) - n.style.padding;
  const cross = set !== undefined ? Math.max(0, room) : Math.max(room, ...kids.map((s) => (vertical ? s.w : s.h)));
  const out = arrange(p, kids, cross);
  p.order.forEach((id, i) => {
    rel.set(id, out.rel[i]);
    sizes.set(id, out.sizes[i]);
    const was = kids[i];
    if (byId.get(id)!.policy && (was.w !== out.sizes[i].w || was.h !== out.sizes[i].h)) arrangeFrame(byId, measures, id, rel, sizes, out.sizes[i]);
  });
}

// crossAxis returns the axis of the size of its children a container's policy sets, if it does.
function crossAxis(p: Policy | undefined): Axis | undefined {
  if (!p?.stretch || p.layout !== 'stack') return undefined;
  return p.direction === 'vertical' ? 'w' : 'h';
}

// stretchedAxes returns, for each child of a stretched stack, the axis of its size arrange sets.
export function stretchedAxes(view: ResolvedView): Map<string, Stretched> {
  const axes = new Map<string, Stretched>();
  for (const { policy: p } of view.nodes) {
    const axis = crossAxis(p);
    if (axis) for (const id of p!.order) axes.set(id, { axis });
  }
  return axes;
}

// stretchedSizes adds to stretchedAxes the size arrange will set (spec 7.2 rule 1), given measures
// in which no child is wrapped at it: for each child of a stack a pin sets the size of, directly
// or through outer stacks, and for each pinned child of a stack that takes the widest of children
// whose sizes are all known before placement. Only a child with a pin wraps differently where no
// pin sets the stack's size, and a pin gives its size whatever the wrapping, so no size here
// depends on itself.
export function stretchedSizes(view: ResolvedView, measures: Map<string, NodeMeasure>, grid: number): Map<string, Stretched> {
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  const out = stretchedAxes(view);
  const framed = new Set(view.nodes.map((n) => n.frame));
  const inside = (s: ViewNode, axis: Axis, size: number) => {
    const m = measures.get(s.id)!;
    return Math.max(0, size - (axis === 'w' ? m.origin.x : m.origin.y) - s.style.padding);
  };
  const widest = (s: ViewNode, axis: Axis): number | undefined => {
    const kids = s.policy!.order.map((id) => natural(byId.get(id)!, axis));
    if (kids.includes(undefined)) return undefined;
    return Math.max(inside(s, axis, measures.get(s.id)!.size[axis]), ...(kids as number[]));
  };
  // natural returns a node's size along axis with nothing stretching it, if that is known before
  // placement, the sizes of a stack's children first.
  const natural = (n: ViewNode, axis: Axis): number | undefined => {
    if (n.pin?.[axis] !== undefined) return n.pin[axis];
    const m = measures.get(n.id)!;
    if (!framed.has(n.id)) return m.size[axis];
    if (crossAxis(n.policy) !== axis) return undefined;
    const room = widest(n, axis);
    return room === undefined ? undefined : Math.max(m.size[axis], ceilTo(room + (axis === 'w' ? m.origin.x : m.origin.y) + n.style.padding, grid));
  };
  // set gives the children of stack s the room its size leaves them, outermost first, as arrange
  // does: all of them when that size is held by a pin, else the pinned ones.
  const set = (s: ViewNode, axis: Axis, size: number | undefined, held: boolean) => {
    const room = size === undefined ? widest(s, axis) : inside(s, axis, size);
    if (room === undefined) return;
    for (const id of s.policy!.order) {
      const kid = byId.get(id)!;
      const kidHeld = held || kid.pin?.[axis] !== undefined;
      if (kidHeld) out.set(id, { axis, size: room });
      if (crossAxis(kid.policy) === axis) set(kid, axis, room, kidHeld);
    }
  };
  for (const s of view.nodes) {
    const axis = crossAxis(s.policy);
    if (axis && crossAxis(byId.get(s.frame ?? '')?.policy) !== axis) set(s, axis, s.pin?.[axis], s.pin?.[axis] !== undefined);
  }
  return out;
}

// alignHeaders gives the container children of a stretched stack one header: in a vertical
// stack, those with their label on the left the widest label strip, and in a horizontal one,
// those with their label on top the tallest label band, so that their content lines up.
export function alignHeaders(view: ResolvedView, measures: Map<string, NodeMeasure>) {
  const byId = new Map(view.nodes.map((n) => [n.id, n]));
  for (const s of view.nodes) {
    const p = s.policy;
    if (!p?.stretch || p.layout !== 'stack') continue;
    const vertical = p.direction === 'vertical';
    const axis = vertical ? 'x' : 'y';
    const kids = p.order.map((id) => byId.get(id)!).filter((n) => n.container && (n.style.label_side === 'left') === vertical);
    const widest = Math.max(0, ...kids.map((n) => measures.get(n.id)!.origin[axis]));
    for (const n of kids) {
      const m = measures.get(n.id)!;
      const grow = widest - m.origin[axis];
      const size = vertical ? { w: m.size.w + grow, h: m.size.h } : { w: m.size.w, h: m.size.h + grow };
      measures.set(n.id, { ...m, origin: { ...m.origin, [axis]: widest }, size });
    }
  }
}

const BLOCK_STYLE: NodeStyle = { shape: 'rect', fill: 'none', stroke: 'none', stroke_width: 0, stroke_dash: [], corner_radius: 0, opacity: 1, text_color: '#202124', font_size: 14, font_weight: 400, padding: 0, min_w: 0, min_h: 0, max_w: 0, z: 0 };

// withBlock returns a view whose policy places children of its of on the canvas as that view
// with an undrawn container standing for the of, holding them: placed as one box, laid out by
// its policy (spec 8.2). Other views are returned as they are.
export function withBlock(view: ResolvedView): ResolvedView {
  const p = view.policy;
  if (!p) return view;
  const members = new Set(p.order);
  const block: ViewNode = { id: p.container!, kind: '', label: '', container: true, policy: p, style: BLOCK_STYLE };
  return { ...view, policy: undefined, nodes: [...view.nodes.map((n) => (members.has(n.id) ? { ...n, frame: block.id } : n)), block] };
}

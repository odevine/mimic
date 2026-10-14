// The geometry of selecting and moving things on the editor's preview, as pure
// functions. Every rectangle is { x, y, width, height } in the finished image's own
// pixels, the numbers the app reports for an item, and the page converts to and from
// the pixels on screen at the edges

// The corner and edge handles of a resizable item, as the sides each one moves
export const HANDLES = {
  nw: { x: -1, y: -1 },
  n: { x: 0, y: -1 },
  ne: { x: 1, y: -1 },
  e: { x: 1, y: 0 },
  se: { x: 1, y: 1 },
  s: { x: 0, y: 1 },
  sw: { x: -1, y: 1 },
  w: { x: -1, y: 0 },
};

// refOf names an item the way the page keeps its selection
export const refOf = (item) => `${item.kind}:${item.key}`;

export function contains(rect, p) {
  return p.x >= rect.x && p.x < rect.x + rect.width && p.y >= rect.y && p.y < rect.y + rect.height;
}

// selectable lists what can be picked on the preview, in draw order: the things
// that are drawn and take up room
export function selectable(items) {
  return (items || []).filter((it) => it.drawn && it.delivered.width > 0 && it.delivered.height > 0);
}

// hitTest picks the item under a point. The topmost wins, which is the last drawn,
// and it is the same one the preview outlines on hover. The refs in locked are not
// there to be hit, so the point reaches what is beneath them
export function hitTest(items, p, locked) {
  const under = selectable(items).filter((it) => contains(it.delivered, p) && !(locked && locked.has(refOf(it))));
  return under.length ? under[under.length - 1] : null;
}

// resizeRect moves the sides a handle holds by (dx, dy), and keeps the rectangle at
// least min wide and high by stopping the side that would cross the other
export function resizeRect(rect, handle, dx, dy, min = 1) {
  const h = HANDLES[handle];
  let left = rect.x;
  let top = rect.y;
  let right = rect.x + rect.width;
  let bottom = rect.y + rect.height;
  if (h.x < 0) left = Math.min(left + dx, right - min);
  if (h.x > 0) right = Math.max(right + dx, left + min);
  if (h.y < 0) top = Math.min(top + dy, bottom - min);
  if (h.y > 0) bottom = Math.max(bottom + dy, top + min);
  return { x: left, y: top, width: right - left, height: bottom - top };
}

// visibleHandles says which handles a box shows, given its size on screen in pixels.
// The corners always show. A handle on the middle of a side needs room along the side
// for the corners beside it and across the box for the one on the opposite side, or
// it would cover the middle of a small box and be pressed in place of moving it
export function visibleHandles(screenWidth, screenHeight) {
  return Object.keys(HANDLES).filter((name) => {
    const { x, y } = HANDLES[name];
    if (x !== 0 && y !== 0) return true;
    if (x === 0) return screenWidth >= 48 && screenHeight >= 24;
    return screenHeight >= 48 && screenWidth >= 24;
  });
}

export function moveRect(rect, dx, dy) {
  return { ...rect, x: rect.x + dx, y: rect.y + dy };
}

const edges = (r) => ({ x: [r.x, r.x + r.width / 2, r.x + r.width], y: [r.y, r.y + r.height / 2, r.y + r.height] });

const EDGE_NAMES = { x: ["left", "center", "right"], y: ["top", "middle", "bottom"] };

// snapTargets are the lines a moving item can snap to: the canvas's edges and middle
// and the edges and middle of the items whose refs are in snap, which are the ones
// the author marked and none by default. nameOf says what an item is called,
// and the result's names say what lies on each line, as { x: Map, y: Map } of a
// line to the edges on it, each { text, rect } with no rect for the canvas
export function snapTargets(items, canvas, except, nameOf = (it) => it.key, snap = new Set()) {
  const whole = { x: 0, y: 0, width: canvas.width, height: canvas.height };
  const names = { x: new Map(), y: new Map() };
  const add = (owner, rect) => {
    const e = edges(rect);
    for (const axis of ["x", "y"]) {
      e[axis].forEach((v, i) => {
        const on = names[axis].get(v) || [];
        on.push({ text: `${owner} ${EDGE_NAMES[axis][i]}`, rect: owner === "canvas" ? null : rect });
        names[axis].set(v, on);
      });
    }
  };
  add("canvas", whole);
  for (const it of selectable(items)) {
    if (refOf(it) !== except && snap.has(refOf(it))) add(nameOf(it), it.delivered);
  }
  const sorted = (m) => [...m.keys()].sort((a, b) => a - b);
  return { x: sorted(names.x), y: sorted(names.y), names };
}

function nearest(values, lines, threshold) {
  let best = null;
  values.forEach((v, edge) => {
    for (const line of lines) {
      const d = line - v;
      if (Math.abs(d) <= threshold && (best === null || Math.abs(d) < Math.abs(best.d))) best = { d, at: line, edge };
    }
  });
  return best;
}

// A guide says which edge of the moving rectangle is on the line and what else is on it
function guideFor(axis, hit, targets) {
  const on = targets.names ? targets.names[axis].get(hit.at) || [] : [];
  const texts = on.map((o) => o.text);
  const shown = texts.length > 2 ? [...texts.slice(0, 2), `${texts.length - 2} more`] : texts;
  const boxes = [];
  for (const o of on) if (o.rect && !boxes.includes(o.rect)) boxes.push(o.rect);
  return { axis, at: hit.at, edge: EDGE_NAMES[axis][hit.edge], to: shown.join(", "), boxes };
}

// snapMove adjusts a move so that an edge or the middle of the rectangle lands on a
// line within threshold pixels, one axis at a time. It returns the move to make and
// the guides to draw, each naming the edge held, what it lines up with and the
// rectangles of the items on the line
export function snapMove(rect, dx, dy, targets, threshold) {
  const moved = moveRect(rect, dx, dy);
  const e = edges(moved);
  const sx = nearest(e.x, targets.x, threshold);
  const sy = nearest(e.y, targets.y, threshold);
  const guides = [];
  if (sx) guides.push(guideFor("x", sx, targets));
  if (sy) guides.push(guideFor("y", sy, targets));
  // A middle line can sit on a half pixel, and a move is whole pixels
  return { dx: Math.round(dx + (sx ? sx.d : 0)), dy: Math.round(dy + (sy ? sy.d : 0)), guides };
}

// toNative converts a distance on screen to the finished image's pixels, given how
// wide the preview is on screen and how wide the finished image is
export function toNative(screenPx, screenWidth, nativeWidth) {
  return screenWidth > 0 ? Math.round((screenPx * nativeWidth) / screenWidth) : 0;
}

// asPercent places a rectangle on the preview as percentages of the finished image,
// so it scales with the preview
export function asPercent(rect, canvas) {
  return {
    left: (rect.x / canvas.width) * 100,
    top: (rect.y / canvas.height) * 100,
    width: (rect.width / canvas.width) * 100,
    height: (rect.height / canvas.height) * 100,
  };
}

// nudgeFor is how far an arrow key moves an item, one pixel and ten with shift
export function nudgeFor(key, shift) {
  const step = shift ? 10 : 1;
  switch (key) {
    case "ArrowLeft":
      return { dx: -step, dy: 0 };
    case "ArrowRight":
      return { dx: step, dy: 0 };
    case "ArrowUp":
      return { dx: 0, dy: -step };
    case "ArrowDown":
      return { dx: 0, dy: step };
  }
  return null;
}

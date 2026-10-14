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

// hitTest picks the item under a point. The topmost wins, which is the last drawn.
// Asked again at a point inside the item already selected, it picks the next one
// beneath it, so a layer under a text box can be reached, and from the lowest it
// goes back to the top
export function hitTest(items, p, selected) {
  const under = selectable(items).filter((it) => contains(it.delivered, p));
  if (!under.length) return null;
  const at = under.findIndex((it) => refOf(it) === selected);
  if (at < 0) return under[under.length - 1];
  return under[at === 0 ? under.length - 1 : at - 1];
}

// pickAt is hitTest without the cycling, for the first press on an item
export function pickAt(items, p, selected) {
  const under = selectable(items).filter((it) => contains(it.delivered, p));
  if (!under.length) return null;
  const current = under.find((it) => refOf(it) === selected);
  return current || under[under.length - 1];
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

// snapTargets are the lines a moving item can snap to: the canvas's edges and middle
// and the edges and middle of every other item
export function snapTargets(items, canvas, except) {
  const whole = { x: 0, y: 0, width: canvas.width, height: canvas.height };
  const xs = new Set(edges(whole).x);
  const ys = new Set(edges(whole).y);
  for (const it of selectable(items)) {
    if (refOf(it) === except) continue;
    // A frame layer that fills the canvas has the canvas's own lines
    const e = edges(it.delivered);
    e.x.forEach((v) => xs.add(v));
    e.y.forEach((v) => ys.add(v));
  }
  return { x: [...xs].sort((a, b) => a - b), y: [...ys].sort((a, b) => a - b) };
}

function nearest(values, lines, threshold) {
  let best = null;
  for (const v of values) {
    for (const line of lines) {
      const d = line - v;
      if (Math.abs(d) <= threshold && (best === null || Math.abs(d) < Math.abs(best.d))) best = { d, at: line };
    }
  }
  return best;
}

// snapMove adjusts a move so that an edge or the middle of the rectangle lands on a
// line within threshold pixels, one axis at a time. It returns the move to make and
// the guides to draw
export function snapMove(rect, dx, dy, targets, threshold) {
  const moved = moveRect(rect, dx, dy);
  const e = edges(moved);
  const sx = nearest(e.x, targets.x, threshold);
  const sy = nearest(e.y, targets.y, threshold);
  const guides = [];
  if (sx) guides.push({ axis: "x", at: sx.at });
  if (sy) guides.push({ axis: "y", at: sy.at });
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

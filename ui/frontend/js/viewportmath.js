// The arithmetic of a viewport over a picture, as pure functions: the zoom stops the
// buttons step through, how far a turn of the wheel zooms, and what a zoom is called.
// A zoom is how many of the picture's own pixels fill one device pixel, so 1 is 100%,
// and 0 stands for fitting the picture to the room there is

export const STEPS = [0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4, 6, 8];
export const MIN_ZOOM = 0.1;
export const MAX_ZOOM = 8;

export const clampZoom = (z) => Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, z));

// stepZoom is the stop above from when dir is 1 and below it when dir is -1, or null
// when there is none. from is the zoom on screen, which is the fit's own scale when the
// picture is fitted
export function stepZoom(from, dir) {
  if (dir > 0) return STEPS.find((s) => s > from + 0.001) ?? null;
  return [...STEPS].reverse().find((s) => s < from - 0.001) ?? null;
}

// wheelZoom is the zoom after a turn of the wheel of deltaY. A turn up, which is a
// negative delta, zooms in. mode is the event's deltaMode, 1 for a wheel that reports
// lines, and pinch is a pinch on a trackpad, which reports smaller turns
export function wheelZoom(from, deltaY, { mode = 0, pinch = false } = {}) {
  const pixels = mode === 1 ? deltaY * 16 : deltaY;
  const rate = pinch ? 0.01 : 0.0015;
  return clampZoom(from * Math.exp(-pixels * rate));
}

// zoomLabel says a zoom to a person
export const zoomLabel = (z) => (z === 0 ? "Fit" : `${Math.round(z * 100)}%`);

// cycleZoom walks fit, 100% and 200%, and goes to 100% from anything else
export function cycleZoom(z) {
  if (z === 0) return 1;
  return z === 1 ? 2 : z === 2 ? 0 : 1;
}

// anchorShift is how far a picture has to be moved to keep the point of it that was under
// the anchor still after its size changed from before to after. Both are the picture's
// box on screen, and the point is the anchor's place in the old box as a share of it
export function anchorShift(before, after, anchor) {
  const fx = before.width ? (anchor.x - before.left) / before.width : 0.5;
  const fy = before.height ? (anchor.y - before.top) / before.height : 0.5;
  return { x: anchor.x - (after.left + fx * after.width), y: anchor.y - (after.top + fy * after.height) };
}

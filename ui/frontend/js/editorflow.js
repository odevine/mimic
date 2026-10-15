// Where a text box's text keeps out of a layer, as pure functions of what the app
// described and what the layout file says. The engine avoids the visible pixels of the
// layer a box names as its avoidLayer, and only for a card whose render draws that layer,
// which is what the app reports for each layer, so the region needs no more than that

const area = (r) => (r.width > 0 && r.height > 0 ? r.width * r.height : 0);

// intersect is the rectangle two rectangles share, or null when they share none
export function intersect(a, b) {
  const x = Math.max(a.x, b.x);
  const y = Math.max(a.y, b.y);
  const right = Math.min(a.x + a.width, b.x + b.width);
  const bottom = Math.min(a.y + a.height, b.y + b.height);
  const r = { x, y, width: right - x, height: bottom - y };
  return area(r) > 0 ? r : null;
}

// avoidance says what a text box's avoidLayer does for the card being drawn, as
// { state, layer, rect, note }, or null for a box that names no layer. rect is the part
// of the layer that lies in the box, in the finished image's pixels, and is set only
// while state is "active". box is the item, rect its place as it is now, flow the
// box's flow from the layout file, and items every item the layout draws
export function avoidance(box, rect, flow, items) {
  const id = flow && typeof flow.avoidLayer === "string" ? flow.avoidLayer : "";
  if (!id || !box || box.kind !== "text") return null;
  const layer = (items || []).find((it) => it.kind === "layer" && it.key === id);
  if (!layer) return { state: "missing", layer: id, rect: null, note: `Steps around ${id}, which is not in the stack` };
  if (!layer.drawn) return { state: "idle", layer: id, rect: null, note: `Steps around ${id}, which this card does not draw, so the text has the whole box` };
  const shared = rect && intersect(layer.delivered, rect);
  if (!shared) return { state: "clear", layer: id, rect: null, note: `Steps around ${id}, which does not reach this box` };
  return { state: "active", layer: id, rect: shared, note: `Steps around ${id}` };
}

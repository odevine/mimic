// The layers list as pure functions: what a row's icon is, where a dragged layer lands
// and what a row's menu offers

// ICON_OF names the glyph that stands for each kind of item in the list
const ICON_OF = { text: "text", symbol: "star", layer: "image", art: "select" };

export const kindIcon = (kind) => ICON_OF[kind] || "layers";

// reorderTarget says where a layer dropped at a height lands among the layers. centers
// are the heights of the middle of each layer's row, top of the list first, and from is
// the row being dragged. The result is the line to show, as the row it lies above or
// n for below the last, and the place in the stack the layer ends up in, where 0 is
// the bottom. It is null when the layer would stay where it is
export function reorderTarget(centers, y, from) {
  const n = centers.length;
  if (from < 0 || from >= n) return null;
  let slot = 0;
  while (slot < n && centers[slot] < y) slot++;
  const row = slot > from ? slot - 1 : slot;
  if (row === from) return { slot, to: null };
  return { slot, to: n - 1 - row };
}

// moveTarget is the place in the stack a layer goes to when it is raised, by 1, or
// lowered, by -1, or null when it is at that end already. at is the layer's place
// among n, where 0 is the bottom
export function moveTarget(at, n, by) {
  const to = at + by;
  return to < 0 || to >= n ? null : to;
}

// rowMenu lists what a row's menu offers, each { id, label }. A symbol is moved and
// resized and not copied or taken out, and a layer that cannot be edited offers none.
// A layer can also be moved a place, which is what dragging does for the keyboard
export function rowMenu(item, { editable, at, layers }) {
  if (!editable || item.kind === "symbol") return [];
  const out = [
    { id: "duplicate", label: "Duplicate" },
    { id: "delete", label: "Delete" },
  ];
  if (item.kind === "layer") {
    if (moveTarget(at, layers, 1) !== null) out.push({ id: "raise", label: "Move up" });
    if (moveTarget(at, layers, -1) !== null) out.push({ id: "lower", label: "Move down" });
  }
  return out;
}

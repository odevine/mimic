// Which items the preview leaves out and which layers it draws that the card would not, as
// pure functions. This is for looking at a template and is kept with nothing: the file is
// not changed by it. An item is named as its ref is, a kind and a key, such as "layer:frame",
// which is how the engine names what it hides and shows, and each is kept for the layout it
// is in, since two layouts may use the same key

// emptyVisibility is nothing hidden and nothing forced
export const emptyVisibility = () => ({ hidden: [], forced: [] });

const mark = (layout, ref) => `${layout}/${ref}`;
const has = (list, layout, ref) => list.includes(mark(layout, ref));
const without = (list, layout, ref) => list.filter((m) => m !== mark(layout, ref));
const withMark = (list, layout, ref) => (has(list, layout, ref) ? list : [...list, mark(layout, ref)]);

// the refs of a layout in a list
const refsOf = (list, layout) => list.filter((m) => m.startsWith(`${layout}/`)).map((m) => m.slice(layout.length + 1));

// requestLists are the names a render is sent: what to hide in the layout, and the layers to
// draw whatever their condition says
export function requestLists(vis, layout) {
  return { hide: refsOf(vis.hidden, layout), show: refsOf(vis.forced, layout).filter((r) => r.startsWith("layer:")) };
}

// hiddenRefs are the refs hidden in a layout, which cannot be picked on the card
export const hiddenRefs = (vis, layout) => new Set(refsOf(vis.hidden, layout));

// eyeState says what an item's eye shows and does. on is whether the item shows in the
// preview. A layer the card does not draw can be forced to, which is what its eye does, and
// anything else the card does not draw has nothing to show. kind is how it is marked: shown,
// hidden, forced, idle for a layer that is not drawn, or blocked for another that is not
export function eyeState(vis, layout, item) {
  const ref = `${item.kind}:${item.key}`;
  if (has(vis.forced, layout, ref)) return { kind: "forced", on: true, disabled: false, tip: "Show/Hide", label: `Stop drawing ${item.key} when its condition does not hold` };
  if (has(vis.hidden, layout, ref)) return { kind: "hidden", on: false, disabled: false, tip: "Show/Hide", label: `Show ${item.key} in the preview` };
  if (item.drawn) return { kind: "shown", on: true, disabled: false, tip: "Show/Hide", label: `Hide ${item.key} in the preview` };
  if (item.kind === "layer") return { kind: "idle", on: false, disabled: false, tip: "Show/Hide", label: `Draw ${item.key} whatever its condition says` };
  return { kind: "blocked", on: false, disabled: true, tip: "Not drawn for this card", label: `${item.key} is not drawn for this card` };
}

// toggleEye is what pressing an item's eye does: it hides a shown item, shows a hidden one,
// forces a layer the card does not draw and lets go of one that was forced. It returns the
// new state and whether a layer was forced or let go, which changes what the card draws and
// so needs the layout described again, and not only drawn again
export function toggleEye(vis, layout, item) {
  const ref = `${item.kind}:${item.key}`;
  const state = eyeState(vis, layout, item);
  switch (state.kind) {
    case "forced":
      return { vis: { ...vis, forced: without(vis.forced, layout, ref) }, described: true };
    case "hidden":
      return { vis: { ...vis, hidden: without(vis.hidden, layout, ref) }, described: false };
    case "shown":
      return { vis: { ...vis, hidden: withMark(vis.hidden, layout, ref) }, described: false };
    case "idle":
      return { vis: { ...vis, forced: withMark(vis.forced, layout, ref) }, described: true };
  }
  return { vis, described: false };
}

// isEmpty says nothing is hidden or forced
export const isEmpty = (vis) => vis.hidden.length === 0 && vis.forced.length === 0;

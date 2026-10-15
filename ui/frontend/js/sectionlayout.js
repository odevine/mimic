// How the editor's right column divides its height among its sections, as pure
// functions: which are open, how much each open one has, where a splitter goes and what
// a drag changes. The column's own code reads and writes the elements

// The sections of the right column, top to bottom
export const SECTIONS = ["layers", "selection", "problems"];

// DEFAULTS is how the right column starts: the layers and the selection are open, with
// the layers given the most room, and the problems are collapsed to their heading. A
// weight says how the height is shared among the open sections
export const DEFAULTS = {
  collapsed: [false, false, true],
  weights: [6, 4, 2],
};

// The left column's sections, the search, which starts collapsed, and the test card, which
// holds the properties of the card
export const LEFT_SECTIONS = ["search", "card"];
export const LEFT_DEFAULTS = {
  collapsed: [true, false],
  weights: [6, 6],
};

// MIN_HEIGHT is the least an open section may be dragged down to, its heading and a
// little of what it holds
export const MIN_HEIGHT = 96;

// restore reads the state kept by pack, which is a list of one number per section for
// whether it is collapsed and then one for how much room it has. Anything else, or a
// weight that is not a number above zero, is the defaults, which are the right
// column's unless others are given
export function restore(saved, defaults = DEFAULTS) {
  const n = defaults.collapsed.length;
  const state = { collapsed: [...defaults.collapsed], weights: [...defaults.weights] };
  if (!Array.isArray(saved) || saved.length !== 2 * n || !saved.every(Number.isFinite)) return state;
  for (let i = 0; i < n; i++) {
    state.collapsed[i] = saved[i] === 1;
    if (saved[n + i] > 0) state.weights[i] = saved[n + i];
  }
  return state;
}

// pack is the state as the list restore reads
export function pack(state) {
  return [...state.collapsed.map((c) => (c ? 1 : 0)), ...state.weights.map((w) => Math.round(w))];
}

// nextOpen is the open section below section i, or -1 when it is collapsed or the
// last one open
export function nextOpen(collapsed, i) {
  if (collapsed[i]) return -1;
  for (let j = i + 1; j < collapsed.length; j++) if (!collapsed[j]) return j;
  return -1;
}

// restFrom is the first of the collapsed sections that end the column, which is where
// the free space goes above so they rest against what is below, or -1 when the last
// section is open
export function restFrom(collapsed) {
  let at = -1;
  for (let i = collapsed.length - 1; i >= 0 && collapsed[i]; i--) at = i;
  return at;
}

// resizePair moves the line between open section i and the open section j below it by
// dy, which grows i and shrinks j, and keeps each at least min. heights are the sections'
// heights now, and the result is the new height of each of the two
export function resizePair(heights, i, j, dy, min = MIN_HEIGHT) {
  const hi = heights[i];
  const hj = heights[j];
  const room = Math.max(0, hi + hj - 2 * min);
  const lowest = Math.min(min - hi, 0);
  const highest = Math.max(hj - min, 0);
  const d = Math.min(Math.max(dy, lowest), highest);
  // A pair with no room beyond the two minimums does not move
  return room === 0 && hi + hj < 2 * min ? { hi, hj } : { hi: hi + d, hj: hj - d };
}

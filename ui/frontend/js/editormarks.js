// The elements an author has locked, and the ones a drag snaps to. A mark names an
// element by its layout and its place in the layout, "layout/kind:key", since two
// layouts can each have a title

export const markKey = (layout, ref) => `${layout}/${ref}`;

// refsFor is the elements of one layout that are in a list of marks, as a set of
// the refs the page selects by
export function refsFor(list, layout) {
  const prefix = `${layout}/`;
  return new Set((list || []).filter((m) => m.startsWith(prefix)).map((m) => m.slice(prefix.length)));
}

export const isMarked = (list, layout, ref) => (list || []).includes(markKey(layout, ref));

// withMark is the list with an element in it or out of it
export function withMark(list, layout, ref, on) {
  const key = markKey(layout, ref);
  const rest = (list || []).filter((m) => m !== key);
  return on ? [...rest, key] : rest;
}

// shiftArt keeps the marks of a layout's art windows on the windows they were made
// for, when one is added at a place in the list or taken out of it. Windows are
// named by their place
export function shiftArt(list, layout, at, delta) {
  const prefix = `${layout}/art:`;
  const out = [];
  for (const m of list || []) {
    if (!m.startsWith(prefix)) {
      out.push(m);
      continue;
    }
    const n = Number(m.slice(prefix.length));
    if (!Number.isInteger(n)) {
      out.push(m);
    } else if (delta < 0 && n === at) {
      continue;
    } else if (n >= at && delta > 0) {
      out.push(`${prefix}${n + 1}`);
    } else if (n > at && delta < 0) {
      out.push(`${prefix}${n - 1}`);
    } else {
      out.push(m);
    }
  }
  return out;
}

// afterOps moves the marks of art windows to follow the edits that add, copy and
// delete them, and leaves the rest as they are
export function afterOps(marks, ops) {
  let { locked, snap } = marks;
  for (const op of ops || []) {
    if (op.kind !== "art" || !op.layout) continue;
    let at;
    let delta;
    if (op.op === "add") [at, delta] = [Number(op.key), 1];
    else if (op.op === "delete") [at, delta] = [Number(op.key), -1];
    else if (op.op === "duplicate") [at, delta] = [Number(op.key) + 1, 1];
    else continue;
    if (!Number.isInteger(at)) continue;
    locked = shiftArt(locked, op.layout, at, delta);
    snap = shiftArt(snap, op.layout, at, delta);
  }
  return { locked, snap };
}

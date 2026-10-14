// The editor's undo and redo. Each entry holds the operations that made a change
// and the ones the app says undo it. A run of the same kind of change to one thing,
// such as an arrow key held down, is one entry, so one undo takes back the whole run

// Changes to one item that follow each other within this many milliseconds are one
// entry
const MERGE_MS = 800;

const sameTarget = (a, b) => a.layout === b.layout && a.kind === b.kind && a.key === b.key;

// merged is two entries as one, or null when they are not the same kind of change to
// the same thing. Moves add up, and for a resize, a field or a file the latest value
// stands with the first undo
function merged(prev, next) {
  if (prev.ops.length !== 1 || next.ops.length !== 1 || prev.inverse.length !== 1 || next.inverse.length !== 1) return null;
  const [a] = prev.ops;
  const [b] = next.ops;
  // A whole file written again is the file as it is now, with the first undo
  if (a.op === "replace" && b.op === "replace" && a.file === b.file) return { ...next, ops: [b], inverse: prev.inverse };
  const field = (op) => op.op === "set" || op.op === "unset";
  if (field(a) && field(b) && sameTarget(a, b) && JSON.stringify(a.path) === JSON.stringify(b.path)) {
    return { ...next, ops: [b], inverse: prev.inverse };
  }
  if (a.op !== b.op || !sameTarget(a, b)) return null;
  if (a.op === "move") {
    const op = { ...a, dx: (a.dx || 0) + (b.dx || 0), dy: (a.dy || 0) + (b.dy || 0) };
    const inverse = { ...prev.inverse[0], dx: -op.dx, dy: -op.dy };
    return { ...next, ops: [op], inverse: [inverse] };
  }
  if (a.op === "resize") return { ...next, ops: [b], inverse: prev.inverse };
  return null;
}

// createHistory keeps up to limit entries. record takes an entry, { ops, inverse,
// label, merge }, where merge marks a change that may join the one before it. undo
// and redo return the entry to apply the other way, or null when there is none
export function createHistory({ limit = 100, now = () => Date.now() } = {}) {
  let undone = [];
  let done = [];
  let last = { at: 0 };

  return {
    record(entry) {
      undone = [];
      const at = now();
      const prev = done[done.length - 1];
      if (entry.merge && prev && prev.merge && at - last.at <= MERGE_MS) {
        const both = merged(prev, entry);
        if (both) {
          done[done.length - 1] = { ...both, merge: entry.merge };
          last = { at };
          return;
        }
      }
      done.push({ ...entry });
      if (done.length > limit) done.shift();
      last = { at };
    },
    // undo takes the latest entry back. The caller applies its inverse
    undo() {
      const entry = done.pop();
      if (!entry) return null;
      undone.push(entry);
      last = { at: 0 };
      return entry;
    },
    // redo makes the entry last undone again. The caller applies its ops
    redo() {
      const entry = undone.pop();
      if (!entry) return null;
      done.push(entry);
      last = { at: 0 };
      return entry;
    },
    get canUndo() {
      return done.length > 0;
    },
    get canRedo() {
      return undone.length > 0;
    },
    get nextUndo() {
      return done.length ? done[done.length - 1].label : "";
    },
    get nextRedo() {
      return undone.length ? undone[undone.length - 1].label : "";
    },
    clear() {
      done = [];
      undone = [];
      last = { at: 0 };
    },
  };
}

import test from "node:test";
import assert from "node:assert/strict";

import { createHistory } from "../js/editorhistory.js";

const target = { layout: "normal", kind: "text", key: "title" };
const move = (dx, dy, extra = {}) => ({ op: "move", ...target, dx, dy, ...extra });
const entry = (dx, dy, merge, label = "Move title") => ({ ops: [move(dx, dy)], inverse: [move(-dx, -dy)], label, merge });

// A clock a test moves by hand
function clock() {
  let t = 1000;
  return { now: () => t, tick: (ms) => (t += ms) };
}

test("undo hands back the latest entry, and redo hands it back again", () => {
  const h = createHistory();
  h.record(entry(1, 0));
  h.record(entry(0, 5, "", "Move art"));
  assert.equal(h.canUndo, true);
  assert.equal(h.canRedo, false);
  assert.equal(h.nextUndo, "Move art");
  const undone = h.undo();
  assert.equal(undone.label, "Move art");
  assert.equal(h.nextRedo, "Move art");
  assert.equal(h.canRedo, true);
  assert.equal(h.redo().label, "Move art");
  assert.equal(h.canRedo, false);
  assert.equal(h.undo().label, "Move art");
  assert.equal(h.undo().label, "Move title");
  assert.equal(h.undo(), null);
  assert.equal(h.canUndo, false);
});

test("a new change forgets what was undone", () => {
  const h = createHistory();
  h.record(entry(1, 0));
  h.record(entry(2, 0));
  h.undo();
  h.record(entry(3, 0));
  assert.equal(h.canRedo, false);
  assert.equal(h.redo(), null);
});

test("a run of nudges to one item is one entry that undoes the whole run", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  for (const [dx, dy] of [[1, 0], [1, 0], [0, -1], [10, 0]]) {
    h.record(entry(dx, dy, "nudge"));
    c.tick(100);
  }
  const e = h.undo();
  assert.deepEqual(e.ops[0], move(12, -1));
  assert.deepEqual(e.inverse[0], move(-12, 1));
  assert.equal(h.undo(), null, "one entry held the run");
});

test("a pause, another item or another kind of change starts a new entry", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  h.record(entry(1, 0, "nudge"));
  c.tick(5000);
  h.record(entry(1, 0, "nudge"));
  h.record({ ops: [move(1, 0, { key: "pt" })], inverse: [move(-1, 0, { key: "pt" })], label: "Move pt", merge: "nudge" });
  h.record(entry(1, 0, ""));
  h.record(entry(1, 0, "nudge"));
  let count = 0;
  while (h.undo()) count++;
  assert.equal(count, 5);
});

test("a change not marked to merge never joins the one before", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  h.record(entry(1, 0, "nudge"));
  h.record(entry(1, 0, ""));
  assert.equal(h.undo().ops[0].dx, 1);
  assert.equal(h.undo().ops[0].dx, 1);
});

test("resizes merge to the latest size with the first undo", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  const resize = (w) => ({ op: "resize", ...target, rect: { x: 0, y: 0, width: w, height: 10 } });
  h.record({ ops: [resize(20)], inverse: [resize(10)], label: "Resize", merge: "resize" });
  c.tick(50);
  h.record({ ops: [resize(30)], inverse: [resize(20)], label: "Resize", merge: "resize" });
  const e = h.undo();
  assert.equal(e.ops[0].rect.width, 30);
  assert.equal(e.inverse[0].rect.width, 10);
  assert.equal(h.undo(), null);
});

test("entries of more than one operation are never merged", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  const batch = { ops: [move(1, 0), move(2, 0)], inverse: [move(-2, 0), move(-1, 0)], label: "Two", merge: "nudge" };
  h.record(batch);
  h.record(batch);
  assert.ok(h.undo() && h.undo() && !h.undo());
});

test("the history keeps only the latest entries", () => {
  const h = createHistory({ limit: 3 });
  for (let i = 1; i <= 5; i++) h.record(entry(i, 0, "", `Move ${i}`));
  assert.deepEqual([h.undo().label, h.undo().label, h.undo().label, h.undo()], ["Move 5", "Move 4", "Move 3", null]);
});

test("a change after an undo is not merged into the entry undone", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  h.record(entry(1, 0, "nudge"));
  h.undo();
  h.record(entry(2, 0, "nudge"));
  assert.equal(h.undo().ops[0].dx, 2);
  assert.equal(h.undo(), null);
});

test("clearing empties both ways", () => {
  const h = createHistory();
  h.record(entry(1, 0));
  h.undo();
  h.clear();
  assert.equal(h.canUndo || h.canRedo, false);
  assert.equal(h.nextUndo + h.nextRedo, "");
});

test("a field set again in a run keeps the latest value and the first undo", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  const set = (value, extra = {}) => ({ op: "set", ...target, path: ["text", "fontSize"], value, ...extra });
  h.record({ ops: [set(20)], inverse: [set(12)], label: "Change size", merge: "lever" });
  c.tick(100);
  h.record({ ops: [set(21)], inverse: [set(20)], label: "Change size", merge: "lever" });
  c.tick(100);
  h.record({ ops: [set(22)], inverse: [set(21)], label: "Change size", merge: "lever" });
  const e = h.undo();
  assert.equal(e.ops[0].value, 22);
  assert.equal(e.inverse[0].value, 12);
  assert.equal(h.undo(), null);
});

test("different fields, or a set and an unset of other fields, stay separate", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  const set = (path, value) => ({ op: "set", ...target, path, value });
  h.record({ ops: [set(["text", "fontSize"], 20)], inverse: [set(["text", "fontSize"], 12)], label: "a", merge: "lever" });
  h.record({ ops: [set(["text", "color"], "#000000")], inverse: [{ op: "unset", ...target, path: ["text", "color"] }], label: "b", merge: "lever" });
  assert.ok(h.undo() && h.undo() && !h.undo());
});

test("a file written again in a run is one entry that goes back to the first text", () => {
  const c = clock();
  const h = createHistory({ now: c.now });
  const write = (content) => ({ op: "replace", file: "layouts/normal.json", content });
  h.record({ ops: [write("{\"a\":1}")], inverse: [write("{}")], label: "Edit", merge: "json" });
  c.tick(200);
  h.record({ ops: [write("{\"a\":12}")], inverse: [write("{\"a\":1}")], label: "Edit", merge: "json" });
  const e = h.undo();
  assert.equal(e.ops[0].content, '{"a":12}');
  assert.equal(e.inverse[0].content, "{}");
  assert.equal(h.undo(), null);
  // Another file is another entry
  h.record({ ops: [write("1")], inverse: [write("0")], label: "Edit", merge: "json" });
  h.record({ ops: [{ ...write("2"), file: "template.json" }], inverse: [{ ...write("0"), file: "template.json" }], label: "Edit", merge: "json" });
  assert.ok(h.undo() && h.undo() && !h.undo());
});

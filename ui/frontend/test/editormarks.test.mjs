import test from "node:test";
import assert from "node:assert/strict";

import { markKey, refsFor, isMarked, withMark, shiftArt, afterOps } from "../js/editormarks.js";

test("a mark names a layout and an element in it", () => {
  assert.equal(markKey("normal", "text:title"), "normal/text:title");
  const list = ["normal/text:title", "normal/layer:frame", "saga/text:title"];
  assert.deepEqual([...refsFor(list, "normal")].sort(), ["layer:frame", "text:title"]);
  assert.deepEqual([...refsFor(list, "saga")], ["text:title"]);
  assert.equal(refsFor(null, "normal").size, 0);
  assert.equal(refsFor(["normalx/text:a"], "normal").size, 0, "a layout whose name starts the same is another");
  assert.ok(isMarked(list, "saga", "text:title"));
  assert.ok(!isMarked(list, "saga", "layer:frame"));
});

test("a mark is added once and taken away", () => {
  let list = withMark([], "normal", "text:title", true);
  list = withMark(list, "normal", "text:title", true);
  assert.deepEqual(list, ["normal/text:title"]);
  list = withMark(list, "normal", "layer:frame", true);
  assert.deepEqual(withMark(list, "normal", "text:title", false), ["normal/layer:frame"]);
  assert.deepEqual(withMark(null, "normal", "x:y", false), []);
});

test("the marks of art windows follow the windows when one is added or taken out", () => {
  const list = ["normal/art:0", "normal/art:1", "normal/art:2", "normal/text:title", "saga/art:1"];
  assert.deepEqual(shiftArt(list, "normal", 1, 1), ["normal/art:0", "normal/art:2", "normal/art:3", "normal/text:title", "saga/art:1"]);
  assert.deepEqual(shiftArt(list, "normal", 1, -1), ["normal/art:0", "normal/art:1", "normal/text:title", "saga/art:1"]);
  assert.deepEqual(shiftArt(list, "normal", 3, 1), list, "a window added at the end moves nothing");
  assert.deepEqual(shiftArt(["normal/art:x"], "normal", 0, 1), ["normal/art:x"]);
});

test("the edits that add, copy and delete art windows move the marks, and no others do", () => {
  const marks = { locked: ["normal/art:1"], snap: ["normal/art:0", "normal/art:1"] };
  assert.deepEqual(afterOps(marks, [{ op: "add", layout: "normal", kind: "art", key: "0" }]), { locked: ["normal/art:2"], snap: ["normal/art:1", "normal/art:2"] });
  assert.deepEqual(afterOps(marks, [{ op: "duplicate", layout: "normal", kind: "art", key: "0" }]), { locked: ["normal/art:2"], snap: ["normal/art:0", "normal/art:2"] });
  assert.deepEqual(afterOps(marks, [{ op: "delete", layout: "normal", kind: "art", key: "0" }]), { locked: ["normal/art:0"], snap: ["normal/art:0"] });
  // An undo is the same edits the other way, and it brings the marks back in step
  const there = afterOps(marks, [{ op: "add", layout: "normal", kind: "art", key: "1" }]);
  assert.deepEqual(afterOps(there, [{ op: "delete", layout: "normal", kind: "art", key: "1" }]), marks);
  assert.deepEqual(afterOps(marks, [{ op: "add", layout: "normal", kind: "text", key: "x" }, { op: "move", layout: "normal", kind: "art", key: "0" }, { op: "put", file: "a.png" }]), marks);
  assert.deepEqual(afterOps(marks, null), marks);
});

import test from "node:test";
import assert from "node:assert/strict";

import { kindIcon, reorderTarget, moveTarget, rowMenu } from "../js/editorlayers.js";

test("each kind of item has its own icon", () => {
  assert.equal(kindIcon("text"), "text");
  assert.equal(kindIcon("symbol"), "star");
  assert.equal(kindIcon("layer"), "image");
  assert.equal(kindIcon("art"), "select");
  assert.equal(kindIcon("something else"), "layers");
});

// Four layers, the top of the list first, 20 pixels apart
const centers = [10, 30, 50, 70];

test("a layer dropped between two rows lands there, counted from the bottom of the stack", () => {
  // Dragging the top row (0) to between the second and third rows puts it second from the top
  assert.deepEqual(reorderTarget(centers, 40, 0), { slot: 2, to: 2 });
  // The bottom row (3) to the very top
  assert.deepEqual(reorderTarget(centers, 0, 3), { slot: 0, to: 3 });
  // The second row (1) to the very bottom
  assert.deepEqual(reorderTarget(centers, 200, 1), { slot: 4, to: 0 });
  // The third row (2) up one, above the second
  assert.deepEqual(reorderTarget(centers, 25, 2), { slot: 1, to: 2 });
});

test("a layer dropped where it already is does not move", () => {
  assert.deepEqual(reorderTarget(centers, 12, 0), { slot: 1, to: null });
  assert.deepEqual(reorderTarget(centers, 0, 0), { slot: 0, to: null });
  assert.deepEqual(reorderTarget(centers, 28, 1), { slot: 1, to: null });
  assert.deepEqual(reorderTarget(centers, 45, 1), { slot: 2, to: null });
  assert.deepEqual(reorderTarget(centers, 100, 3), { slot: 4, to: null });
});

test("a drag that is not of a row of the list lands nowhere", () => {
  assert.equal(reorderTarget(centers, 10, -1), null);
  assert.equal(reorderTarget(centers, 10, 4), null);
  assert.equal(reorderTarget([], 10, 0), null);
});

test("a layer is raised or lowered a place until it reaches an end", () => {
  assert.equal(moveTarget(1, 4, 1), 2);
  assert.equal(moveTarget(1, 4, -1), 0);
  assert.equal(moveTarget(3, 4, 1), null);
  assert.equal(moveTarget(0, 4, -1), null);
});

test("a row's menu offers copying and deleting, and a layer can be moved too", () => {
  const ctx = { editable: true, at: 1, layers: 3 };
  assert.deepEqual(rowMenu({ kind: "text" }, ctx).map((m) => m.id), ["duplicate", "delete"]);
  assert.deepEqual(rowMenu({ kind: "art" }, ctx).map((m) => m.id), ["duplicate", "delete"]);
  assert.deepEqual(rowMenu({ kind: "layer" }, ctx).map((m) => m.id), ["duplicate", "delete", "raise", "lower"]);
  assert.deepEqual(rowMenu({ kind: "layer" }, { ...ctx, at: 2 }).map((m) => m.id), ["duplicate", "delete", "lower"]);
  assert.deepEqual(rowMenu({ kind: "layer" }, { ...ctx, at: 0 }).map((m) => m.id), ["duplicate", "delete", "raise"]);
  assert.deepEqual(rowMenu({ kind: "layer" }, { ...ctx, at: 0, layers: 1 }).map((m) => m.id), ["duplicate", "delete"]);
});

test("a symbol and a layout that cannot be edited have no menu", () => {
  assert.deepEqual(rowMenu({ kind: "symbol" }, { editable: true, at: 0, layers: 1 }), []);
  assert.deepEqual(rowMenu({ kind: "text" }, { editable: false, at: 0, layers: 1 }), []);
});

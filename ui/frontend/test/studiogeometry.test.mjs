import test from "node:test";
import assert from "node:assert/strict";

import { HANDLES, visibleHandles, refOf, contains, selectable, hitTest, pickAt, resizeRect, moveRect, snapTargets, snapMove, toNative, asPercent, nudgeFor } from "../js/studiogeometry.js";

const item = (kind, key, x, y, width, height, drawn = true) => ({ kind, key, drawn, delivered: { x, y, width, height } });

const items = [
  item("layer", "background", 0, 0, 100, 140),
  item("layer", "frame", 10, 10, 80, 120),
  item("art", "0", 15, 20, 70, 50),
  item("text", "title", 20, 12, 50, 8),
  item("text", "pt", 60, 120, 20, 10, false),
];

test("a rectangle holds the points inside it and not its far edges", () => {
  const r = { x: 10, y: 20, width: 5, height: 4 };
  assert.equal(contains(r, { x: 10, y: 20 }), true);
  assert.equal(contains(r, { x: 14, y: 23 }), true);
  assert.equal(contains(r, { x: 15, y: 23 }), false);
  assert.equal(contains(r, { x: 14, y: 24 }), false);
  assert.equal(contains(r, { x: 9, y: 20 }), false);
});

test("only what is drawn and has room can be picked", () => {
  assert.deepEqual(selectable(items).map(refOf), ["layer:background", "layer:frame", "art:0", "text:title"]);
  assert.deepEqual(selectable([item("layer", "empty", 0, 0, 0, 5), item("layer", "gone", 0, 0, 5, 5, false)]), []);
  assert.deepEqual(selectable(null), []);
});

test("the topmost item under a point is picked", () => {
  assert.equal(refOf(hitTest(items, { x: 25, y: 14 }, null)), "text:title");
  assert.equal(refOf(hitTest(items, { x: 25, y: 40 }, null)), "art:0");
  assert.equal(refOf(hitTest(items, { x: 12, y: 100 }, null)), "layer:frame");
  assert.equal(refOf(hitTest(items, { x: 2, y: 2 }, null)), "layer:background");
  assert.equal(hitTest(items, { x: 200, y: 2 }, null), null);
  // An item that is not drawn is not there to hit, whatever its rectangle says
  assert.equal(refOf(hitTest(items, { x: 65, y: 125 }, null)), "layer:frame");
});

test("asking again inside the selected item reaches the one beneath, and wraps", () => {
  const p = { x: 25, y: 14 };
  const order = [];
  let selected = null;
  for (let i = 0; i < 5; i++) {
    selected = refOf(hitTest(items, p, selected));
    order.push(selected);
  }
  // The art starts below the point, so it is not among the items there
  assert.deepEqual(order, ["text:title", "layer:frame", "layer:background", "text:title", "layer:frame"]);
});

test("a point outside the selected item picks the topmost there", () => {
  assert.equal(refOf(hitTest(items, { x: 2, y: 2 }, "text:title")), "layer:background");
});

test("a first press keeps the selected item when it is under the point, and otherwise picks the topmost", () => {
  assert.equal(refOf(pickAt(items, { x: 25, y: 14 }, "layer:frame")), "layer:frame");
  assert.equal(refOf(pickAt(items, { x: 25, y: 14 }, "art:0")), "text:title", "an item that is not under the point is not kept");
  assert.equal(refOf(pickAt(items, { x: 25, y: 14 }, "layer:gone")), "text:title");
  assert.equal(pickAt(items, { x: 500, y: 14 }, null), null);
});

test("each handle moves the sides it holds", () => {
  const r = { x: 10, y: 10, width: 40, height: 20 };
  assert.deepEqual(resizeRect(r, "se", 5, 3), { x: 10, y: 10, width: 45, height: 23 });
  assert.deepEqual(resizeRect(r, "nw", 5, 3), { x: 15, y: 13, width: 35, height: 17 });
  assert.deepEqual(resizeRect(r, "e", 5, 99), { x: 10, y: 10, width: 45, height: 20 });
  assert.deepEqual(resizeRect(r, "n", 99, -4), { x: 10, y: 6, width: 40, height: 24 });
  assert.deepEqual(resizeRect(r, "sw", -6, 2), { x: 4, y: 10, width: 46, height: 22 });
  assert.deepEqual(resizeRect(r, "ne", 2, -2), { x: 10, y: 8, width: 42, height: 22 });
  assert.equal(Object.keys(HANDLES).length, 8);
});

test("a rectangle cannot be dragged smaller than the least, or inside out", () => {
  const r = { x: 10, y: 10, width: 40, height: 20 };
  assert.deepEqual(resizeRect(r, "e", -100, 0), { x: 10, y: 10, width: 1, height: 20 });
  assert.deepEqual(resizeRect(r, "w", 100, 0), { x: 49, y: 10, width: 1, height: 20 });
  assert.deepEqual(resizeRect(r, "s", 0, -100, 4), { x: 10, y: 10, width: 40, height: 4 });
  assert.deepEqual(resizeRect(r, "n", 0, 100, 4), { x: 10, y: 26, width: 40, height: 4 });
});

test("moving a rectangle keeps its size", () => {
  assert.deepEqual(moveRect({ x: 1, y: 2, width: 3, height: 4 }, 10, -5), { x: 11, y: -3, width: 3, height: 4 });
});

test("the lines to snap to are the canvas's and every other item's", () => {
  const canvas = { width: 100, height: 140 };
  const t = snapTargets(items, canvas, "text:title");
  // The canvas's edges and middle
  for (const v of [0, 50, 100]) assert.ok(t.x.includes(v), `x ${v}`);
  for (const v of [0, 70, 140]) assert.ok(t.y.includes(v), `y ${v}`);
  // The art's middle, which is 50 across and 45 down, and the title's own right edge,
  // which is a line only while another item moves
  assert.ok(t.y.includes(45));
  assert.ok(!t.x.includes(20), "the title's left edge counts for itself");
  assert.ok(snapTargets(items, canvas, "art:0").x.includes(20), "the title's left edge counts for another item");
  assert.deepEqual(t.x, [...t.x].sort((a, b) => a - b));
  assert.equal(new Set(t.x).size, t.x.length, "no line twice");
});

test("an edge or the middle within the threshold lands on a line, one axis at a time", () => {
  const targets = { x: [0, 50, 100], y: [0, 70, 140] };
  const r = { x: 20, y: 30, width: 30, height: 10 };
  // The right edge would be at 52 and snaps to 50, and the top at 31 is too far from 0
  assert.deepEqual(snapMove(r, 2, 1, targets, 3), { dx: 0, dy: 1, guides: [{ axis: "x", at: 50 }] });
  // The middle would be at 68 and snaps to 70, and the x edges at 60 and 70 snap to nothing
  assert.deepEqual(snapMove({ x: 20, y: 60, width: 10, height: 10 }, 40, 3, targets, 3), { dx: 40, dy: 5, guides: [{ axis: "y", at: 70 }] });
  // Out of reach on both axes
  assert.deepEqual(snapMove(r, 11, 0, targets, 3).guides, []);
  assert.equal(snapMove(r, 2, 0, targets, 0).dx, 2, "a threshold of nothing snaps to nothing");
});

test("snapping prefers the nearest line", () => {
  const targets = { x: [48, 52], y: [] };
  const r = { x: 20, y: 0, width: 30, height: 10 };
  // The right edge at 51 is one from 52 and three from 48
  assert.equal(snapMove(r, 1, 0, targets, 5).dx, 2);
});

test("a distance on screen converts to the finished image's pixels", () => {
  assert.equal(toNative(100, 400, 2000), 500);
  assert.equal(toNative(-3, 400, 2000), -15);
  assert.equal(toNative(1, 400, 1000), 3);
  assert.equal(toNative(10, 0, 1000), 0);
});

test("a rectangle is placed on the preview as percentages of the canvas", () => {
  assert.deepEqual(asPercent({ x: 10, y: 20, width: 50, height: 40 }, { width: 100, height: 200 }), { left: 10, top: 10, width: 50, height: 20 });
});

test("arrow keys nudge by one pixel, and by ten with shift", () => {
  assert.deepEqual(nudgeFor("ArrowLeft", false), { dx: -1, dy: 0 });
  assert.deepEqual(nudgeFor("ArrowDown", true), { dx: 0, dy: 10 });
  assert.deepEqual(nudgeFor("ArrowUp", false), { dx: 0, dy: -1 });
  assert.deepEqual(nudgeFor("ArrowRight", true), { dx: 10, dy: 0 });
  assert.equal(nudgeFor("a", false), null);
});

test("a small box shows its corners and no handle that would sit on its middle", () => {
  const all = Object.keys(HANDLES).sort();
  assert.deepEqual([...visibleHandles(300, 200)].sort(), all);
  assert.deepEqual([...visibleHandles(240, 32)].sort(), ["n", "ne", "nw", "s", "se", "sw"].sort(), "a wide, short box keeps the top and bottom");
  assert.deepEqual([...visibleHandles(32, 240)].sort(), ["e", "ne", "nw", "se", "sw", "w"].sort(), "a tall, narrow box keeps the sides");
  assert.deepEqual([...visibleHandles(240, 17)].sort(), ["ne", "nw", "se", "sw"], "a box too short for the top and bottom shows corners only");
  assert.deepEqual([...visibleHandles(10, 10)].sort(), ["ne", "nw", "se", "sw"]);
});

test("a snap to a line on a half pixel still moves by whole pixels", () => {
  // The middle of a 499 wide canvas is 249.5, and the box's middle is at 240
  const targets = { x: [249.5], y: [] };
  const r = { x: 230, y: 0, width: 20, height: 10 };
  const got = snapMove(r, 0, 0, targets, 12);
  assert.ok(Number.isInteger(got.dx) && Number.isInteger(got.dy), `moved by ${got.dx}, ${got.dy}`);
  assert.deepEqual(got.guides, [{ axis: "x", at: 249.5 }]);
});

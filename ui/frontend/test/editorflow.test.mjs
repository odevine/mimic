import test from "node:test";
import assert from "node:assert/strict";

import { intersect, avoidance } from "../js/editorflow.js";

const r = (x, y, width, height) => ({ x, y, width, height });
const layer = (key, rect, drawn = true) => ({ kind: "layer", key, drawn, delivered: drawn ? rect : r(0, 0, 0, 0) });
const oracle = { kind: "text", key: "oracle", drawn: true };

test("two rectangles share the part where they overlap, and nothing when they only touch", () => {
  assert.deepEqual(intersect(r(0, 0, 10, 10), r(5, 5, 10, 10)), r(5, 5, 5, 5));
  assert.deepEqual(intersect(r(0, 0, 10, 10), r(2, 3, 4, 2)), r(2, 3, 4, 2));
  assert.equal(intersect(r(0, 0, 10, 10), r(10, 0, 5, 5)), null);
  assert.equal(intersect(r(0, 0, 10, 10), r(20, 20, 5, 5)), null);
  assert.equal(intersect(r(0, 0, 10, 10), r(1, 1, 0, 5)), null);
});

test("a box that names no layer avoids nothing", () => {
  assert.equal(avoidance(oracle, r(0, 0, 100, 100), {}, []), null);
  assert.equal(avoidance(oracle, r(0, 0, 100, 100), undefined, []), null);
  assert.equal(avoidance(oracle, r(0, 0, 100, 100), { avoidLayer: "" }, []), null);
  // Only a text box flows
  assert.equal(avoidance({ kind: "art", key: "0" }, r(0, 0, 10, 10), { avoidLayer: "pt" }, [layer("pt", r(0, 0, 5, 5))]), null);
});

test("the region is the part of the layer that lies in the box", () => {
  const items = [layer("frame", r(0, 0, 1000, 1400)), layer("pt", r(700, 1200, 200, 100))];
  const got = avoidance(oracle, r(100, 900, 700, 350), { avoidLayer: "pt" }, items);
  assert.deepEqual(got, { state: "active", layer: "pt", rect: r(700, 1200, 100, 50), note: "Steps around pt" });
});

test("the box as it is dragged is the one the region is cut to", () => {
  const items = [layer("pt", r(700, 1200, 200, 100))];
  assert.equal(avoidance(oracle, r(100, 900, 700, 350), { avoidLayer: "pt" }, items).state, "active");
  assert.equal(avoidance(oracle, r(100, 100, 300, 300), { avoidLayer: "pt" }, items).state, "clear");
});

test("a layer the card does not draw leaves the text the whole box, and a layer that is not there is said", () => {
  const idle = avoidance(oracle, r(0, 0, 100, 100), { avoidLayer: "pt" }, [layer("pt", null, false)]);
  assert.equal(idle.state, "idle");
  assert.equal(idle.rect, null);
  assert.match(idle.note, /does not draw, so the text has the whole box/);
  const gone = avoidance(oracle, r(0, 0, 100, 100), { avoidLayer: "pt" }, [layer("frame", r(0, 0, 10, 10))]);
  assert.equal(gone.state, "missing");
  assert.match(gone.note, /pt, which is not in the stack/);
  // A text box or art window of the same key is not a layer
  assert.equal(avoidance(oracle, r(0, 0, 10, 10), { avoidLayer: "oracle" }, [oracle]).state, "missing");
});

test("a layer that is drawn but does not reach the box says so", () => {
  const got = avoidance(oracle, r(0, 0, 100, 100), { avoidLayer: "pt" }, [layer("pt", r(500, 500, 50, 50))]);
  assert.equal(got.state, "clear");
  assert.match(got.note, /does not reach this box/);
});

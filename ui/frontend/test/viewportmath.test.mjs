import test from "node:test";
import assert from "node:assert/strict";

import { STEPS, MIN_ZOOM, MAX_ZOOM, clampZoom, stepZoom, wheelZoom, zoomLabel, cycleZoom, anchorShift } from "../js/viewportmath.js";

test("the buttons step through the stops, from where the picture is", () => {
  assert.equal(stepZoom(1, 1), 1.5);
  assert.equal(stepZoom(1, -1), 0.75);
  // From a zoom between stops it goes to the next one in the direction asked
  assert.equal(stepZoom(1.2, 1), 1.5);
  assert.equal(stepZoom(1.2, -1), 1);
  assert.equal(stepZoom(0.3, -1), 0.25);
  // A zoom on a stop is not stepped to itself
  assert.equal(stepZoom(2, 1), 3);
  assert.equal(stepZoom(2, -1), 1.5);
  // At either end there is nowhere to go
  assert.equal(stepZoom(STEPS.at(-1), 1), null);
  assert.equal(stepZoom(STEPS[0], -1), null);
  assert.equal(stepZoom(0.1, -1), null);
});

test("a zoom is kept between the least and the most", () => {
  assert.equal(clampZoom(0.01), MIN_ZOOM);
  assert.equal(clampZoom(100), MAX_ZOOM);
  assert.equal(clampZoom(1.7), 1.7);
});

test("a turn of the wheel up zooms in and down zooms out, by more for a longer turn", () => {
  const up = wheelZoom(1, -100);
  const down = wheelZoom(1, 100);
  assert.ok(up > 1 && down < 1);
  assert.ok(wheelZoom(1, -300) > up);
  // In and out by the same turn come back to where they began
  assert.ok(Math.abs(wheelZoom(wheelZoom(1, -120), 120) - 1) < 1e-9);
  // A wheel that reports lines turns further than one pixel's worth, and a pinch is finer
  assert.ok(wheelZoom(1, -3, { mode: 1 }) > wheelZoom(1, -3));
  assert.ok(wheelZoom(1, -3, { pinch: true }) > wheelZoom(1, -3));
  // It never leaves the range
  assert.equal(wheelZoom(MAX_ZOOM, -1000), MAX_ZOOM);
  assert.equal(wheelZoom(MIN_ZOOM, 1000), MIN_ZOOM);
});

test("a zoom is called by its percent, and zero is a fit", () => {
  assert.equal(zoomLabel(0), "Fit");
  assert.equal(zoomLabel(1), "100%");
  assert.equal(zoomLabel(1.5), "150%");
  assert.equal(zoomLabel(0.333), "33%");
});

test("the label cycles fit, 100% and 200%", () => {
  assert.equal(cycleZoom(0), 1);
  assert.equal(cycleZoom(1), 2);
  assert.equal(cycleZoom(2), 0);
  assert.equal(cycleZoom(3), 1);
  assert.equal(cycleZoom(0.5), 1);
});

test("the point under the anchor stays under it when the picture grows", () => {
  // A picture 100 wide at 0,0 that grows to 200 wide at the same corner, with the anchor
  // at 50,50, which is the middle of it. The middle is now at 100,100, so it moves back
  assert.deepEqual(anchorShift({ left: 0, top: 0, width: 100, height: 100 }, { left: 0, top: 0, width: 200, height: 200 }, { x: 50, y: 50 }), { x: -50, y: -50 });
  // Grown evenly about the middle, with the anchor at the middle, it needs no shift
  assert.deepEqual(anchorShift({ left: 50, top: 50, width: 100, height: 100 }, { left: 0, top: 0, width: 200, height: 200 }, { x: 100, y: 100 }), { x: 0, y: 0 });
  // An anchor at the corner it grew from needs none either
  assert.deepEqual(anchorShift({ left: 10, top: 20, width: 100, height: 100 }, { left: 10, top: 20, width: 300, height: 300 }, { x: 10, y: 20 }), { x: 0, y: 0 });
  // A picture with no size is anchored at its middle
  assert.deepEqual(anchorShift({ left: 0, top: 0, width: 0, height: 0 }, { left: 0, top: 0, width: 100, height: 100 }, { x: 0, y: 0 }), { x: -50, y: -50 });
});

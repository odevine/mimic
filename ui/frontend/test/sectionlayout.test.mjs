import test from "node:test";
import assert from "node:assert/strict";

import { SECTIONS, DEFAULTS, LEFT_SECTIONS, LEFT_DEFAULTS, MIN_HEIGHT, restore, pack, nextOpen, restFrom, resizePair } from "../js/sectionlayout.js";

test("the column starts with the layers and selection open and the problems down to their heading", () => {
  assert.deepEqual(SECTIONS, ["layers", "selection", "problems"]);
  assert.deepEqual(DEFAULTS.collapsed, [false, false, true]);
  // The layers have the most room
  assert.equal(Math.max(...DEFAULTS.weights), DEFAULTS.weights[0]);
  assert.deepEqual(restore(undefined), DEFAULTS);
});

test("what was kept is read back, and anything else is the default", () => {
  const state = { collapsed: [true, false, true], weights: [100.4, 300, 200] };
  const list = pack(state);
  assert.deepEqual(list, [1, 0, 1, 100, 300, 200]);
  assert.deepEqual(restore(list), { collapsed: [true, false, true], weights: [100, 300, 200] });
  // What was kept when the column had four sections is not read as these three
  assert.deepEqual(restore([1, 0, 0, 1, 100, 300, 200, 150]), DEFAULTS);
  for (const bad of [null, [], [1, 2, 3], "x", [...list, 1], list.map(() => NaN), { length: 10 }]) {
    assert.deepEqual(restore(bad), DEFAULTS, JSON.stringify(bad));
  }
  // A weight that is not above zero keeps the default for that section
  const odd = restore([0, 0, 0, 0, -5, 50]);
  assert.equal(odd.weights[0], DEFAULTS.weights[0]);
  assert.equal(odd.weights[1], DEFAULTS.weights[1]);
  assert.equal(odd.weights[2], 50);
  // The default is not changed by what is restored
  restore(list).weights[0] = 1;
  assert.deepEqual(restore(undefined), DEFAULTS);
});

test("a splitter lies below an open section that has another open below it", () => {
  const c = [false, true, false, true, false];
  assert.equal(nextOpen(c, 0), 2);
  assert.equal(nextOpen(c, 2), 4);
  assert.equal(nextOpen(c, 4), -1, "the last open section has none below");
  assert.equal(nextOpen(c, 1), -1, "a collapsed section has no splitter");
  assert.equal(nextOpen([false, true, true], 0), -1);
});

test("the collapsed sections at the bottom rest together, and one in the middle does not", () => {
  assert.equal(restFrom([false, false, true, true, true]), 2);
  assert.equal(restFrom([false, true, false, true, true]), 3);
  assert.equal(restFrom([true, true, true, true, true]), 0);
  assert.equal(restFrom([false, false, false, false, false]), -1);
  assert.equal(restFrom([false, true, false, false, false]), -1, "a collapsed section above an open one stays where it is");
});

test("dragging a line grows one section and shrinks the other by as much", () => {
  assert.deepEqual(resizePair([200, 300], 0, 1, 40), { hi: 240, hj: 260 });
  assert.deepEqual(resizePair([200, 300], 0, 1, -50), { hi: 150, hj: 350 });
  // Neither goes under the minimum
  assert.deepEqual(resizePair([200, 300], 0, 1, 1000), { hi: 200 + 300 - MIN_HEIGHT, hj: MIN_HEIGHT });
  assert.deepEqual(resizePair([200, 300], 0, 1, -1000), { hi: MIN_HEIGHT, hj: 200 + 300 - MIN_HEIGHT });
  // Sections apart in the list, with others between them left alone
  assert.deepEqual(resizePair([120, 0, 0, 220], 0, 3, 10), { hi: 130, hj: 210 });
  // The total is always kept
  for (const dy of [-500, -37, 0, 12, 91, 800]) {
    const { hi, hj } = resizePair([180, 260], 0, 1, dy);
    assert.equal(hi + hj, 440);
  }
});

test("a pair already at the minimum or less does not move", () => {
  assert.deepEqual(resizePair([MIN_HEIGHT, MIN_HEIGHT], 0, 1, 30), { hi: MIN_HEIGHT, hj: MIN_HEIGHT });
  assert.deepEqual(resizePair([50, 60], 0, 1, 30), { hi: 50, hj: 60 });
});

test("the left column has its own sections, the search above the test card, with only the test card open", () => {
  assert.deepEqual(LEFT_SECTIONS, ["search", "card"]);
  assert.deepEqual(LEFT_DEFAULTS.collapsed, [true, false]);
  assert.equal(LEFT_DEFAULTS.weights.length, LEFT_SECTIONS.length);
  assert.deepEqual(restore(undefined, LEFT_DEFAULTS), LEFT_DEFAULTS);
  // What it kept is read as its own, and what the right column kept is not
  const kept = pack({ collapsed: [false, false], weights: [300, 90] });
  assert.deepEqual(kept, [0, 0, 300, 90]);
  assert.deepEqual(restore(kept, LEFT_DEFAULTS), { collapsed: [false, false], weights: [300, 90] });
  assert.deepEqual(restore([1, 0, 0, 1, 100, 300, 200, 150], LEFT_DEFAULTS), LEFT_DEFAULTS);
});

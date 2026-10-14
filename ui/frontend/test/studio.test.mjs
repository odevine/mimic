import test from "node:test";
import assert from "node:assert/strict";

import { parseDpi, forkName, checkName, draftLabel, layoutLine, problems, cardLine, startingCard, forkedFrom, DPI_DEFAULT } from "../js/studio.js";

test("an empty resolution is the default and a number in range is itself", () => {
  assert.deepEqual(parseDpi(""), { dpi: DPI_DEFAULT, error: "" });
  assert.deepEqual(parseDpi("  "), { dpi: DPI_DEFAULT, error: "" });
  assert.deepEqual(parseDpi("300"), { dpi: 300, error: "" });
  assert.deepEqual(parseDpi(" 72 "), { dpi: 72, error: "" });
  assert.deepEqual(parseDpi("1200"), { dpi: 1200, error: "" });
});

test("a resolution out of range or not a number is refused with the reason", () => {
  for (const bad of ["71", "1201", "0"]) assert.match(parseDpi(bad).error, /72 to 1200/, bad);
  for (const bad of ["abc", "300.5", "3e2", "300 dpi", "-300"]) assert.match(parseDpi(bad).error, /whole number/, bad);
  assert.equal(parseDpi("5000").dpi, 0);
});

test("a copy is named for what it copies and stays within the limit", () => {
  assert.equal(forkName("Modern frame (M15)"), "Modern frame (M15) copy");
  assert.equal(forkName(""), "Template copy");
  assert.equal(forkName(undefined), "Template copy");
  const long = forkName("x".repeat(200));
  assert.ok(long.length <= 80, long.length);
});

test("a name must be there and not too long", () => {
  assert.deepEqual(checkName("  My frame "), { name: "My frame", error: "" });
  assert.match(checkName("   ").error, /name/);
  assert.match(checkName("x".repeat(81)).error, /80/);
  assert.equal(checkName("x".repeat(80)).error, "");
});

test("a draft that cannot be read says so in the picker", () => {
  assert.equal(draftLabel({ name: "A" }), "A");
  assert.equal(draftLabel({ name: "A", problem: "bad" }), "A (cannot be read)");
});

test("a layout is described by its canvas, with a turn when it has one", () => {
  assert.equal(layoutLine({ name: "normal", width: 2176, height: 2960, dpi: 800 }), "normal · 2176 × 2960 px · 800 dpi");
  assert.equal(layoutLine({ name: "saga", width: 100, height: 50, dpi: 72, rotate: 90 }), "saga · 100 × 50 px · 72 dpi, turned 90°");
});

test("diagnostics split into errors and warnings, each with its path", () => {
  const out = problems([
    { severity: "error", path: "layouts.normal", message: "no serves" },
    { severity: "warning", message: "no description" },
    { severity: "error", message: "broken" },
  ]);
  assert.deepEqual(out.errors, ["layouts.normal: no serves", "broken"]);
  assert.deepEqual(out.warnings, ["no description"]);
  assert.deepEqual(problems(null), { errors: [], warnings: [] });
});

test("a card is named with its set when it has one", () => {
  assert.equal(cardLine({ Name: "Sol Ring", SetCode: "c21" }), "Sol Ring · C21");
  assert.equal(cardLine({ Name: "Sol Ring" }), "Sol Ring");
  assert.equal(cardLine(null), "");
});

test("a draft opens with its last test card, or else the first preset", () => {
  const presets = [{ id: "white-creature", card: { Name: "A" } }, { id: "land", card: { Name: "B" } }];
  assert.deepEqual(startingCard({ testCard: { Name: "Bolt" }, face: 1 }, presets), { card: { Name: "Bolt" }, face: 1, preset: "" });
  assert.deepEqual(startingCard({}, presets), { card: { Name: "A" }, face: 0, preset: "white-creature" });
  assert.deepEqual(startingCard(null, presets), { card: { Name: "A" }, face: 0, preset: "white-creature" });
  assert.equal(startingCard({}, []), null);
});

test("a copy says where it began", () => {
  assert.equal(forkedFrom({ forkedFrom: "mimic.m15 0.1.0" }), "Copy of mimic.m15 0.1.0");
  assert.equal(forkedFrom({}), "");
});

test("a canvas is the size of a card with its bleed at the resolution", async () => {
  const { canvasSize } = await import("../js/studio.js");
  assert.deepEqual(canvasSize(800), { width: 2176, height: 2960 });
  assert.deepEqual(canvasSize(72), { width: 196, height: 266 });
});

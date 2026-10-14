import test from "node:test";
import assert from "node:assert/strict";

import { parseDpi, forkName, checkName, draftLabel, layoutLine, problems, cardLine, startingCard, forkedFrom, DPI_DEFAULT, creditLine, bytesLabel, useMessage, exportMessage, releaseFacts, checkTestCardLabel, suggestedLabel, TEST_CARD_LABEL_MAX, coverageTile, coverageSummary } from "../js/editor.js";

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

test("a draft opens with its last test card, or else the first preset, which is looked up", () => {
  const presets = [{ id: "monocolored", name: "Mirran Spy" }, { id: "land", name: "Flooded Strand" }];
  assert.deepEqual(startingCard({ testCard: { Name: "Bolt" }, face: 1 }, presets), { card: { Name: "Bolt" }, face: 1, preset: "" });
  assert.deepEqual(startingCard({}, presets), { card: null, face: 0, preset: "monocolored" });
  assert.deepEqual(startingCard(null, presets), { card: null, face: 0, preset: "monocolored" });
  assert.equal(startingCard({}, []), null);
});

test("a copy says where it began", () => {
  assert.equal(forkedFrom({ forkedFrom: "mimic.m15 0.1.0" }), "Copy of mimic.m15 0.1.0");
  assert.equal(forkedFrom({}), "");
});

test("a canvas is the size of a card with its bleed at the resolution", async () => {
  const { canvasSize } = await import("../js/editor.js");
  assert.deepEqual(canvasSize(800), { width: 2176, height: 2960 });
  assert.deepEqual(canvasSize(72), { width: 196, height: 266 });
});

test("a credit is a line of its name and note, and an unnamed one still has a line", () => {
  assert.equal(creditLine({ name: "Jane Doe", note: "frame art" }), "Jane Doe, frame art");
  assert.equal(creditLine({ name: " Jane ", url: "https://x" }), "Jane");
  assert.equal(creditLine({}), "Unnamed credit");
});

test("a size is in kilobytes under a megabyte and megabytes above it", () => {
  assert.equal(bytesLabel(10), "1 KB");
  assert.equal(bytesLabel(2500), "3 KB");
  assert.equal(bytesLabel(1_500_000), "1.5 MB");
  assert.equal(bytesLabel(25_000_000), "25 MB");
});

test("using a template says what it is called, whether it was replaced and whether it is in use", () => {
  const first = useMessage({ template: "user.mine", replaced: false, active: false }, "Mine");
  assert.equal(first, "Mine is ready in Single, List and Run as user.mine. Choose it in Templates to draw with it.");
  const again = useMessage({ template: "user.mine", replaced: true, active: true }, "Mine");
  assert.equal(again, "Mine is up to date in Single, List and Run as user.mine. It is the template in use, so the next card shows the change.");
});

test("an export names the file and its size, whichever way the path is written", () => {
  assert.equal(exportMessage({ path: "/Users/o/Desktop/user.mine-0.1.0.mimic", bytes: 2_000_000 }), "Saved user.mine-0.1.0.mimic (2.0 MB)");
  assert.equal(exportMessage({ path: "C:\\out\\a.mimic", bytes: 900 }), "Saved a.mimic (1 KB)");
});

test("the export dialog states what the template says and skips what it does not", () => {
  const facts = releaseFacts({ name: "Mine", templateId: "user.mine", version: "0.1.0", author: "", license: "CC0-1.0" });
  assert.deepEqual(facts, [
    { label: "Template", value: "Mine" },
    { label: "Id", value: "user.mine" },
    { label: "Version", value: "0.1.0" },
    { label: "License", value: "CC0-1.0" },
  ]);
});

test("a test card's name is tidied and held to the length the app allows", () => {
  assert.deepEqual(checkTestCardLabel("  Long   title "), { label: "Long title", error: "" });
  assert.match(checkTestCardLabel("  ").error, /Give the test card a name/);
  assert.match(checkTestCardLabel("x".repeat(TEST_CARD_LABEL_MAX + 1)).error, /at most 40/);
  assert.equal(checkTestCardLabel("x".repeat(TEST_CARD_LABEL_MAX)).error, "");
  assert.equal(suggestedLabel({ Name: "Mirran Spy" }), "Mirran Spy");
  assert.equal(suggestedLabel({ Name: "y".repeat(60) }).length, TEST_CARD_LABEL_MAX);
  assert.equal(suggestedLabel(null), "");
});

test("a coverage tile says where its card is, and why one could not be drawn", () => {
  const p = { id: "land", label: "Land" };
  assert.deepEqual(coverageTile(p), { id: "land", label: "Land", state: "waiting", text: "Not drawn yet" });
  assert.equal(coverageTile(p, { state: "drawing" }).text, "Drawing");
  assert.equal(coverageTile(p, { state: "drawn" }).text, "Drawn");
  assert.equal(coverageTile(p, { state: "failed", reason: "no frame for this land" }).text, "no frame for this land");
  assert.equal(coverageTile(p, { state: "failed" }).text, "Could not be drawn");
});

test("the coverage line counts the cards drawn, names those that were not, and says when it is out of date", () => {
  const tile = (label, state) => ({ label, state });
  assert.match(coverageSummary([tile("A", "waiting"), tile("B", "waiting")]), /Draws every test card/);
  assert.equal(coverageSummary([tile("A", "drawn"), tile("B", "drawn")]), "2 of 2 drawn");
  assert.equal(coverageSummary([tile("A", "drawn"), tile("B", "failed"), tile("C", "failed")]), "1 of 3 drawn. Not drawn: B, C");
  assert.equal(coverageSummary([tile("A", "drawn")], true), "1 of 1 drawn. The template has changed since");
});

import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { at, done, draftSummary, draftView, describeView, standardItems } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The standard items, with the type line set on its baseline, which the layout writes at
// (100, 820) across 800, and the title too, higher on the card
const baselineItems = (extra = {}) =>
  standardItems.map((it) => {
    if (it.kind === "text" && it.key === "type") return { ...it, baseline: at(100, 820, 800, 0), ...extra.type };
    if (it.kind === "text" && it.key === "title") return { ...it, ink: true, delivered: at(100, 100, 420, 52), authored: at(100, 100, 420, 52), baseline: at(100, 160, 600, 0), ...extra.title };
    if (it.kind === "text" && it.key === "oracle") return { ...it, ...extra.oracle };
    return it;
  });

// The P/T box's frame as a layer the oracle box steps around, reaching into its bottom right
const ptLayer = { kind: "layer", key: "pt", part: "pt", drawn: true, why: "", authored: at(700, 1100, 200, 150), delivered: at(700, 1100, 200, 150) };
const flowItems = () => [...standardItems.filter((i) => i.key !== "pt" || i.kind !== "text"), ptLayer];

async function ready(bridge, page, { items = baselineItems(), avoid } = {}) {
  if (avoid !== undefined) {
    bridge.source.layout.stack.push({ id: "pt", part: "pt" });
    if (avoid) bridge.source.layout.textBoxes.oracle.flow = { avoidLayer: avoid };
  }
  bridge.reply("Studio.Describe", () => describeView(items));
  bridge.reply("Studio.Apply", (req) => ({ rev: 1, inverse: req.ops.map((op) => ({ ...op })).reverse(), valid: true, diagnostics: [] }));
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.layers).toHaveCount(items.length);
  const box = await editor.sizeCanvas(400);
  return { editor, box };
}

const select = (editor, name) => editor.row(name).click();
const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);
const marks = (page) => page.locator("#ed-overlay .ed-box.ghost, #ed-overlay .ed-box.link, #ed-overlay .ed-box.hazard, #ed-overlay .ed-caption");

async function drag(page, from, by, { steps = 6 } = {}) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + by.x, from.y + by.y, { steps });
  await page.mouse.up();
}

test("a baseline box reads as its line, and its ends stretch it down to a pixel unless it is locked", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await select(editor, "type");
  await expect(page.locator("#ed-overlay .ed-box.baseline.h")).toHaveCount(1);
  await expect(page.locator(".ed-readout-grid dt")).toHaveText(["X", "Width", "Baseline", ""]);
  await expect(page.locator("#ed-sel-y")).toHaveText("820");
  await expect(page.locator("#ed-sel-h")).toHaveText("");
  // It has an end at each side and none of the handles of an area
  await expect(page.locator("#ed-overlay .ed-handle:not([data-line])")).toHaveCount(0);
  const end = (side) => page.locator(`#ed-overlay .ed-baseline-handle[data-handle='${side}']`);
  const middle = async (side) => {
    const b = await end(side).boundingBox();
    return { x: b.x + b.width / 2, y: b.y + b.height / 2 };
  };

  // 40 pixels on screen are 100 of the finished image's at this size
  await drag(page, await middle("e"), { x: -40, y: 0 });
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "stretch", layout: "normal", kind: "text", key: "type", edge: "e", dx: -100, dy: 0 }]);
  // Far past the other end, the line stops one pixel long
  await drag(page, await middle("w"), { x: 400, y: 0 }, { steps: 8 });
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "stretch", edge: "w", dx: 799 });

  await editor.lockToggle("type").click();
  await expect(page.locator("#ed-overlay .ed-baseline-handle")).toHaveCount(0);
  await select(editor, "oracle");
  await expect(page.locator(".ed-readout-grid dt")).toHaveText(["X", "Width", "Y", "Height"]);
  await expect(page.locator("#ed-overlay .ed-box.baseline")).toHaveCount(0);
});

test("a baseline box is moved by its line, and its line snaps to another box's", async ({ bridge, page }) => {
  const { editor, box } = await ready(bridge, page);
  // The title's baseline at 160 is a line the type line snaps to
  await editor.snapToggle("title").click();
  await select(editor, "type");
  const ink = EditorPage.at(box, 300, 810);
  // Taken up by 650 of the finished image's pixels its baseline is at 170, within reach of 160
  await page.mouse.move(ink.x, ink.y);
  await page.mouse.down();
  await page.mouse.move(ink.x, ink.y - (650 * box.height) / 1400, { steps: 6 });
  await expect(page.locator("#ed-overlay .ed-guide.y .ed-guide-label")).toHaveText("baseline to title baseline");
  await page.mouse.up();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "move", layout: "normal", kind: "text", key: "type", dx: 0, dy: -660 }]);
});

test("the note under the selection says how the card placed the box, which override is in force and how the text fitted", async ({ bridge, page }) => {
  bridge.source.layout.textBoxes.title.flow = { clearOf: "mana", duck: { layer: "pt", row: "artist" } };
  const items = baselineItems({
    title: { narrowed: true, moved: true, placed: at(100, 700, 380, 80) },
    oracle: { override: { when: "land", index: 0 }, fit: { size: 24, wanted: 36, floor: 20, lines: 5, overflows: false } },
    type: { fit: { size: 20, wanted: 40, floor: 20, lines: 1, overflows: true } },
  });
  const { editor } = await ready(bridge, page, { items });
  const note = page.locator("#ed-selection-note");
  await select(editor, "title");
  await expect(page.locator("#ed-overlay .ed-box.placed-line.h")).toHaveCount(1);
  await expect(note).toContainText("Moved to the row of artist and narrowed to stay clear of mana for this card");
  await select(editor, "oracle");
  await expect(note).toContainText('An override for "land" is in force for this card');
  await expect(note).toContainText("The text is shrunk to 24 px to fit, from the 36 px the box asks for");
  await expect(note).toHaveAttribute("data-level", "info");
  await select(editor, "type");
  await expect(note).toContainText("The text does not fit even at its smallest size, 20 px, so it runs past the box");
  await expect(note).toHaveAttribute("data-level", "warn");
  await select(editor, "frame");
  await expect(note).toBeHidden();
});

test("a box that steps around a layer outlines it and stripes where they meet, from either side", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page, { items: flowItems(), avoid: "pt" });
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.link")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.hazard")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.sel .ed-tag.avoid")).toHaveText("Avoids pt");
  await expect(page.locator("#ed-selection-note")).toHaveText("Steps around pt");

  await select(editor, "pt");
  await expect(page.locator("#ed-overlay .ed-box.link.by")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.sel .ed-tag.avoid")).toHaveText("Avoided by oracle");
  await select(editor, "frame");
  await expect(page.locator("#ed-overlay .ed-box.link, #ed-overlay .ed-box.hazard")).toHaveCount(0);
});

test("the flow fields name the layers and the other boxes, write the setting, and clearing one takes it out", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page, { items: flowItems(), avoid: "" });
  // The files take each setting, so the field reads it back
  bridge.reply("Studio.Apply", (req) => {
    const flow = (bridge.source.layout.textBoxes.oracle.flow ??= {});
    for (const op of req.ops) op.op === "set" ? (flow[op.path[1]] = op.value) : delete flow[op.path[1]];
    return { rev: 1, inverse: [], valid: true, diagnostics: [] };
  });
  await select(editor, "oracle");
  const avoid = editor.lever("lever-avoidLayer");
  await expect(avoid.locator("option")).toHaveText(["Default (nothing)", "background", "frame", "pt"]);
  // Not the box itself
  await expect(editor.lever("lever-clearOf").locator("option")).toHaveText(["Default (nothing)", "title", "type", "pt"]);
  await avoid.selectOption("pt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", layout: "normal", kind: "text", key: "oracle", path: ["flow", "avoidLayer"], value: "pt" }]);
  await editor.lever("lever-clearGap").fill("0.75");
  await editor.lever("lever-clearGap").blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "set", path: ["flow", "clearGap"], value: 0.75 });
  await avoid.selectOption("");
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops).toEqual([{ op: "unset", layout: "normal", kind: "text", key: "oracle", path: ["flow", "avoidLayer"] }]);
});

test("the overlays are hidden by their button or the O key, leave the selection's handles, and stay hidden next time", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page, { items: flowItems(), avoid: "pt" });
  const button = page.locator("#ed-overlays");
  await select(editor, "oracle");
  await expect(marks(page)).not.toHaveCount(0);
  await button.click();
  await expect(button).toHaveAttribute("aria-pressed", "false");
  await expect(marks(page)).toHaveCount(0);
  await expect(editor.selectionBox).toHaveCount(1);
  await expect(editor.handles).not.toHaveCount(0);
  await expect.poll(() => bridge.calledWith("Settings.Put").at(-1)?.[0].splits.editorOverlays).toEqual([1]);
  await page.keyboard.press("o");
  await expect(marks(page)).not.toHaveCount(0);
  await expect.poll(() => bridge.calledWith("Settings.Put").at(-1)[0].splits.editorOverlays).toEqual([0]);
  // Typing the letter into a field does not toggle it
  await editor.openFine();
  await editor.cardField("name").click();
  await page.keyboard.type("o");
  await expect(button).toHaveAttribute("aria-pressed", "true");

  bridge.reply("Settings.Get", { ...bridge.defaults["Settings.Get"], splits: { editorOverlays: [1] } });
  await bridge.boot();
  await editor.show();
  await expect(button).toHaveAttribute("aria-pressed", "false");
});

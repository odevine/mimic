import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { at, done, drawn, draftSummary, draftView, describeView, standardItems } from "../../bridge/cards.mjs";

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

async function ready(bridge, page, { items = baselineItems(), describe, count = 7 } = {}) {
  bridge.reply("Studio.Describe", describe || (() => describeView(items)));
  bridge.reply("Studio.Apply", (req) => ({ rev: 1, inverse: req.ops.map((op) => ({ ...op })).reverse(), valid: true, diagnostics: [] }));
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.layers).toHaveCount(count);
  const box = await editor.sizeCanvas(400);
  return { editor, box, point: (x, y) => EditorPage.at(box, x, y) };
}

const select = (editor, name) => editor.layers.filter({ has: editor.page.locator(".ed-layer-name", { hasText: new RegExp(`^${name}$`) }) }).click();
const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);

async function drag(page, from, by, { steps = 6 } = {}) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + by.x, from.y + by.y, { steps });
  await page.mouse.up();
}

test("a baseline box has its line drawn across its width, with an end to drag at each side", async ({ bridge, page }) => {
  const { editor, box } = await ready(bridge, page);
  await select(editor, "type");
  const line = page.locator("#ed-overlay .ed-box.baseline.h");
  await expect(line).toHaveCount(1);
  // The word is in a caption beneath the outline of the ink, and not on the line over the text
  const tag = page.locator("#ed-overlay .ed-box.sel .ed-caption .ed-tag.line");
  await expect(tag).toHaveText("baseline");
  const ink = await editor.selectionBox.boundingBox();
  const named = await tag.boundingBox();
  expect(named.y).toBeGreaterThanOrEqual(ink.y + ink.height);
  await expect(line.locator(".ed-baseline-label")).toHaveCount(0);
  const l = await line.boundingBox();
  expect(Math.abs(l.x - (box.x + box.width * 0.1))).toBeLessThan(1.5);
  expect(Math.abs(l.width - box.width * 0.8)).toBeLessThan(1.5);
  expect(Math.abs(l.y - (box.y + (box.height * 820) / 1400))).toBeLessThan(1.5);
  const ends = page.locator("#ed-overlay .ed-baseline-handle");
  await expect(ends).toHaveCount(2);
  await expect(ends.nth(0)).toHaveAttribute("data-handle", "w");
  await expect(ends.nth(1)).toHaveAttribute("data-handle", "e");
  const w = await ends.nth(0).boundingBox();
  const e = await ends.nth(1).boundingBox();
  expect(Math.abs(w.x + w.width / 2 - l.x)).toBeLessThan(2);
  expect(Math.abs(e.x + e.width / 2 - (l.x + l.width))).toBeLessThan(2);
  expect(Math.abs(w.y + w.height / 2 - l.y)).toBeLessThan(2);
  // The box has none of the handles of an area, since its height is not used
  await expect(page.locator("#ed-overlay .ed-handle:not([data-line])")).toHaveCount(0);
  // Another item has no baseline
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.baseline")).toHaveCount(0);
  await expect(page.locator("#ed-overlay .ed-baseline-handle")).toHaveCount(0);
});

test("the selection of a baseline box reads as its line, and the box's own height is left out", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await select(editor, "type");
  await expect(page.locator(".ed-readout-grid dt")).toHaveText(["X", "Width", "Baseline", ""]);
  await expect(page.locator("#ed-sel-x")).toHaveText("100");
  await expect(page.locator("#ed-sel-w")).toHaveText("800");
  await expect(page.locator("#ed-sel-y")).toHaveText("820");
  await expect(page.locator("#ed-sel-h")).toHaveText("");
  await select(editor, "oracle");
  await expect(page.locator(".ed-readout-grid dt")).toHaveText(["X", "Width", "Y", "Height"]);
  await expect(page.locator("#ed-sel-h")).toHaveText("300");
});

test("dragging the right end of the line stretches the box by the distance, and the line follows", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await select(editor, "type");
  const end = await page.locator("#ed-overlay .ed-baseline-handle[data-handle='e']").boundingBox();
  const from = { x: end.x + end.width / 2, y: end.y + end.height / 2 };
  const line = page.locator("#ed-overlay .ed-box.baseline.h");
  const before = await line.boundingBox();
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x - 40, from.y, { steps: 5 });
  // The line is shorter while it is dragged, and nothing is saved until the press ends
  await expect.poll(async () => (await line.boundingBox()).width).toBeLessThan(before.width - 30);
  expect(applied(bridge)).toEqual([]);
  await page.mouse.up();
  await expect.poll(() => applied(bridge).length).toBe(1);
  // 40 pixels on screen are 100 of the finished image's at this size
  expect(applied(bridge)[0].ops).toEqual([{ op: "stretch", layout: "normal", kind: "text", key: "type", edge: "e", dx: -100, dy: 0 }]);
});

test("dragging the left end moves that side, and the line stops at a pixel", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await select(editor, "type");
  const end = await page.locator("#ed-overlay .ed-baseline-handle[data-handle='w']").boundingBox();
  await drag(page, { x: end.x + end.width / 2, y: end.y + end.height / 2 }, { x: 20, y: 0 });
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "stretch", layout: "normal", kind: "text", key: "type", edge: "w", dx: 50, dy: 0 }]);
  // Far past the other end, the line stops one pixel long and the move is only as far as it went
  const again = await page.locator("#ed-overlay .ed-baseline-handle[data-handle='w']").boundingBox();
  await drag(page, { x: again.x + again.width / 2, y: again.y + again.height / 2 }, { x: 400, y: 0 }, { steps: 8 });
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "stretch", edge: "w", dx: 799 });
});

test("a locked baseline box has no ends to drag", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  bridge.reply("Studio.SetMarks", null);
  await editor.lockToggle("type").click();
  await select(editor, "type");
  await expect(page.locator("#ed-overlay .ed-box.baseline")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-baseline-handle")).toHaveCount(0);
});

test("a baseline box is moved by its line and the line is what lines up with another's", async ({ bridge, page }) => {
  const { editor, box } = await ready(bridge, page);
  bridge.reply("Studio.SetMarks", null);
  // The title's baseline at 160 is a line the type line snaps to
  await editor.snapToggle("title").click();
  await select(editor, "type");
  const ink = EditorPage.at(box, 300, 810);
  // Press on the type's ink and take it up by 650 of the finished image's pixels, so its baseline
  // is at 170, within reach of the title's at 160
  await page.mouse.move(ink.x, ink.y);
  await page.mouse.down();
  await page.mouse.move(ink.x, ink.y - (650 * box.height) / 1400, { steps: 6 });
  const guide = page.locator("#ed-overlay .ed-guide.y .ed-guide-label");
  await expect(guide).toHaveText("baseline to title baseline");
  // The line moves with the box while it is held
  const line = await page.locator("#ed-overlay .ed-box.baseline.h").first().boundingBox();
  expect(Math.abs(line.y - (box.y + (box.height * 160) / 1400))).toBeLessThan(2);
  await page.mouse.up();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "move", layout: "normal", kind: "text", key: "type", dx: 0, dy: -660 }]);
});

test("a box the card narrowed or moved is outlined where it ends up, and the note says for what", async ({ bridge, page }) => {
  bridge.source.layout.textBoxes.title.flow = { clearOf: "mana", duck: { layer: "pt", row: "artist" } };
  const items = baselineItems({ title: { narrowed: true, moved: true, placed: at(100, 700, 380, 80) } });
  const { editor, box } = await ready(bridge, page, { items });
  await select(editor, "title");
  // A box on its baseline is marked by the line it ends up on, and not by a rectangle that hangs
  // below the line where its text is not
  const placed = page.locator("#ed-overlay .ed-box.placed-line.h");
  await expect(placed).toHaveCount(1);
  const l = await placed.boundingBox();
  expect(Math.abs(l.x - (box.x + box.width * 0.1))).toBeLessThan(1.5);
  expect(Math.abs(l.width - box.width * 0.38)).toBeLessThan(1.5);
  expect(Math.abs(l.y - (box.y + (box.height * 700) / 1400))).toBeLessThan(3);
  // The box's own line is under it, so the part the card gave up shows past its end
  const own = await page.locator("#ed-overlay .ed-box.baseline.h").boundingBox();
  expect(own.width).toBeGreaterThan(l.width);
  await expect(page.locator("#ed-overlay .ed-box.sel .ed-caption .ed-tag.placed")).toHaveText("Moved and narrowed");
  await expect(page.locator("#ed-selection-note")).toContainText("Moved to the row of artist and narrowed to stay clear of mana for this card");
  // A box the card leaves alone has none
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.placed-line")).toHaveCount(0);
});

test("an override in force and how the text fitted are said under the selection", async ({ bridge, page }) => {
  const items = baselineItems({
    oracle: { override: { when: "land", index: 0 }, fit: { size: 24, wanted: 36, floor: 20, lines: 5, overflows: false } },
    type: { fit: { size: 20, wanted: 40, floor: 20, lines: 1, overflows: true } },
  });
  const { editor } = await ready(bridge, page, { items });
  await select(editor, "oracle");
  const note = page.locator("#ed-selection-note");
  await expect(note).toContainText('An override for "land" is in force for this card');
  await expect(note).toContainText("The text is shrunk to 24 px to fit, from the 36 px the box asks for");
  await expect(note).toHaveAttribute("data-level", "info");
  await select(editor, "type");
  await expect(note).toContainText("The text does not fit even at its smallest size, 20 px, so it runs past the box");
  await expect(note).toHaveAttribute("data-level", "warn");
  await select(editor, "title");
  await expect(note).toBeHidden();
});

test("a box with an area that the card narrowed has the narrowed box for its room, with its name beneath", async ({ bridge, page }) => {
  bridge.source.layout.textBoxes.oracle.flow = { clearOf: "pt" };
  const items = baselineItems({ oracle: { narrowed: true, placed: at(100, 900, 600, 300) } });
  const { editor, box } = await ready(bridge, page, { items });
  await select(editor, "oracle");
  const ghost = page.locator("#ed-overlay .ed-box.ghost");
  await expect(ghost).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.placed-line")).toHaveCount(0);
  const p = await ghost.boundingBox();
  expect(Math.abs(p.width - box.width * 0.6)).toBeLessThan(1.5);
  const tag = page.locator("#ed-overlay .ed-box.sel .ed-caption .ed-tag.placed");
  await expect(tag).toHaveText("Narrowed");
  const sel = await editor.selectionBox.boundingBox();
  expect((await tag.boundingBox()).y).toBeGreaterThanOrEqual(sel.y + sel.height);
});

test("the marks are not drawn on the box while it is dragged", async ({ bridge, page }) => {
  const items = baselineItems({ title: { narrowed: true, placed: at(100, 160, 380, 80) } });
  const { editor, box } = await ready(bridge, page, { items });
  await select(editor, "title");
  await expect(page.locator("#ed-overlay .ed-box.placed-line")).toHaveCount(1);
  const ink = EditorPage.at(box, 300, 125);
  await page.mouse.move(ink.x, ink.y);
  await page.mouse.down();
  await page.mouse.move(ink.x + 20, ink.y, { steps: 4 });
  await expect(page.locator("#ed-overlay .ed-box.placed-line")).toHaveCount(0);
  await expect(page.locator("#ed-overlay .ed-tag.placed")).toHaveCount(0);
  await page.mouse.up();
});

test("the room a baseline box has left is a blue ghost from its text to the end the card leaves its line", async ({ bridge, page }) => {
  // The title's text ends at 520 and its line is 600 long, narrowed by the card to end at 600 from 100
  const items = baselineItems({ title: { narrowed: true, placed: at(100, 160, 500, 80) } });
  const { editor, box } = await ready(bridge, page, { items });
  await select(editor, "title");
  const ghost = page.locator("#ed-overlay .ed-box.ghost");
  await expect(ghost).toHaveCount(1);
  const g = await ghost.boundingBox();
  const ink = await editor.selectionBox.boundingBox();
  // It starts where the purple outline ends, as tall as it, and ends at the narrowed line's end
  expect(Math.abs(g.x - (ink.x + ink.width))).toBeLessThan(1.5);
  expect(Math.abs(g.height - ink.height)).toBeLessThan(1.5);
  expect(Math.abs(g.x + g.width - (box.x + box.width * 0.6))).toBeLessThan(1.5);
  // Not narrowed, it runs to the end of the box's own line
  await select(editor, "type");
  const type = page.locator("#ed-overlay .ed-box.ghost");
  await expect(type).toHaveCount(1);
  const t = await type.boundingBox();
  expect(Math.abs(t.x + t.width - (box.x + box.width * 0.9))).toBeLessThan(1.5);
});

test("a box on its baseline that stays clear of another outlines it in red, and stripes where its line would reach it", async ({ bridge, page }) => {
  bridge.source.layout.textBoxes.title.flow = { clearOf: "mana" };
  const mana = drawn("text", "mana", at(700, 100, 200, 52), { ink: true, baseline: at(700, 160, 200, 0) });
  const items = [...baselineItems({ title: { narrowed: true, placed: at(100, 160, 500, 80) } }), mana];
  const { editor, box } = await ready(bridge, page, { items, count: 8 });
  await select(editor, "title");
  const link = page.locator("#ed-overlay .ed-box.link");
  await expect(link).toHaveCount(1);
  const l = await link.boundingBox();
  expect(Math.abs(l.x - (box.x + box.width * 0.7))).toBeLessThan(1.5);
  // The title's line, 600 long from 100, reaches 700 where the mana begins, so no stripes, but a line
  // as long as the type's would
  await expect(page.locator("#ed-overlay .ed-box.hazard")).toHaveCount(0);
  await expect(page.locator("#ed-overlay .ed-box.sel .ed-tag.avoid")).toHaveText("Clears mana");
  // Stretching the title's line over the mana, held, stripes the overlap
  const end = await page.locator("#ed-overlay .ed-baseline-handle[data-handle='e']").boundingBox();
  await page.mouse.move(end.x + end.width / 2, end.y + end.height / 2);
  await page.mouse.down();
  await page.mouse.move(end.x + end.width / 2 + 40, end.y + end.height / 2, { steps: 5 });
  await expect(page.locator("#ed-overlay .ed-box.hazard")).toHaveCount(1);
  const h = await page.locator("#ed-overlay .ed-box.hazard").boundingBox();
  expect(Math.abs(h.x - (box.x + box.width * 0.7))).toBeLessThan(1.5);
  await page.mouse.up();
});

test("the box a baseline box is kept clear of shows it, when it is selected", async ({ bridge, page }) => {
  bridge.source.layout.textBoxes.title.flow = { clearOf: "mana" };
  const mana = drawn("text", "mana", at(650, 100, 250, 52), { ink: true, baseline: at(650, 160, 250, 0) });
  const items = [...baselineItems(), mana];
  const { editor } = await ready(bridge, page, { items, count: 8 });
  await editor.layers.filter({ has: page.locator(".ed-layer-name", { hasText: /^mana$/ }) }).click();
  const by = page.locator("#ed-overlay .ed-box.link.by");
  await expect(by).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.sel .ed-tag.avoid")).toHaveText("Cleared by title");
  // The title's room, as wide as its line, reaches 700 and meets the mana's text from 650, so that is striped
  const stripes = page.locator("#ed-overlay .ed-box.hazard");
  await expect(stripes).toHaveCount(1);
  const hz = (await stripes.getAttribute("style")).replace(/\s/g, "");
  expect(hz).toContain("left:65%");
  expect(hz).toContain("width:5%");
});

test("with the overlays hidden a baseline box keeps its line and ends to drag, and loses what is drawn about its room", async ({ bridge, page }) => {
  bridge.reply("Settings.Put", (s) => s);
  const items = baselineItems({ title: { narrowed: true, placed: at(100, 160, 500, 80) } });
  const { editor } = await ready(bridge, page, { items });
  await select(editor, "title");
  await expect(page.locator("#ed-overlay .ed-box.ghost")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.placed-line")).toHaveCount(1);
  await page.locator("#ed-overlays").click();
  await expect(page.locator("#ed-overlay .ed-box.ghost, #ed-overlay .ed-box.placed-line, #ed-overlay .ed-caption")).toHaveCount(0);
  await expect(page.locator("#ed-overlay .ed-box.baseline.h")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-baseline-handle")).toHaveCount(2);
});

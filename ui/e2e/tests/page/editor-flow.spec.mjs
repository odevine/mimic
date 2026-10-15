import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { at, done, draftSummary, draftView, describeView, standardItems } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The P/T box's frame, a layer that the card draws or not
const pt = (drawn = true) =>
  drawn
    ? { kind: "layer", key: "pt", part: "pt", drawn: true, why: "", authored: at(700, 1230, 200, 100), delivered: at(700, 1230, 200, 100) }
    : { kind: "layer", key: "pt", part: "pt", drawn: false, why: "the card has no pt to draw", authored: at(0, 0, 0, 0), delivered: at(0, 0, 0, 0) };

async function ready(bridge, page, { avoid = "pt", layer = pt() } = {}) {
  bridge.source.layout.stack.push({ id: "pt", part: "pt" });
  if (avoid) bridge.source.layout.textBoxes.oracle.flow = { avoidLayer: avoid };
  bridge.reply("Studio.Describe", () => describeView([...standardItems.filter((i) => i.key !== "pt" || i.kind !== "text"), layer]));
  bridge.reply("Studio.Apply", (req) => ({ rev: 1, inverse: req.ops.map((op) => ({ ...op, op: op.op === "set" ? "unset" : "set" })), valid: true, diagnostics: [] }));
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.layers.first()).toBeVisible();
  return editor;
}

const select = (editor, name) => editor.layers.filter({ has: editor.page.locator(".ed-layer-name", { hasText: new RegExp(`^${name}$`) }) }).click();
const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);

test("the part of a text box its text keeps out of is hatched, where the layer lies inside the box", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { layer: { ...pt(), authored: at(700, 1100, 200, 150), delivered: at(700, 1100, 200, 150) } });
  await select(editor, "oracle");
  const style = await page.locator("#ed-overlay .ed-box.avoid").getAttribute("style");
  // 700..900 across and 1100..1200 down of a 1000 by 1400 card, where the box ends
  expect(style).toContain("left:70%");
  expect(style).toContain("width:20%");
  expect(style.replace(/\s/g, "")).toMatch(/top:78\.57/);
  expect(style.replace(/\s/g, "")).toMatch(/height:7\.14/);
  await expect(page.locator("#ed-overlay .ed-box.avoid .ed-avoid-label")).toHaveText("Avoids pt");
  await expect(editor.page.locator("#ed-selection-note")).toHaveText("Steps around pt");
});

test("nothing is hatched for another box, or for a layer the card does not draw, and the note says why", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { layer: pt(false) });
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.avoid")).toHaveCount(0);
  await expect(page.locator("#ed-selection-note")).toContainText("pt, which this card does not draw, so the text has the whole box");
  await select(editor, "title");
  await expect(page.locator("#ed-selection-note")).toBeHidden();
});

test("a layer that is drawn but does not reach the box says so and hatches nothing", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.avoid")).toHaveCount(0);
  await expect(page.locator("#ed-selection-note")).toContainText("pt, which does not reach this box");
});

test("a box that steps around nothing has no region and no note", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { avoid: "" });
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.avoid")).toHaveCount(0);
  await expect(page.locator("#ed-selection-note")).toBeHidden();
});

test("the flow fields name the layers and the other boxes, and write the setting", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { avoid: "" });
  await select(editor, "oracle");
  const avoid = editor.lever("lever-avoidLayer");
  await expect(avoid.locator("option")).toHaveText(["Default (nothing)", "background", "frame", "pt"]);
  const clear = editor.lever("lever-clearOf");
  // Not the box itself
  await expect(clear.locator("option")).toHaveText(["Default (nothing)", "title", "type", "pt"]);
  await avoid.selectOption("pt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", layout: "normal", kind: "text", key: "oracle", path: ["flow", "avoidLayer"], value: "pt" }]);
  await clear.selectOption("title");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "set", path: ["flow", "clearOf"], value: "title" });
  await editor.lever("lever-clearGap").fill("0.75");
  await editor.lever("lever-clearGap").blur();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ op: "set", path: ["flow", "clearGap"], value: 0.75 });
});

test("a box's flow settings are shown, and clearing one takes it out of the file", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await select(editor, "oracle");
  await expect(editor.lever("lever-avoidLayer")).toHaveValue("pt");
  await editor.lever("lever-avoidLayer").selectOption("");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "unset", layout: "normal", kind: "text", key: "oracle", path: ["flow", "avoidLayer"] }]);
});

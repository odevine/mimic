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

test("what a text box keeps its text out of is outlined, and the part of its room that is lost to it is striped", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { layer: { ...pt(), authored: at(700, 1100, 200, 150), delivered: at(700, 1100, 200, 150) } });
  await select(editor, "oracle");
  // The layer is outlined where it is, whole
  const link = page.locator("#ed-overlay .ed-box.link");
  await expect(link).toHaveCount(1);
  const linkStyle = (await link.getAttribute("style")).replace(/\s/g, "");
  expect(linkStyle).toContain("left:70%");
  expect(linkStyle).toContain("width:20%");
  expect(linkStyle).toMatch(/height:10\.71/);
  // The part of the box it covers is striped: 700..900 across and 1100..1200 down of a 1000 by 1400 card
  const stripes = (await page.locator("#ed-overlay .ed-box.hazard").getAttribute("style")).replace(/\s/g, "");
  expect(stripes).toContain("left:70%");
  expect(stripes).toContain("width:20%");
  expect(stripes).toMatch(/top:78\.57/);
  expect(stripes).toMatch(/height:7\.14/);
  // The box with an area has all of it for room, tinted
  const ghost = page.locator("#ed-overlay .ed-box.ghost");
  await expect(ghost).toHaveCount(1);
  expect((await ghost.getAttribute("style")).replace(/\s/g, "")).toContain("left:10%");
  await expect(page.locator("#ed-overlay .ed-box.sel .ed-tag.avoid")).toHaveText("Avoids pt");
  await expect(editor.page.locator("#ed-selection-note")).toHaveText("Steps around pt");
});

test("the layer a box steers around, when selected, outlines the box and stripes where their room meets", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { layer: { ...pt(), authored: at(700, 1100, 200, 150), delivered: at(700, 1100, 200, 150) } });
  await select(editor, "pt");
  const by = page.locator("#ed-overlay .ed-box.link.by");
  await expect(by).toHaveCount(1);
  expect((await by.getAttribute("style")).replace(/\s/g, "")).toContain("left:10%");
  await expect(page.locator("#ed-overlay .ed-box.hazard")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.sel .ed-tag.avoid")).toHaveText("Avoided by oracle");
  // A layer is no room, so there is no ghost for it
  await expect(page.locator("#ed-overlay .ed-box.ghost")).toHaveCount(0);
  // And a layer nothing steers around has none of it
  await select(editor, "frame");
  await expect(page.locator("#ed-overlay .ed-box.link, #ed-overlay .ed-box.hazard")).toHaveCount(0);
});

test("the outlines and stripes stay on the card while a box is dragged, and follow it", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { layer: { ...pt(), authored: at(700, 1100, 200, 150), delivered: at(700, 1100, 200, 150) } });
  const box = await editor.sizeCanvas(400);
  await select(editor, "oracle");
  const stripes = page.locator("#ed-overlay .ed-box.hazard");
  const before = await stripes.boundingBox();
  const grab = EditorPage.at(box, 400, 1050);
  await page.mouse.move(grab.x, grab.y);
  await page.mouse.down();
  // 30 pixels on screen are 75 of the finished image's, taking the box off the top of the layer
  // by that much, so less of the box meets it
  await page.mouse.move(grab.x, grab.y - 30, { steps: 6 });
  await expect(page.locator("#ed-overlay .ed-box.link")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.ghost")).toHaveCount(1);
  const during = await stripes.boundingBox();
  expect(during.height).toBeLessThan(before.height - 10);
  await page.mouse.up();
});

test("nothing is outlined or striped for a layer the card does not draw, and the note says why", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { layer: pt(false) });
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.link, #ed-overlay .ed-box.hazard")).toHaveCount(0);
  await expect(page.locator("#ed-selection-note")).toContainText("pt, which this card does not draw, so the text has the whole box");
  await select(editor, "title");
  await expect(page.locator("#ed-selection-note")).toBeHidden();
});

test("a layer that is drawn but does not reach the box is outlined, with no stripes, and the note says so", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.link")).toHaveCount(1);
  await expect(page.locator("#ed-overlay .ed-box.hazard")).toHaveCount(0);
  await expect(page.locator("#ed-selection-note")).toContainText("pt, which does not reach this box");
});

test("a box that steps around nothing has no outline or stripes and no note", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { avoid: "" });
  await select(editor, "oracle");
  await expect(page.locator("#ed-overlay .ed-box.link, #ed-overlay .ed-box.hazard")).toHaveCount(0);
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

const marks = (page) => page.locator("#ed-overlay .ed-box.ghost, #ed-overlay .ed-box.link, #ed-overlay .ed-box.hazard, #ed-overlay .ed-caption");

test("the overlays can be hidden from the view's controls, leaving the selection and its handles", async ({ bridge, page }) => {
  bridge.reply("Settings.Put", (s) => s);
  const editor = await ready(bridge, page, { layer: { ...pt(), authored: at(700, 1100, 200, 150), delivered: at(700, 1100, 200, 150) } });
  const button = page.locator("#ed-overlays");
  await expect(button).toHaveAttribute("aria-pressed", "true");
  await expect(button).toHaveAttribute("data-tip", "Show/Hide overlays (O)");
  await expect(button.locator("use")).toHaveAttribute("href", "icons.svg#i-eye");
  await select(editor, "oracle");
  await expect(marks(page)).not.toHaveCount(0);

  await button.click();
  await expect(button).toHaveAttribute("aria-pressed", "false");
  await expect(button.locator("use")).toHaveAttribute("href", "icons.svg#i-eye-off");
  await expect(marks(page)).toHaveCount(0);
  // The outline and the handles are how the item is edited, so they stay
  await expect(editor.selectionBox).toHaveCount(1);
  await expect(editor.handles).not.toHaveCount(0);
  // The note under the selection's name is not an overlay
  await expect(page.locator("#ed-selection-note")).toContainText("Steps around pt");
  // The layer's own marks go too, for the box that steers around it
  await select(editor, "pt");
  await expect(marks(page)).toHaveCount(0);

  await button.click();
  await expect(button).toHaveAttribute("aria-pressed", "true");
  await expect(marks(page)).not.toHaveCount(0);
});

test("the overlays toggle is kept with the layout, and the O key does it too", async ({ bridge, page }) => {
  bridge.reply("Settings.Put", (s) => s);
  const editor = await ready(bridge, page, { layer: { ...pt(), authored: at(700, 1100, 200, 150), delivered: at(700, 1100, 200, 150) } });
  await select(editor, "oracle");
  await page.keyboard.press("o");
  await expect(page.locator("#ed-overlays")).toHaveAttribute("aria-pressed", "false");
  await expect(marks(page)).toHaveCount(0);
  await expect.poll(() => bridge.calledWith("Settings.Put").length).toBeGreaterThan(0);
  expect(bridge.calledWith("Settings.Put").at(-1)[0].splits.editorOverlays).toEqual([1]);
  await page.keyboard.press("o");
  await expect(marks(page)).not.toHaveCount(0);
  await expect.poll(() => bridge.calledWith("Settings.Put").at(-1)[0].splits.editorOverlays).toEqual([0]);
  // Typing the letter into a field does not toggle it
  await editor.openFine();
  await editor.cardField("name").click();
  await page.keyboard.type("o");
  await expect(page.locator("#ed-overlays")).toHaveAttribute("aria-pressed", "true");
});

test("hidden overlays are still hidden when the page is opened again", async ({ bridge, page }) => {
  bridge.reply("Settings.Get", { ...bridge.defaults["Settings.Get"], splits: { editorOverlays: [1] } });
  const editor = await ready(bridge, page, { layer: { ...pt(), authored: at(700, 1100, 200, 150), delivered: at(700, 1100, 200, 150) } });
  await expect(page.locator("#ed-overlays")).toHaveAttribute("aria-pressed", "false");
  await select(editor, "oracle");
  await expect(marks(page)).toHaveCount(0);
  await expect(editor.selectionBox).toHaveCount(1);
});

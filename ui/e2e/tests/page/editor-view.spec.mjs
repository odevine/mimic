import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView } from "../../bridge/cards.mjs";
import { solidPng } from "../../png.mjs";

const mine = draftSummary("mine", "Mine");
const other = draftSummary("other", "Other");

// The editor with its card drawn from a picture 400 wide and 560 high, big enough to zoom
async function ready(bridge, page, { drafts = [mine] } = {}) {
  await page.route("**/img/render/**", (route) => route.fulfill({ contentType: "image/png", body: solidPng(400, 560, [60, 90, 140, 255]) }));
  bridge.reply("Studio.List", drafts);
  bridge.reply("Studio.Open", (id) => draftView(drafts.find((d) => d.id === id) || mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  await expect.poll(() => editor.preview.evaluate((img) => img.complete && img.naturalWidth)).toBe(400);
  return editor;
}

const imageBox = (page) => page.locator("#ed-img").boundingBox();
const label = (page) => page.locator("#ed-zoom-label");
const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);

test("the camera controls are pinned to the bottom corner of the view", async ({ bridge, page }) => {
  await ready(bridge, page);
  const camera = await page.locator(".ed-camera").boundingBox();
  const viewport = await page.locator(".ed-viewport").boundingBox();
  expect(viewport.x + viewport.width - (camera.x + camera.width)).toBeLessThan(20);
  expect(viewport.y + viewport.height - (camera.y + camera.height)).toBeLessThan(20);
  for (const id of ["ed-tool-select", "ed-tool-pan", "ed-zoom-out", "ed-zoom-label", "ed-zoom-in", "ed-zoom-fit"]) await expect(page.locator(`#${id}`)).toBeVisible();
  await expect(page.locator(".ed-camera")).toHaveAttribute("role", "toolbar");
  // They stay in the corner when the card is zoomed past the edges of the view, though
  // they grow a little to the left for a longer zoom
  await page.locator("#ed-zoom-in").click();
  await page.locator("#ed-zoom-in").click();
  await page.locator("#ed-zoom-in").click();
  const after = await page.locator(".ed-camera").boundingBox();
  expect(Math.abs(after.x + after.width - (camera.x + camera.width))).toBeLessThan(1);
  expect(Math.abs(after.y + after.height - (camera.y + camera.height))).toBeLessThan(1);
});

test("the view starts fitted, with the select tool", async ({ bridge, page }) => {
  await ready(bridge, page);
  await expect(label(page)).toHaveText("Fit");
  await expect(page.locator("#ed-tool-select")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#ed-tool-pan")).toHaveAttribute("aria-pressed", "false");
});

test("the buttons zoom in and out and the fit button puts the card back", async ({ bridge, page }) => {
  await ready(bridge, page);
  const fit = await imageBox(page);
  await page.locator("#ed-zoom-in").click();
  await expect(label(page)).toHaveText(/^\d+%$/);
  const bigger = await imageBox(page);
  expect(bigger.width).toBeGreaterThan(fit.width);
  await page.locator("#ed-zoom-out").click();
  await page.locator("#ed-zoom-out").click();
  const smaller = await imageBox(page);
  expect(smaller.width).toBeLessThan(bigger.width);
  await page.locator("#ed-zoom-fit").click();
  await expect(label(page)).toHaveText("Fit");
  const back = await imageBox(page);
  expect(Math.abs(back.width - fit.width)).toBeLessThan(1);
  expect(Math.abs(back.x - fit.x) + Math.abs(back.y - fit.y)).toBeLessThan(1);
});

test("the zoom label cycles fit, 100% and 200%, and 100% is the picture's own size", async ({ bridge, page }) => {
  await ready(bridge, page);
  // One of the picture's pixels for one of the screen's, which is fewer of the page's
  // pixels on a screen that has more than one to each
  const ratio = await page.evaluate(() => window.devicePixelRatio);
  await label(page).click();
  await expect(label(page)).toHaveText("100%");
  expect(Math.round((await imageBox(page)).width)).toBe(Math.round(400 / ratio));
  await label(page).click();
  await expect(label(page)).toHaveText("200%");
  expect(Math.round((await imageBox(page)).width)).toBe(Math.round(800 / ratio));
  await label(page).click();
  await expect(label(page)).toHaveText("Fit");
});

test("turning the wheel up over the card zooms in about the pointer, and down zooms out", async ({ bridge, page }) => {
  await ready(bridge, page);
  const box = await imageBox(page);
  // A point on the card, a quarter of the way across and a third of the way down
  const at = { x: box.x + box.width * 0.25, y: box.y + box.height * 0.33 };
  await page.mouse.move(at.x, at.y);
  await page.mouse.wheel(0, -240);
  await expect(label(page)).toHaveText(/^\d+%$/);
  const zoomed = await imageBox(page);
  expect(zoomed.width).toBeGreaterThan(box.width);
  // The point of the card that was under the pointer still is
  expect(Math.abs((at.x - zoomed.x) / zoomed.width - 0.25)).toBeLessThan(0.02);
  expect(Math.abs((at.y - zoomed.y) / zoomed.height - 0.33)).toBeLessThan(0.02);
  await page.mouse.wheel(0, 480);
  await expect.poll(async () => (await imageBox(page)).width).toBeLessThan(zoomed.width);
});

test("the wheel stops at the most and the least", async ({ bridge, page }) => {
  await ready(bridge, page);
  const stage = await page.locator("#ed-stage").boundingBox();
  await page.mouse.move(stage.x + stage.width / 2, stage.y + stage.height / 2);
  for (let i = 0; i < 12; i++) await page.mouse.wheel(0, -600);
  await expect(label(page)).toHaveText("800%");
  for (let i = 0; i < 40; i++) await page.mouse.wheel(0, 600);
  await expect(label(page)).toHaveText("10%");
});

test("the keys zoom: plus and minus step, and 0, 1 and 2 are fit, 100% and 200%", async ({ bridge, page }) => {
  await ready(bridge, page);
  await page.keyboard.press("1");
  await expect(label(page)).toHaveText("100%");
  await page.keyboard.press("2");
  await expect(label(page)).toHaveText("200%");
  await page.keyboard.press("-");
  await expect(label(page)).toHaveText("150%");
  await page.keyboard.press("+");
  await expect(label(page)).toHaveText("200%");
  await page.keyboard.press("0");
  await expect(label(page)).toHaveText("Fit");
});

test("the zoom keys are left to a field that is being typed in", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openFine();
  await editor.cardField("name").focus();
  await page.keyboard.type("a1+2-");
  await expect(label(page)).toHaveText("Fit");
  await expect(editor.cardField("name")).toHaveValue(/a1\+2-$/);
});

test("the pan tool moves the card about the view and leaves what is on it alone", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await page.locator("#ed-tool-pan").click();
  await expect(page.locator("#ed-tool-pan")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#ed-tool-select")).toHaveAttribute("aria-pressed", "false");
  const before = await imageBox(page);
  const from = { x: before.x + before.width / 2, y: before.y + before.height / 2 };
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + 90, from.y - 40, { steps: 6 });
  await page.mouse.up();
  const after = await imageBox(page);
  expect(Math.round(after.x - before.x)).toBe(90);
  expect(Math.round(after.y - before.y)).toBe(-40);
  expect(after.width).toBeCloseTo(before.width, 0);
  // Nothing was selected or moved by it
  await expect(editor.selection).toBeHidden();
  expect(applied(bridge)).toEqual([]);
  // The card is moved by any distance, not only as far as it is bigger than the view
  await page.locator("#ed-tool-select").click();
  await page.mouse.move(after.x + after.width / 2, after.y + after.height / 2);
  await page.mouse.down();
  await page.mouse.move(after.x + after.width / 2 + 50, after.y + after.height / 2, { steps: 4 });
  await page.mouse.up();
  const still = await imageBox(page);
  expect(Math.abs(still.x - after.x)).toBeLessThan(1);
});

test("holding Space over the view pans for as long as it is held", async ({ bridge, page }) => {
  await ready(bridge, page);
  const stage = page.locator("#ed-stage");
  const before = await imageBox(page);
  const from = { x: before.x + before.width / 2, y: before.y + before.height / 2 };
  await page.mouse.move(from.x, from.y);
  await expect(stage).not.toHaveClass(/pan-mode/);
  await page.keyboard.down("Space");
  await expect(stage).toHaveClass(/pan-mode/);
  await expect(page.locator("#ed-tool-select")).toHaveAttribute("aria-pressed", "true");
  await page.mouse.down();
  await page.mouse.move(from.x - 60, from.y + 30, { steps: 5 });
  await page.mouse.up();
  await page.keyboard.up("Space");
  await expect(stage).not.toHaveClass(/pan-mode/);
  const after = await imageBox(page);
  expect(Math.round(after.x - before.x)).toBe(-60);
  expect(Math.round(after.y - before.y)).toBe(30);
});

test("Space does not pan with the pointer away from the view, and still presses a button", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await page.locator("#ed-card-toggle").focus();
  await page.mouse.move(5, 5);
  await page.keyboard.press("Space");
  await expect(page.locator("#ed-stage")).not.toHaveClass(/pan-mode/);
  // The button was pressed, which collapsed the section
  await expect(page.locator("#ed-card-body")).toBeHidden();
  void editor;
});

test("the middle button pans in any tool", async ({ bridge, page }) => {
  await ready(bridge, page);
  const before = await imageBox(page);
  const from = { x: before.x + before.width / 2, y: before.y + before.height / 2 };
  await page.mouse.move(from.x, from.y);
  await page.mouse.down({ button: "middle" });
  await page.mouse.move(from.x + 40, from.y + 40, { steps: 4 });
  await page.mouse.up({ button: "middle" });
  const after = await imageBox(page);
  expect(Math.round(after.x - before.x)).toBe(40);
  expect(Math.round(after.y - before.y)).toBe(40);
});

test("fit brings a card that was panned back to the middle", async ({ bridge, page }) => {
  await ready(bridge, page);
  const fit = await imageBox(page);
  await page.locator("#ed-zoom-in").click();
  await page.locator("#ed-tool-pan").click();
  const box = await imageBox(page);
  await page.mouse.move(box.x + 20, box.y + 20);
  await page.mouse.down();
  await page.mouse.move(box.x + 200, box.y + 120, { steps: 5 });
  await page.mouse.up();
  await page.locator("#ed-zoom-fit").click();
  const back = await imageBox(page);
  expect(Math.abs(back.x - fit.x) + Math.abs(back.y - fit.y)).toBeLessThan(1);
});

test("selecting still lands on the right item after the card is zoomed and moved", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  // At the picture's own size, moved a little, so the title stays in view
  await page.keyboard.press("1");
  await page.locator("#ed-tool-pan").click();
  const box = await imageBox(page);
  await page.mouse.move(box.x + 100, box.y + 100);
  await page.mouse.down();
  await page.mouse.move(box.x + 140, box.y + 120, { steps: 4 });
  await page.mouse.up();
  await page.locator("#ed-tool-select").click();
  // The title is at 100,100 of the 1000 by 1400 card, wherever the card is on screen now
  const overlay = await page.locator("#ed-overlay").boundingBox();
  const p = EditorPage.at(overlay, 150, 100);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");
  // And the box drawn on it is where the card shows it
  const sel = await editor.selectionBox.boundingBox();
  expect(Math.abs(sel.x - (overlay.x + overlay.width * 0.1))).toBeLessThan(2);
});

test("hovering does not outline an item while the card is being panned", async ({ bridge, page }) => {
  await ready(bridge, page);
  const overlay = await page.locator("#ed-overlay").boundingBox();
  const p = EditorPage.at(overlay, 150, 100);
  await page.mouse.move(p.x, p.y);
  await expect(page.locator("#ed-overlay .ed-box.hover")).toHaveCount(1);
  await page.locator("#ed-tool-pan").click();
  await page.mouse.move(p.x + 3, p.y + 3);
  await expect(page.locator("#ed-overlay .ed-box.hover")).toHaveCount(0);
});

test("opening another template puts the view back to fit", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { drafts: [mine, other] });
  await page.keyboard.press("2");
  await expect(label(page)).toHaveText("200%");
  await editor.picker.selectOption("other");
  await expect(label(page)).toHaveText("Fit");
});

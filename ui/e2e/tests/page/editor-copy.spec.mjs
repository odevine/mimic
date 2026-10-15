import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

async function ready(bridge, page) {
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  return editor;
}

const tip = (locator) => locator.getAttribute("data-tip");
const box = async (locator) => locator.boundingBox();
const select = (editor, name) => editor.layers.filter({ has: editor.page.locator(".ed-layer-name", { hasText: new RegExp(`^${name}$`) }) }).click();

test("the buttons and tooltips say what they do in the words the editor uses", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  expect(await tip(page.locator("#ed-delete"))).toBe("Delete template");
  await expect(page.locator("#ed-delete")).toHaveAttribute("aria-label", "Delete template");
  expect(await tip(page.locator("#ed-add"))).toBe("Add layer");
  expect(await tip(page.locator("#ed-tool-select"))).toBe("Select/Move");
  expect(await tip(page.locator("#ed-tool-pan"))).toBe("Pan");
  await expect(editor.exportButton).toHaveText("Export");
  await expect(editor.jsonButton).toHaveText("Edit JSON");
  await expect(editor.useButton).toHaveText("Use in mimic");
  await expect(editor.folderButton).toHaveText("Show folder");
  // The layer's toggles say the same thing whatever state they are in
  expect(await tip(editor.lockToggle("title"))).toBe("Lock/Unlock");
  expect(await tip(editor.snapToggle("title"))).toBe("Allow/Prevent snapping");
  await editor.lockToggle("title").click();
  expect(await tip(editor.lockToggle("title"))).toBe("Lock/Unlock");
  await editor.snapToggle("title").click();
  expect(await tip(editor.snapToggle("title"))).toBe("Allow/Prevent snapping");
});

test("the symbol palette's button is called Insert Symbol", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  const button = page.locator("#mode-editor button[data-tip='Insert Symbol']").filter({ visible: true }).first();
  await expect(button).toBeVisible();
  await expect(button).toHaveAttribute("aria-label", "Insert Symbol");
  void editor;
});

test("the ratio lock says what pressing it does", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  const lock = page.locator("#ed-canvas-levers [data-ratio='lock']");
  expect(await tip(lock)).toBe("Unlock ratio");
  await lock.click();
  expect(await tip(lock)).toBe("Lock ratio");
});

test("the selection says its name and what it is on one line, and its place and size in two columns", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await select(editor, "title");
  const head = page.locator(".ed-selection-head");
  await expect(head).toHaveText("title - Text box");
  const name = await box(page.locator("#ed-selection-name"));
  const kind = await box(page.locator("#ed-selection-kind"));
  expect(Math.abs(name.y - kind.y)).toBeLessThan(4);
  expect(kind.x).toBeGreaterThan(name.x);
  // The type is dimmer than the name, as it was
  const color = (sel) => page.locator(sel).evaluate((el) => getComputedStyle(el).color);
  expect(await color("#ed-selection-kind")).not.toBe(await color("#ed-selection-name"));
  // X over Y in one column, and width over height in the other, each beside its number
  const [x, y, w, h] = await Promise.all(["#ed-sel-x", "#ed-sel-y", "#ed-sel-w", "#ed-sel-h"].map((s) => box(page.locator(s))));
  expect(Math.abs(x.x - y.x)).toBeLessThan(2);
  expect(Math.abs(w.x - h.x)).toBeLessThan(2);
  expect(w.x).toBeGreaterThan(x.x + 20);
  expect(Math.abs(x.y - w.y)).toBeLessThan(2);
  expect(Math.abs(y.y - h.y)).toBeLessThan(2);
  expect(y.y).toBeGreaterThan(x.y + 5);
  const labels = await page.locator(".ed-readout-grid dt").allTextContents();
  expect(labels).toEqual(["X", "Width", "Y", "Height"]);
});

test("an item that is not drawn says why under its name", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(page.locator("#ed-selection-note")).toBeHidden();
  await editor.lockToggle("title").click();
  await select(editor, "title");
  await expect(page.locator("#ed-selection-note")).toHaveText("Locked, so the card cannot pick or move it");
});

test("every text button is as tall as the primary one, and a field as tall as its neighbors", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  const heights = await Promise.all([editor.useButton, editor.exportButton, editor.folderButton, editor.jsonButton, page.locator("#ed-rename")].map(async (b) => (await box(b)).height));
  expect(new Set(heights).size, `heights ${heights}`).toBe(1);
  // An icon button stays square
  const icon = await box(page.locator("#ed-new"));
  expect(icon.width).toBe(icon.height);
  // The fields of the card form are all one height, a dropdown with the text fields
  await editor.openFine();
  const fields = await page.locator("#ed-card-form input:not([type=checkbox]):not([type=radio]), #ed-card-form select").evaluateAll((els) => els.filter((el) => el.offsetParent).map((el) => Math.round(el.getBoundingClientRect().height)));
  expect(fields.length).toBeGreaterThan(3);
  expect(new Set(fields).size, `field heights ${fields}`).toBe(1);
});

test("an image's remove button is in the top right corner of the image", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await select(editor, "frame");
  const cell = editor.images.locator('.ed-cell[data-material="w"]');
  const thumb = await box(cell.locator(".ed-cell-thumb"));
  const remove = await box(cell.locator(".ed-cell-remove"));
  expect(remove.x + remove.width).toBeLessThanOrEqual(thumb.x + thumb.width + 0.5);
  expect(remove.x).toBeGreaterThan(thumb.x + thumb.width / 2);
  expect(remove.y).toBeGreaterThanOrEqual(thumb.y - 1);
  expect(remove.y + remove.height).toBeLessThanOrEqual(thumb.y + thumb.height / 2 + 4);
});

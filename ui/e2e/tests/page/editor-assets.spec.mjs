import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, describeView, standardItems, drawn, at, assetsView, partView, standardAssets } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The editor on one template with its card drawn, as the other editor page tests have it
async function ready(bridge, page, { meta, count = 7 } = {}) {
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine, meta ? { meta } : {}));
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

const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);
const select = (editor, name) => editor.layers.filter({ has: editor.page.locator(".ed-layer-name", { hasText: new RegExp(`^${name}$`) }) }).click();

test("a layer shows a cell for each material of its part, and says what covers the ones with no image", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await expect(editor.images).toBeHidden();
  await select(editor, "frame");
  await expect(editor.images).toBeVisible();
  await expect(editor.images.locator("h4")).toHaveText("Images of frame");
  await expect(editor.images.locator(".ed-images-note")).toContainText("5 of 21 materials have an image, 6 would leave a hole");
  await expect(editor.cells.locator("xpath=self::*[@data-state='image']")).toHaveCount(5);
  await expect(editor.cells.locator("xpath=self::*[@data-state='blended']")).toHaveCount(10);
  await expect(editor.cells.locator("xpath=self::*[@data-state='missing']")).toHaveCount(6);
  await expect(editor.images.locator('.ed-grid-group[aria-label="Colors"] .ed-cell-label')).toHaveText(["White", "Blue", "Black", "Red", "Green"]);
  await expect(editor.images.locator('.ed-cell[data-material="wu"] .ed-cell-note')).toHaveText("blended from White and Blue");
  await expect(editor.images.locator('.ed-cell[data-material="gold"]')).toHaveAttribute("data-state", "missing");
  // The images that exist are drawn small
  await expect(editor.images.locator('.ed-cell[data-material="w"] img')).toHaveCount(1);
  expect(bridge.calledWith("Studio.Thumbs")[0][0]).toMatchObject({ id: "mine", edge: 160 });
  expect(bridge.calledWith("Studio.Thumbs")[0][0].files).toEqual(["parts/frame/w.png", "parts/frame/u.png", "parts/frame/b.png", "parts/frame/r.png", "parts/frame/g.png"]);

  // A part with no slot has the one cell, and a text box has none
  await select(editor, "background");
  await expect(editor.images.locator("h4")).toHaveText("Images of background");
  await expect(editor.cells).toHaveCount(1);
  await expect(editor.images.locator(".ed-images-note")).toContainText("one image for every card");
  await select(editor, "title");
  await expect(editor.images).toBeHidden();
});

test("a part's images that are not one size are said", async ({ bridge, page }) => {
  bridge.assets = assetsView([partView("background", "", ["any"], { problems: ["The images are not one size: 100x100 for any; 200x200 for w"] }), partView("frame", "background", ["w"])]);
  const { editor } = await ready(bridge, page);
  await select(editor, "background");
  await expect(editor.images.locator(".ed-problem")).toHaveText("The images are not one size: 100x100 for any; 200x200 for w");
});

test("replacing an image asks for a file, saves it, and can be undone and redone", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "/art/white.png");
  bridge.reply("Studio.ImportImage", { rev: 1, inverse: [{ op: "put", file: "parts/frame/w.png", stash: "old" }], valid: true, diagnostics: [], id: "frame", warnings: [] });
  const { editor } = await ready(bridge, page);
  await select(editor, "frame");
  const renders = bridge.calledWith("Studio.Render").length;
  await editor.images.locator('.ed-cell[data-material="w"] .ed-cell-main').click();
  await expect.poll(() => bridge.calledWith("Studio.ImportImage").length).toBe(1);
  expect(bridge.calledWith("Studio.ImportImage")[0][0]).toEqual({ id: "mine", part: "frame", material: "w", slot: "", path: "/art/white.png" });
  // The card is drawn again and what the draft holds is read again
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBeGreaterThan(renders);
  await expect.poll(() => bridge.calledWith("Studio.Assets").length).toBeGreaterThan(1);
  await expect(editor.undoButton).toBeEnabled();
  await expect(page.locator("#ed-undo")).toHaveAttribute("data-tip", /Undo/);

  // Undo puts back what the app said undoes it, and redo makes it again with what undo said
  bridge.reply("Studio.Apply", (req) => ({ rev: 2, inverse: [{ op: "put", file: "parts/frame/w.png", stash: "new" }], valid: true, diagnostics: [] }));
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "put", file: "parts/frame/w.png", stash: "old" }]);
  await expect(editor.redoButton).toBeEnabled();
  await editor.redoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "put", file: "parts/frame/w.png", stash: "new" }]);
});

test("choosing no file changes nothing", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "");
  const { editor } = await ready(bridge, page);
  await select(editor, "frame");
  await editor.images.locator('.ed-cell[data-material="w"] .ed-cell-main').click();
  await page.waitForTimeout(200);
  expect(bridge.calledWith("Studio.ImportImage")).toEqual([]);
  await expect(editor.undoButton).toBeDisabled();
});

test("an image the app refuses is said and leaves nothing to undo", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "/art/not.png");
  bridge.fail("Studio.ImportImage", "That is not a PNG image. Export the layer as a PNG");
  const { editor } = await ready(bridge, page);
  await select(editor, "frame");
  await editor.images.locator('.ed-cell[data-material="gold"] .ed-cell-main').click();
  await expect(page.locator(".toast.err")).toContainText("That is not a PNG image");
  await expect(editor.undoButton).toBeDisabled();
});

test("what is worth knowing about an image is said as a warning", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "/art/big.png");
  bridge.reply("Studio.ImportImage", { rev: 1, inverse: [{ op: "drop", file: "parts/frame/gold.png" }], valid: true, diagnostics: [], id: "frame", warnings: ["This image is larger than the normal layout's canvas of 1000x1400, so part of it is cut off"] });
  const { editor } = await ready(bridge, page);
  await select(editor, "frame");
  await editor.images.locator('.ed-cell[data-material="gold"] .ed-cell-main').click();
  await expect(page.locator(".toast.warn")).toContainText("larger than the normal layout's canvas");
  await expect(editor.undoButton).toBeEnabled();
});

test("an image is removed with its file, and put back by undo", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await select(editor, "frame");
  bridge.reply("Studio.Apply", { rev: 1, inverse: [{ op: "put", file: "parts/frame/u.png", stash: "kept" }], valid: true, diagnostics: [] });
  await editor.images.locator('.ed-cell[data-material="u"] .ed-cell-remove').click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "drop", file: "parts/frame/u.png" }]);
  // Only an image has one to remove
  await expect(editor.images.locator('.ed-cell[data-material="gold"] .ed-cell-remove')).toHaveCount(0);
  await expect(editor.images.locator('.ed-cell[data-material="wu"] .ed-cell-remove')).toHaveCount(0);
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "put", file: "parts/frame/u.png", stash: "kept" }]);
});

test("a variant the part names itself is taken out with the image", async ({ bridge, page }) => {
  bridge.source.template.parts.frame.variants = { u: "parts/frame/u.png" };
  const { editor } = await ready(bridge, page);
  await select(editor, "frame");
  await editor.images.locator('.ed-cell[data-material="u"] .ed-cell-remove').click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([
    { op: "unset", kind: "part", key: "frame", path: ["variants", "u"] },
    { op: "drop", file: "parts/frame/u.png" },
  ]);
});

test("duplicate and delete are in a row's menu and act on that row", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  // The menu is for the row it is in, whatever is selected
  await select(editor, "oracle");
  await editor.choose("title", "Duplicate");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "duplicate", layout: "normal", kind: "text", key: "title", dx: 20, dy: 20, as: "title_copy" }]);
  await editor.choose("frame", "Delete");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "delete", layout: "normal", kind: "layer", key: "frame" }]);
  // The selected row is still selected, since another was deleted
  await expect(editor.selectionName).toHaveText("oracle");
});

test("deleting the selected row lets go of the selection", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await select(editor, "frame");
  await editor.choose("frame", "Delete");
  await expect(editor.selection).toBeHidden();
});

test("the selection panel no longer has the buttons that moved into the rows", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await select(editor, "title");
  for (const id of ["ed-raise", "ed-lower", "ed-lock", "ed-snap", "ed-duplicate", "ed-delete-item"]) await expect(page.locator(`#${id}`)).toHaveCount(0);
});

test("a copy is selected once it is drawn", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  bridge.reply("Studio.Describe", () => describeView([...standardItems, drawn("text", "title_copy", at(120, 100, 600, 80))]));
  await editor.choose("title", "Duplicate");
  await expect(editor.selectionName).toHaveText("title_copy");
});

test("a symbol has no menu to copy or delete it", async ({ bridge, page }) => {
  bridge.reply("Studio.Describe", () => describeView([...standardItems, drawn("symbol", "set", at(800, 600, 100, 60))]));
  const { editor } = await ready(bridge, page, { count: 8 });
  await expect(editor.moreButton("Set symbol")).toBeDisabled();
  await expect(editor.grip("Set symbol")).toHaveCount(0);
  await select(editor, "Set symbol");
  await expect(editor.selection).toBeVisible();
  // It can still be locked and made a line to snap to
  await expect(editor.lockToggle("Set symbol")).toBeEnabled();
});

test("a text box is added from the dialog, named for what it draws and placed in the middle", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await editor.addButton.click();
  await expect(editor.addDialog).toBeVisible();
  await expect(editor.addKind).toHaveValue("text");
  await editor.addDraws.selectOption("artist");
  bridge.reply("Studio.Describe", () => describeView([...standardItems, drawn("text", "artist", at(150, 644, 700, 112))]));
  await editor.addConfirm.click();
  await expect(editor.addDialog).toBeHidden();
  await expect.poll(() => applied(bridge).length).toBe(1);
  const op = applied(bridge)[0].ops[0];
  expect(op).toMatchObject({ op: "add", layout: "normal", kind: "text", key: "artist", rect: { x: 150, y: 644, width: 700, height: 112 } });
  expect(op.value.draws).toBe("artist");
  await expect(editor.selectionName).toHaveText("artist");
});

test("an art window goes above the layer chosen", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await editor.addButton.click();
  await editor.addKind.selectOption("art");
  await expect(editor.addAfter).toBeVisible();
  await expect(editor.addDraws).toBeHidden();
  await editor.addAfter.selectOption("background");
  await editor.addConfirm.click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  const op = applied(bridge)[0].ops[0];
  expect(op).toMatchObject({ op: "add", kind: "art", key: "1" });
  expect(op.value).toEqual({ after: "background" });
});

test("a layer is added for a part the template has, with an id when its name is taken", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await editor.addButton.click();
  await editor.addKind.selectOption("layer");
  await expect(editor.addPart).toBeVisible();
  await expect(editor.addPartName).toBeHidden();
  await editor.addPart.selectOption("frame");
  await editor.addConfirm.click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  const op = applied(bridge)[0].ops[0];
  expect(op).toMatchObject({ op: "add", kind: "layer", key: "frame-2", to: 2 });
  expect(op.value).toEqual({ id: "frame-2", part: "frame" });
  expect(bridge.calledWith("System.PickFile")).toEqual([]);
});

test("a layer for a new part takes a PNG for its first image, and is one step of undo", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "/art/glow.png");
  bridge.reply("Studio.ImportImage", { rev: 1, inverse: [{ op: "drop", file: "parts/glow/any.png" }, { op: "unset", kind: "template", path: ["parts", "glow"] }], valid: true, diagnostics: [], id: "glow", warnings: [] });
  const { editor } = await ready(bridge, page);
  await editor.addButton.click();
  await editor.addKind.selectOption("layer");
  await editor.addPart.selectOption({ label: "A new part from a PNG image" });
  await expect(editor.addPartName).toBeVisible();
  await editor.addSlot.selectOption("pinlines");
  await editor.addPartName.fill("glow");
  bridge.reply("Studio.Apply", { rev: 2, inverse: [{ op: "delete", layout: "normal", kind: "layer", key: "glow" }], valid: true, diagnostics: [] });
  await editor.addConfirm.click();
  await expect(editor.addDialog).toBeHidden();
  expect(bridge.calledWith("Studio.ImportImage")[0][0]).toEqual({ id: "mine", part: "glow", material: "any", slot: "pinlines", path: "/art/glow.png" });
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0]).toMatchObject({ op: "add", kind: "layer", key: "glow" });

  // One undo takes the layer and then the part and its image
  bridge.reply("Studio.Apply", { rev: 3, inverse: [], valid: true, diagnostics: [] });
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([
    { op: "delete", layout: "normal", kind: "layer", key: "glow" },
    { op: "drop", file: "parts/glow/any.png" },
    { op: "unset", kind: "template", path: ["parts", "glow"] },
  ]);
  await expect(editor.undoButton).toBeDisabled();
});

test("the dialog says what is wrong with a new part and stays open", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "");
  const { editor } = await ready(bridge, page);
  await editor.addButton.click();
  await editor.addKind.selectOption("layer");
  await editor.addPart.selectOption({ label: "A new part from a PNG image" });
  await editor.addPartName.fill("");
  await editor.addConfirm.click();
  await expect(editor.addError).toContainText("lowercase letters, digits, underscores and hyphens");
  await editor.addPartName.fill("frame");
  await editor.addConfirm.click();
  await expect(editor.addError).toContainText("has a part named frame already");
  await editor.addPartName.fill("fresh");
  await editor.addConfirm.click();
  // Cancelling the file chooser leaves the dialog as it was, with nothing said
  await expect(editor.addDialog).toBeVisible();
  await expect(editor.addError).toBeHidden();
  expect(bridge.calledWith("Studio.ImportImage")).toEqual([]);
  expect(applied(bridge)).toEqual([]);
});

test("an add that is refused leaves the dialog's work to be said and nothing recorded", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  bridge.fail("Studio.Apply", "That is not allowed: error: layouts.normal.stack[2].part: no part is named \"nope\"");
  await editor.addButton.click();
  await editor.addKind.selectOption("layer");
  await editor.addPart.selectOption("frame");
  await editor.addConfirm.click();
  await expect(page.locator(".toast.err")).toContainText("That is not allowed");
  await expect(editor.undoButton).toBeDisabled();
});

test("the add button is off for a layout that cannot be edited", async ({ bridge, page }) => {
  bridge.reply("Studio.Describe", () => describeView(standardItems, { editable: false, reason: "This layout extends another one" }));
  const { editor } = await ready(bridge, page);
  await expect(editor.addButton).toBeDisabled();
});

test("each row has a padlock that is faint when open and full when closed, and its icon changes", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  const lock = editor.lockToggle("title");
  await expect(lock).toHaveAttribute("aria-pressed", "false");
  await expect(lock.locator("use")).toHaveAttribute("href", "icons.svg#i-lock-open");
  const faint = await lock.evaluate((el) => getComputedStyle(el).color);
  await lock.click();
  await expect(lock).toHaveAttribute("aria-pressed", "true");
  await expect(lock.locator("use")).toHaveAttribute("href", "icons.svg#i-lock");
  const full = await lock.evaluate((el) => getComputedStyle(el).color);
  expect(full).not.toBe(faint);
  expect(full).toBe(await editor.row("title").locator(".ed-layer-name").evaluate((el) => getComputedStyle(el).color));
  // Locking a row that is not selected does not select it
  await expect(editor.selection).toBeHidden();
  await lock.click();
  await expect(lock).toHaveAttribute("aria-pressed", "false");
});

test("each row has a grid that is off and faint until the item is a line to snap to", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  const snap = editor.snapToggle("oracle");
  await expect(snap).toHaveAttribute("aria-pressed", "false");
  await expect(snap.locator("use")).toHaveAttribute("href", "icons.svg#i-grid-off");
  await snap.click();
  await expect(snap).toHaveAttribute("aria-pressed", "true");
  await expect(snap.locator("use")).toHaveAttribute("href", "icons.svg#i-grid-on");
  await expect.poll(() => bridge.calledWith("Studio.SetMarks").length).toBe(1);
  expect(bridge.calledWith("Studio.SetMarks")[0][0]).toEqual({ id: "mine", locked: [], snapTo: ["normal/text:oracle"] });
});

test("locking an item keeps the card from picking or moving it", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  await select(editor, "title");
  await expect(editor.lockToggle("title")).toHaveAttribute("aria-pressed", "false");
  await editor.lockToggle("title").click();
  await expect.poll(() => bridge.calledWith("Studio.SetMarks").length).toBe(1);
  expect(bridge.calledWith("Studio.SetMarks")[0][0]).toEqual({ id: "mine", locked: ["normal/text:title"], snapTo: [] });
  await expect(editor.lockToggle("title")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#ed-selection-kind")).toContainText("Locked");
  // It keeps its outline, with no handles, and is not moved by the arrow keys
  await expect(editor.selectionBox).toHaveClass(/locked/);
  await expect(editor.handles).toHaveCount(0);
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(400);
  expect(applied(bridge)).toEqual([]);

  // A click where it lies reaches what is beneath, and hovering shows that and not the title
  const p = point(150, 100);
  await page.mouse.move(p.x, p.y);
  await expect(page.locator("#ed-overlay .ed-box.hover")).toHaveCount(1);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("frame");

  // Its levers and the list still work, and unlocking gives it back to the card
  await select(editor, "title");
  await expect(editor.lockToggle("title")).toHaveAttribute("aria-pressed", "true");
  await editor.lockToggle("title").click();
  await expect.poll(() => bridge.calledWith("Studio.SetMarks").length).toBe(2);
  expect(bridge.calledWith("Studio.SetMarks")[1][0].locked).toEqual([]);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");
});

test("the L key locks and unlocks the selection", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await select(editor, "oracle");
  await page.keyboard.press("l");
  await expect(editor.lockToggle("oracle")).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("l");
  await expect(editor.lockToggle("oracle")).toHaveAttribute("aria-pressed", "false");
  expect(bridge.calledWith("Studio.SetMarks").map(([r]) => r.locked)).toEqual([["normal/text:oracle"], []]);
});

test("what was locked when the template was last open is locked again", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page, { meta: { locked: ["normal/text:title"], snapTo: ["normal/text:oracle"] } });
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("frame");
  await expect(editor.lockToggle("title")).toHaveAttribute("aria-pressed", "true");
  await expect(editor.snapToggle("oracle")).toHaveAttribute("aria-pressed", "true");
  await expect(editor.lockToggle("oracle")).toHaveAttribute("aria-pressed", "false");
  // Opening it did not write what it read
  expect(bridge.calledWith("Studio.SetMarks")).toEqual([]);
});

// drag presses at a place on the finished image and moves by a distance on screen
async function drag(page, from, by) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + by.x, from.y + by.y, { steps: 6 });
}

test("a drag snaps to the card's lines, and to an item only once it is marked", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  // The type line's left edge is at 100 as the title's is, and a pixel on screen is two and a half
  await drag(page, point(300, 810), { x: -3, y: 0 });
  await expect(editor.guides).toHaveCount(0);
  await expect(page.locator("#ed-overlay .ed-box.snap")).toHaveCount(0);
  await page.mouse.up();
  await expect.poll(() => applied(bridge).length).toBe(1);

  // Marked, the title is a line, and the guide names it
  await editor.snapToggle("title").click();
  await expect.poll(() => bridge.calledWith("Studio.SetMarks").length).toBe(1);
  expect(bridge.calledWith("Studio.SetMarks")[0][0]).toEqual({ id: "mine", locked: [], snapTo: ["normal/text:title"] });
  await drag(page, point(300, 810), { x: -3, y: 0 });
  await expect(editor.guides.first()).toContainText("left to title left");
  await expect(page.locator("#ed-overlay .ed-box.snap")).toHaveCount(1);
  await page.mouse.up();

  // It snapped back to the line it was on, which is no move, so nothing more was saved
  await page.waitForTimeout(300);
  expect(applied(bridge).length).toBe(1);
});

test("the fonts of the template are listed and a role is given one", async ({ bridge, page }) => {
  bridge.assets = assetsView(standardAssets().parts, { fonts: [{ id: "plantin-regular", file: "font/plantin-regular.ttf", family: "Plantin", style: "Regular" }] });
  const { editor } = await ready(bridge, page);
  await editor.openSection("about");
  await expect(editor.fonts).toHaveCount(1);
  await expect(editor.fonts.first()).toContainText("Plantin Regular");
  await expect(editor.roles).toHaveCount(6);
  const body = page.locator("#ed-role-body");
  await expect(body).toHaveValue("");
  await expect(body.locator("option").first()).toHaveText("Default (Built-in m15 body)");
  await expect(body.locator("option")).toHaveText(["Default (Built-in m15 body)", "Built-in m15 title", "Built-in m15 body", "Built-in m15 body italic", "Built-in m15 mana", "Built-in m15 small caps", "Built-in m15 info", "Plantin Regular"]);
  await body.selectOption("font:plantin-regular");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", kind: "template", path: ["roles", "body"], value: { font: "plantin-regular" } }]);
  await body.selectOption("");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "unset", kind: "template", path: ["roles", "body"] }]);
});

test("a template with no fonts says so", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await editor.openSection("about");
  await expect(editor.fonts).toHaveCount(0);
  await expect(page.locator("#ed-fonts")).toContainText("ships no fonts");
});

test("a font is added from a file, and undo takes it away", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "/fonts/Plantin.ttf");
  bridge.reply("Studio.ImportFont", { rev: 1, inverse: [{ op: "unset", kind: "template", path: ["fonts", "plantin-regular"] }, { op: "drop", file: "font/plantin-regular.ttf" }], valid: true, diagnostics: [], id: "plantin-regular", warnings: [] });
  const { editor } = await ready(bridge, page);
  await editor.openSection("about");
  bridge.assets = assetsView(standardAssets().parts, { fonts: [{ id: "plantin-regular", file: "font/plantin-regular.ttf", family: "Plantin", style: "Regular" }] });
  await editor.fontAdd.click();
  await expect.poll(() => bridge.calledWith("Studio.ImportFont").length).toBe(1);
  expect(bridge.calledWith("Studio.ImportFont")[0][0]).toEqual({ id: "mine", path: "/fonts/Plantin.ttf", assign: [] });
  await expect(editor.fonts).toHaveCount(1);
  await expect(page.locator(".toast")).toContainText("Choose where it is used");
  await expect(editor.undoButton).toBeEnabled();

  bridge.assets = standardAssets();
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([
    { op: "unset", kind: "template", path: ["fonts", "plantin-regular"] },
    { op: "drop", file: "font/plantin-regular.ttf" },
  ]);
  await expect(editor.fonts).toHaveCount(0);
});

test("removing a font frees the roles that use it, in one step", async ({ bridge, page }) => {
  const roles = assetsView([]).roles.map((r) => (r.role === "title" || r.role === "info" ? { ...r, font: "plantin-regular" } : r));
  bridge.assets = assetsView(standardAssets().parts, { roles, fonts: [{ id: "plantin-regular", file: "font/plantin-regular.ttf", family: "Plantin", style: "Regular" }] });
  const { editor } = await ready(bridge, page);
  await editor.openSection("about");
  await expect(page.locator("#ed-role-title")).toHaveValue("font:plantin-regular");
  await expect(page.locator("#ed-role-body")).toHaveValue("");
  await editor.fonts.first().getByRole("button", { name: /Remove the font Plantin Regular/ }).click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([
    { op: "unset", kind: "template", path: ["roles", "title"] },
    { op: "unset", kind: "template", path: ["roles", "info"] },
    { op: "unset", kind: "template", path: ["fonts", "plantin-regular"] },
    { op: "drop", file: "font/plantin-regular.ttf" },
  ]);
});

test("a font the app refuses is said and changes nothing", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "/fonts/broken.ttf");
  bridge.fail("Studio.ImportFont", "That is not a font the engine can read");
  const { editor } = await ready(bridge, page);
  await editor.openSection("about");
  await editor.fontAdd.click();
  await expect(page.locator(".toast.err")).toContainText("not a font the engine can read");
  await expect(editor.undoButton).toBeDisabled();
});

test("a font's license is kept with it and cleared by emptying the field", async ({ bridge, page }) => {
  bridge.assets = assetsView(standardAssets().parts, { fonts: [{ id: "plantin-regular", file: "font/plantin-regular.ttf", family: "Plantin", style: "Regular", license: "OFL 1.1" }] });
  const { editor } = await ready(bridge, page);
  await editor.openSection("about");
  const license = page.getByLabel("License of the font Plantin Regular");
  await expect(license).toHaveValue("OFL 1.1");
  await license.fill("SIL OFL 1.1");
  await license.blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", kind: "template", path: ["fonts", "plantin-regular", "license"], value: "SIL OFL 1.1" }]);
  await license.fill("");
  await license.blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "unset", kind: "template", path: ["fonts", "plantin-regular", "license"] }]);
  await expect(editor.undoButton).toBeEnabled();
});

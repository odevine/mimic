import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { test, expect } from "../fixtures.mjs";

// Drafts are the app's own files, so these tests read and change them where the
// scratch folder keeps them
const draftFile = (scratch, id, ...parts) => path.join(scratch.home, "mimic", "drafts", id, ...parts);

test("with no template of its own the editor offers to make one", async ({ editor }) => {
  await expect(editor.empty).toBeVisible();
  await expect(editor.workspace).toBeHidden();
  await expect(editor.page.locator("#ed-empty-new")).toBeVisible();
});

test("a new template is drawn with a test card as soon as it is made", async ({ editor }) => {
  await editor.create("Test frame", 150);

  await expect(editor.workspace).toBeVisible();
  await expect(editor.aboutName).toHaveText("Test frame");
  await expect(editor.aboutId).toHaveText("user.test-frame");
  await expect(editor.layouts).toHaveText(["normal · 408 × 555 px · 150 dpi"]);
  await expect(editor.problems).toHaveCount(0);
  await expect(editor.page.locator("#ed-problems-none")).toBeVisible();
  await expect(editor.cardLine).toHaveText("Dawnwatch Sentinel · TST");
  await expect(editor.preview).toBeVisible();
  await expect(editor.page.locator("#ed-activity-title")).toHaveText("Drew Dawnwatch Sentinel");
  const size = await editor.preview.evaluate((img) => [img.naturalWidth, img.naturalHeight]);
  expect(size).toEqual([408, 555]);
});

test("the dialog says what is wrong before it makes anything", async ({ editor }) => {
  await editor.openNewDialog();
  await expect(editor.dialogDpi).toHaveValue("800");
  await expect(editor.page.locator("#draft-dpi-size")).toHaveText("A canvas of 2176 × 2960 pixels");

  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toHaveText("Give the template a name");

  await editor.dialogName.fill("Frame");
  await editor.dialogDpi.fill("5000");
  await expect(editor.page.locator("#draft-dpi-size")).toContainText("72 to 1200");
  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toContainText("72 to 1200");
  await expect(editor.dialog).toBeVisible();

  await editor.page.keyboard.press("Escape");
  await expect(editor.dialog).toBeHidden();
  await expect(editor.empty).toBeVisible();
});

test("a preset or a searched card is drawn and remembered", async ({ editor, page }) => {
  await editor.create("Cards", 100);

  await editor.preset("Land").click();
  await expect(editor.cardLine).toHaveText("Mistral Shore · TST");
  await expect(editor.preset("Land")).toHaveAttribute("aria-pressed", "true");
  await editor.waitForPreview();
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Mistral Shore");

  await editor.search("llanowar elves");
  await editor.results.filter({ has: page.locator(".r-name", { hasText: "Llanowar Elves" }) }).first().click();
  await expect(editor.cardLine).toContainText("Llanowar Elves");
  await expect(editor.preset("Land")).toHaveAttribute("aria-pressed", "false");
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Llanowar Elves");

  // The card is kept with the template, so the page finds it again after a reload
  await page.reload();
  await page.locator("html.booted").waitFor();
  await page.locator('button[data-mode="editor"]').click();
  await expect(editor.cardLine).toContainText("Llanowar Elves");
  await editor.waitForPreview();
});

test("a copy of an installed template is drawn and says where it began", async ({ editor, templates, page }) => {
  await templates.show();
  const entry = templates.entry("Test frame");
  await entry.getByRole("button", { name: "Edit a copy" }).click();
  await expect(editor.dialogName).toHaveValue("Test frame copy");
  await expect(editor.page.locator("#draft-dpi-row")).toBeHidden();
  await editor.dialogName.fill("My copy");
  await editor.dialogConfirm.click();

  await expect(editor.page.locator("#mode-editor")).toBeVisible();
  await editor.waitForPreview();
  await expect(editor.aboutName).toHaveText("My copy");
  await expect(editor.aboutId).toHaveText("user.my-copy");
  await expect(editor.aboutFork).toHaveText("Copy of mimic.m15");
  await expect(editor.problems).toHaveCount(0);
  await expect(page.locator("#draft-dialog")).toBeHidden();
});

test("renaming changes the name and not the id", async ({ editor }) => {
  await editor.create("First", 100);
  await editor.page.locator("#ed-rename").click();
  await expect(editor.dialogName).toHaveValue("First");
  await editor.dialogName.fill("Second");
  await editor.dialogConfirm.click();
  await expect(editor.dialog).toBeHidden();
  await expect(editor.aboutName).toHaveText("Second");
  await expect(editor.aboutId).toHaveText("user.first");
  await expect(editor.picker.locator("option:checked")).toHaveText("Second");
});

test("deleting asks first, and a no leaves the template alone", async ({ editor, scratch }) => {
  await editor.create("Keep", 100);
  await scratch.decline();
  await editor.page.locator("#ed-delete").click();
  await expect(editor.workspace).toBeVisible();
  await expect(editor.aboutName).toHaveText("Keep");

  await scratch.reset();
  await editor.page.locator("#ed-delete").click();
  await expect(editor.empty).toBeVisible();
});

test("two templates can be switched between", async ({ editor }) => {
  await editor.create("One", 100);
  await editor.create("Two", 100);
  await expect(editor.picker.locator("option")).toHaveCount(2);
  await expect(editor.aboutName).toHaveText("Two");
  await editor.picker.selectOption({ label: "One" });
  await expect(editor.aboutName).toHaveText("One");
  await editor.waitForPreview();
});

test("a template that does not load says why and draws nothing", async ({ editor, scratch }) => {
  await editor.create("Broken", 100);
  await writeFile(draftFile(scratch, "broken", "layouts", "normal.json"), "{");
  // Opening it again reads the files as they are
  await editor.page.reload();
  await editor.page.locator("html.booted").waitFor();
  await editor.page.locator('button[data-mode="editor"]').click();

  await expect(editor.problems.first()).toContainText("normal");
  await expect(editor.page.locator("#ed-activity-title")).toHaveText("This template does not load");
  await expect(editor.activity).toHaveAttribute("data-state", "error");
  await expect(editor.preview).toBeHidden();
  await expect(editor.layouts).toHaveCount(0);
});

test("the editor shows what is not built yet as locked", async ({ editor }) => {
  await editor.create("Gated", 100);
  await expect(editor.page.locator("#ed-use")).toHaveAttribute("aria-disabled", "true");
  await expect(editor.page.locator("#ed-export")).toHaveAttribute("aria-disabled", "true");
  await expect(editor.page.locator("#ed-json")).toHaveAttribute("aria-disabled", "true");
  await expect(editor.page.locator("#ed-use")).toHaveAttribute("data-gate-reason", /#115/);
});

// The starter's canvas at 100 dpi is 272 pixels across, and the preview is drawn at
// that size, so the overlay's box tells how many pixels on screen make one of the card's
const CARD = { width: 272, height: 370 };

async function titleBox(editor) {
  const box = await editor.overlay.boundingBox();
  await editor.layers.filter({ hasText: "title" }).click();
  await expect(editor.selectionName).toHaveText("title");
  return { box, x: Number(await editor.page.locator("#ed-sel-x").textContent()) };
}

test("dragging a text box on the card saves the layout and draws the card again", async ({ editor, scratch, page }) => {
  await editor.create("Edits", 100);
  await expect(editor.layers.first()).toBeVisible();
  const file = draftFile(scratch, "edits", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  const src = await editor.preview.getAttribute("src");

  // Pick the title where it is on the card, not from the list
  const box = await editor.overlay.boundingBox();
  await editor.layers.filter({ hasText: "title" }).click();
  const x = Number(await page.locator("#ed-sel-x").textContent());
  const y = Number(await page.locator("#ed-sel-y").textContent());
  const w = Number(await page.locator("#ed-sel-w").textContent());
  const h = Number(await page.locator("#ed-sel-h").textContent());
  await page.keyboard.press("Escape");
  await expect(editor.selection).toBeHidden();
  const from = { x: box.x + ((x + w / 2) / CARD.width) * box.width, y: box.y + ((y + h / 2) / CARD.height) * box.height };
  await page.mouse.click(from.x, from.y);
  await expect(editor.selectionName).toHaveText("title");

  const per = CARD.width / box.width;
  await page.keyboard.down("Alt");
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + 30, from.y, { steps: 5 });
  await page.mouse.up();
  await page.keyboard.up("Alt");
  const moved = Math.round(30 * per);

  await expect(page.locator("#ed-sel-x")).toHaveText(String(x + moved));
  const edited = await readFile(file, "utf8");
  expect(edited).not.toBe(original);
  // Only the title's x changed, and the file kept its order and layout
  const was = JSON.parse(original);
  const now = JSON.parse(edited);
  expect(Object.keys(now)).toEqual(Object.keys(was));
  was.textBoxes.title.rect.x += moved;
  expect(now).toEqual(was);
  expect(edited.split("\n").length).toBe(original.split("\n").length);

  // The card is drawn again with the title in its new place
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(src);
  await editor.waitForPreview();

  // Undo puts the file back as it was, byte for byte
  await editor.undoButton.click();
  await expect(page.locator("#ed-sel-x")).toHaveText(String(x));
  expect(await readFile(file, "utf8")).toBe(original);
  await expect(editor.undoButton).toBeDisabled();
  await editor.redoButton.click();
  await expect(page.locator("#ed-sel-x")).toHaveText(String(x + moved));
  expect(await readFile(file, "utf8")).toBe(edited);
});

test("arrow keys nudge a selection and the keyboard undoes the run", async ({ editor, scratch, page }) => {
  await editor.create("Nudge", 100);
  const file = draftFile(scratch, "nudge", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  const { x } = await titleBox(editor);

  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("Shift+ArrowDown");
  await expect(editor.undoButton).toBeEnabled();
  await expect(page.locator("#ed-sel-x")).toHaveText(String(x + 2));
  expect(JSON.parse(await readFile(file, "utf8")).textBoxes.title.rect.x).toBe(JSON.parse(original).textBoxes.title.rect.x + 2);

  await page.keyboard.press("ControlOrMeta+z");
  await expect(page.locator("#ed-sel-x")).toHaveText(String(x));
  expect(await readFile(file, "utf8")).toBe(original);
});

test("a layer is raised in the stack and the layout keeps its other keys", async ({ editor, scratch, page }) => {
  await editor.create("Stack", 100);
  const file = draftFile(scratch, "stack", "layouts", "normal.json");
  const original = JSON.parse(await readFile(file, "utf8"));
  // The starter has one layer, so a second is added to have two to order
  const next = structuredClone(original);
  next.stack.push({ id: "mark", part: "background" });
  await writeFile(file, JSON.stringify(next, null, 2) + "\n");
  await editor.page.reload();
  await editor.page.locator("html.booted").waitFor();
  await editor.page.locator('button[data-mode="editor"]').click();
  await editor.waitForPreview();

  await editor.layers.filter({ hasText: "background" }).click();
  await page.locator("#ed-raise").click();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).stack.map((e) => e.id)).toEqual(["mark", "background"]);
  const after = JSON.parse(await readFile(file, "utf8"));
  expect(after.textBoxes).toEqual(original.textBoxes);
});

test("a layout that builds on another can be looked at and not changed", async ({ editor, scratch, page }) => {
  await editor.create("Built", 100);
  const dir = path.join(scratch.home, "mimic", "drafts", "built", "layouts");
  const raw = await readFile(path.join(dir, "normal.json"), "utf8");
  await writeFile(path.join(dir, "base.json"), raw.replace('"role": "single"', '"role": "split"'));
  await writeFile(path.join(dir, "normal.json"), JSON.stringify({ serves: [{ role: "single", kind: "standard" }], extends: ["base"] }));
  await page.reload();
  await page.locator("html.booted").waitFor();
  await page.locator('button[data-mode="editor"]').click();
  await editor.waitForPreview();

  await expect(page.locator("#ed-edit-note")).toContainText("extends another one");
  await editor.layers.filter({ hasText: "title" }).click();
  await expect(editor.selectionName).toHaveText("title");
  await expect(editor.handles).toHaveCount(0);
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(500);
  await expect(editor.undoButton).toBeDisabled();
  expect(JSON.parse(await readFile(path.join(dir, "normal.json"), "utf8")).extends).toEqual(["base"]);
});

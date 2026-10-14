import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { test, expect } from "../fixtures.mjs";
import { solidPng } from "../png.mjs";

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
  // The problems are collapsed to their heading until they are asked for
  await editor.openSection("problems");
  await expect(editor.page.locator("#ed-problems-none")).toBeVisible();
  await expect(editor.cardLine).toHaveText("Mirran Spy · MBS");
  await expect(editor.preview).toBeVisible();
  await expect(editor.page.locator("#ed-activity-title")).toHaveText("Drew Mirran Spy");
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
  await expect(editor.cardLine).toHaveText("Flooded Strand · KTK");
  await expect(editor.preset("Land")).toHaveAttribute("aria-pressed", "true");
  await editor.waitForPreview();
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Flooded Strand");

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
  // The JSON editor is built, so it is not locked
  await expect(editor.page.locator("#ed-json")).not.toHaveAttribute("aria-disabled", "true");
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

  await editor.choose("background", "Move up");
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

async function selectTitle(editor) {
  await expect(editor.layers.first()).toBeVisible();
  await editor.layers.filter({ hasText: "title" }).click();
  await expect(editor.selectionName).toHaveText("title");
}

test("a lever changes one field of the file, the card is drawn with it, and undo restores the file", async ({ editor, scratch, page }) => {
  await editor.create("Levers", 100);
  const file = draftFile(scratch, "levers", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  const src = await editor.preview.getAttribute("src");
  await selectTitle(editor);

  const was = JSON.parse(original).textBoxes.title.text.fontSize;
  await editor.lever("lever-fontSize").fill(String(was + 4));
  await editor.lever("lever-fontSize").blur();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).textBoxes.title.text.fontSize).toBe(was + 4);

  // Only that number changed, and the file kept its order and layout
  const after = await readFile(file, "utf8");
  const edited = JSON.parse(original);
  edited.textBoxes.title.text.fontSize = was + 4;
  expect(JSON.parse(after)).toEqual(edited);
  expect(Object.keys(JSON.parse(after).textBoxes)).toEqual(Object.keys(JSON.parse(original).textBoxes));
  expect(after.split("\n").length).toBe(original.split("\n").length);
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(src);
  await editor.waitForPreview();

  await editor.undoButton.click();
  await expect(editor.lever("lever-fontSize")).toHaveValue(String(was));
  expect(await readFile(file, "utf8")).toBe(original);
});

test("a setting the engine does not accept is refused with its reason and the file is left alone", async ({ editor, scratch, page }) => {
  await editor.create("Refuse", 100);
  const file = draftFile(scratch, "refuse", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  await selectTitle(editor);

  await editor.lever("lever-condition").fill("made_up");
  await editor.lever("lever-condition").blur();
  await expect(page.locator("#toasts")).toContainText("not allowed");
  await expect(page.locator("#toasts")).toContainText("made_up");
  await expect(editor.lever("lever-condition")).toHaveValue("");
  expect(await readFile(file, "utf8")).toBe(original);
  await expect(editor.undoButton).toBeDisabled();

  // A condition the engine knows is let through
  await editor.lever("lever-condition").fill("creature");
  await editor.lever("lever-condition").blur();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).textBoxes.title.condition).toBe("creature");
});

test("the shadow, a layer's blend and the canvas are edited from their levers", async ({ editor, scratch, page }) => {
  await editor.create("More", 100);
  const file = draftFile(scratch, "more", "layouts", "normal.json");
  await expect(editor.layers.first()).toBeVisible();

  await editor.layers.filter({ hasText: "mana" }).first().click();
  const hadShadow = !!JSON.parse(await readFile(file, "utf8")).textBoxes.mana.text.shadow;
  expect(hadShadow).toBe(true);
  await page.locator("#lever-shadow-on").uncheck();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).textBoxes.mana.text.shadow).toBeUndefined();
  await page.locator("#lever-shadow-on").check();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).textBoxes.mana.text.shadow).toEqual({ distance: 0.08, angle: 25 });

  await editor.layers.filter({ hasText: "background" }).click();
  await editor.lever("lever-mode").selectOption("multiply");
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).stack[0].mode).toBe("multiply");

  await editor.openCanvas();
  await editor.lever("canvas-dpi").fill("90");
  await editor.lever("canvas-dpi").blur();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).canvas.dpi).toBe(90);
  await expect(editor.layouts.first()).toContainText("90 dpi");
});

test("the template's name is changed from its details and the picker follows", async ({ editor, scratch, page }) => {
  await editor.create("Named", 100);
  await editor.openSection("about");
  await editor.lever("template-name").fill("Better name");
  await editor.lever("template-name").blur();
  const file = path.join(scratch.home, "mimic", "drafts", "named", "template.json");
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).name).toBe("Better name");
  await expect(editor.aboutName).toHaveText("Better name");
  await expect(editor.picker.locator("option:checked")).toHaveText("Better name");

  await page.locator("#ed-credit-add").click();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).credits).toEqual([{ name: "New credit" }]);
  await page.locator("#ed-credits .ed-credit input").first().fill("Someone");
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).credits).toEqual([{ name: "Someone" }]);
});

test("a copy keeps the credits and license of the original out of reach of the levers", async ({ editor, templates, page }) => {
  await templates.show();
  await templates.entry("Test frame").getByRole("button", { name: "Edit a copy" }).click();
  await editor.dialogName.fill("Kept");
  await editor.dialogConfirm.click();
  await editor.waitForPreview();
  await editor.openSection("about");
  const credits = page.locator("#ed-credits .ed-credit");
  // The copy names the original in a credit that cannot be removed
  await expect(credits.first()).toContainText("Kept from the template this was copied from");
  await expect(credits.first().getByRole("button")).toHaveCount(0);
  await expect(credits.first().locator("input").nth(2)).toHaveValue(/Forked from mimic\.m15/);
});

test("the JSON editor saves a change to a layout and shows a mistake with its line", async ({ editor, scratch, page }) => {
  await editor.create("Json", 100);
  const file = draftFile(scratch, "json", "layouts", "normal.json");
  await expect(editor.layers.first()).toBeVisible();
  await editor.jsonButton.click();
  await editor.jsonTabs.filter({ hasText: "normal" }).click();
  const text = await editor.jsonText.inputValue();
  expect(text).toBe(await readFile(file, "utf8"));

  // A change that is valid is saved as typed
  const edited = text.replace(/"dpi": \d+/, '"dpi": 95');
  await editor.jsonText.fill(edited);
  await expect.poll(async () => await readFile(file, "utf8")).toBe(edited);
  await expect(editor.jsonStatus).toHaveText("Saved");
  await expect(editor.layouts.first()).toContainText("95 dpi");

  // A mistake in the JSON is shown with the line, and the file is left as it was
  const broken = edited.replace('"serves": [', '"serves": ,[');
  await editor.jsonText.fill(broken);
  await expect(editor.jsonProblems.first()).toContainText(/^Line \d+, column \d+:/);
  await expect(editor.jsonStatus).toHaveText("Not saved, fix the problems first");
  expect(await readFile(file, "utf8")).toBe(edited);

  // Valid JSON that is not a layout that loads is not saved over one that does
  const wrong = edited.replace('"align": "left"', '"align": "diagonal"');
  await editor.jsonText.fill(wrong);
  await expect(editor.jsonProblems.first()).toContainText("align");
  await expect(editor.jsonStatus).toHaveText("Not saved, fix the problems first");
  expect(await readFile(file, "utf8")).toBe(edited);
});

test("a template that does not load can be fixed in the JSON editor", async ({ editor, scratch, page }) => {
  await editor.create("Fixme", 100);
  const file = draftFile(scratch, "fixme", "layouts", "normal.json");
  const good = await readFile(file, "utf8");
  await writeFile(file, good.replace('"align": "left"', '"align": "diagonal"'));
  await page.reload();
  await page.locator("html.booted").waitFor();
  await page.locator('button[data-mode="editor"]').click();
  await expect(editor.problems.first()).toContainText("align");
  await expect(editor.activity).toHaveAttribute("data-state", "error");

  await editor.jsonButton.click();
  await editor.jsonTabs.filter({ hasText: "normal" }).click();
  await editor.jsonText.fill(good);
  await expect.poll(async () => await readFile(file, "utf8")).toBe(good);
  await expect(editor.problems).toHaveCount(0);
  await editor.waitForPreview();
});

// previewPixel is a pixel of the card as the page shows it
const previewPixel = (editor, x, y) =>
  editor.preview.evaluate(
    (img, [px, py]) => {
      const c = document.createElement("canvas");
      c.width = img.naturalWidth;
      c.height = img.naturalHeight;
      const ctx = c.getContext("2d");
      ctx.drawImage(img, 0, 0);
      return [...ctx.getImageData(px, py, 1, 1).data];
    },
    [x, y],
  );

test("an image brought in replaces the layer's and the card is drawn with it, and undo and redo bring each back", async ({ editor, scratch, page }) => {
  await editor.create("Images", 100);
  const file = draftFile(scratch, "images", "parts", "background", "any.png");
  const original = await readFile(file);
  await editor.layers.filter({ hasText: "background" }).click();
  await expect(editor.images.locator("h4")).toHaveText("Images of background");
  await expect(editor.cells).toHaveCount(1);
  await expect(editor.images.locator(".ed-cell-thumb img")).toHaveCount(1);

  const red = solidPng(272, 370, [200, 20, 20, 255]);
  await scratch.offer("red.png", red);
  const src = await editor.preview.getAttribute("src");
  await editor.cells.first().locator(".ed-cell-main").click();
  await expect.poll(async () => (await readFile(file)).equals(red)).toBe(true);
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(src);
  await editor.waitForPreview();
  const [r, g] = await previewPixel(editor, 2, 2);
  expect(r).toBeGreaterThan(150);
  expect(g).toBeLessThan(60);

  const drawnRed = await editor.preview.getAttribute("src");
  await editor.undoButton.click();
  await expect.poll(async () => (await readFile(file)).equals(original)).toBe(true);
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(drawnRed);
  await editor.waitForPreview();
  const [r2, g2] = await previewPixel(editor, 2, 2);
  expect(r2).toBeGreaterThan(200);
  expect(g2).toBeGreaterThan(150);
  const drawnOriginal = await editor.preview.getAttribute("src");
  await editor.redoButton.click();
  await expect.poll(async () => (await readFile(file)).equals(red)).toBe(true);
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(drawnOriginal);
});

test("an image that is not a PNG is refused with the reason and the template is left alone", async ({ editor, scratch, page }) => {
  await editor.create("Refuse image", 100);
  const file = draftFile(scratch, "refuse-image", "parts", "background", "any.png");
  const original = await readFile(file);
  await editor.layers.filter({ hasText: "background" }).click();
  await scratch.offer("fake.png", "this is not an image");
  await editor.cells.first().locator(".ed-cell-main").click();
  await expect(page.locator("#toasts")).toContainText("not a PNG");
  expect((await readFile(file)).equals(original)).toBe(true);
  await expect(editor.undoButton).toBeDisabled();
});

test("a layer for a new part makes the part and its folder, and one undo takes both away", async ({ editor, scratch, page }) => {
  await editor.create("New layer", 100);
  const dir = draftFile(scratch, "new-layer");
  const template = JSON.parse(await readFile(path.join(dir, "template.json"), "utf8"));
  const layout = await readFile(path.join(dir, "layouts", "normal.json"), "utf8");
  await scratch.offer("glow.png", solidPng(272, 370, [10, 200, 10, 90]));

  await editor.addButton.click();
  await editor.addKind.selectOption("layer");
  await editor.addPart.selectOption({ label: "A new part from a PNG image" });
  await editor.addPartName.fill("glow");
  await editor.addConfirm.click();
  await expect(editor.addDialog).toBeHidden();
  await expect(editor.layers.filter({ hasText: "glow" })).toHaveCount(1);
  await expect(editor.selectionName).toHaveText("glow");
  await expect(editor.images.locator("h4")).toHaveText("Images of glow");

  const after = JSON.parse(await readFile(path.join(dir, "template.json"), "utf8"));
  expect(after.parts.glow).toEqual({ dir: "parts/glow" });
  expect((await readFile(path.join(dir, "parts", "glow", "any.png"))).length).toBeGreaterThan(0);
  const stack = JSON.parse(await readFile(path.join(dir, "layouts", "normal.json"), "utf8")).stack;
  expect(stack.map((e) => e.id || e.part)).toEqual(["background", "glow"]);
  await editor.waitForPreview();
  await expect(editor.problems).toHaveCount(0);

  await editor.undoButton.click();
  await expect(editor.layers.filter({ hasText: "glow" })).toHaveCount(0);
  expect(await readFile(path.join(dir, "layouts", "normal.json"), "utf8")).toBe(layout);
  expect(JSON.parse(await readFile(path.join(dir, "template.json"), "utf8")).parts).toEqual(template.parts);
  await expect.poll(async () => readFile(path.join(dir, "parts", "glow", "any.png")).then(() => true, () => false)).toBe(false);
  await expect(editor.undoButton).toBeDisabled();
});

test("a text box is added, copied and deleted, and undoing all of it restores the layout as it was", async ({ editor, scratch, page }) => {
  await editor.create("Boxes", 100);
  const file = draftFile(scratch, "boxes", "layouts", "normal.json");
  const original = await readFile(file, "utf8");

  await editor.addButton.click();
  await editor.addDraws.selectOption("artist");
  await editor.addConfirm.click();
  await expect(editor.addDialog).toBeHidden();
  await expect.poll(async () => Object.keys(JSON.parse(await readFile(file, "utf8")).textBoxes)).toContain("artist_2");
  await expect(editor.selectionName).toHaveText("artist_2");

  await editor.choose("oracle", "Duplicate");
  await expect(editor.selectionName).toHaveText("oracle_copy");
  const boxes = JSON.parse(await readFile(file, "utf8")).textBoxes;
  expect(boxes["oracle_copy"].draws).toBe("oracle");
  expect(boxes["oracle_copy"].rect.x).toBe(boxes.oracle.rect.x + 8);

  await editor.choose("oracle_copy", "Delete");
  await expect(editor.selection).toBeHidden();
  await expect.poll(async () => Object.keys(JSON.parse(await readFile(file, "utf8")).textBoxes)).not.toContain("oracle_copy");

  for (let i = 0; i < 3; i++) await editor.undoButton.click();
  await expect(editor.undoButton).toBeDisabled();
  // The undos are made one after another, so the file is read until it is as it was
  await expect.poll(() => readFile(file, "utf8")).toBe(original);
});

test("a font is brought in, given to a role and the card is still drawn", async ({ editor, scratch, page }) => {
  await editor.create("Fonts", 100);
  const dir = draftFile(scratch, "fonts");
  await scratch.offer("Plantin.ttf", { path: path.resolve(import.meta.dirname, "../../../engine/fonts/embedded/m15/plantin-body.ttf") });
  await editor.openSection("about");
  await editor.fontAdd.click();
  await expect(editor.fonts).toHaveCount(1);
  const added = JSON.parse(await readFile(path.join(dir, "template.json"), "utf8"));
  const [id, entry] = Object.entries(added.fonts)[0];
  expect(entry.file).toBe(`font/${id}.ttf`);
  expect(entry.sha256).toHaveLength(64);
  expect((await readFile(path.join(dir, entry.file))).length).toBeGreaterThan(1000);

  await page.getByLabel(/License of the font/).fill("SIL OFL 1.1");
  await page.getByLabel(/License of the font/).blur();
  await expect.poll(async () => JSON.parse(await readFile(path.join(dir, "template.json"), "utf8")).fonts[id].license).toBe("SIL OFL 1.1");

  await page.locator("#ed-role-body").selectOption(`font:${id}`);
  await expect.poll(async () => JSON.parse(await readFile(path.join(dir, "template.json"), "utf8")).roles?.body?.font).toBe(id);
  await editor.waitForPreview();
  await expect(editor.problems).toHaveCount(0);

  // Removing the font frees the role first, so the template still loads
  await editor.fonts.first().getByRole("button", { name: /Remove the font/ }).click();
  await expect(editor.fonts).toHaveCount(0);
  const gone = JSON.parse(await readFile(path.join(dir, "template.json"), "utf8"));
  expect(gone.fonts ?? {}).toEqual({});
  expect(gone.roles?.body).toBeUndefined();
  await expect(editor.problems).toHaveCount(0);
});

test("what is locked and what is snapped to is kept with the template across a reload", async ({ editor, scratch, page }) => {
  await editor.create("Marks", 100);
  await editor.lockToggle("title").click();
  await editor.snapToggle("oracle").click();
  const meta = draftFile(scratch, "marks", ".studio.json");
  await expect.poll(async () => JSON.parse(await readFile(meta, "utf8")).locked).toEqual(["normal/text:title"]);
  await expect.poll(async () => JSON.parse(await readFile(meta, "utf8")).snapTo).toEqual(["normal/text:oracle"]);

  await page.reload();
  await page.locator("html.booted").waitFor();
  await page.locator('button[data-mode="editor"]').click();
  await editor.waitForPreview();
  await expect(editor.lockToggle("title")).toHaveAttribute("aria-pressed", "true");
  await expect(editor.snapToggle("oracle")).toHaveAttribute("aria-pressed", "true");
  await expect(editor.lockToggle("oracle")).toHaveAttribute("aria-pressed", "false");
});

test("changing the test card's fields changes what the template draws, and the changes are kept", async ({ editor, page }) => {
  await editor.create("Fine tune", 100);
  // The first preset is a creature, so its power and toughness box is drawn
  await expect(editor.row("pt")).not.toHaveClass(/undrawn/);
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "true");
  const src = await editor.preview.getAttribute("src");

  await editor.openFine();
  await editor.cardField("typeLine").fill("Land");
  await editor.cardField("power").fill("");
  await editor.cardField("toughness").fill("");
  await editor.cardField("name").fill("My Test Land");
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "false");
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew My Test Land");
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(src);
  // A land has no power or toughness, so the box has nothing to draw and says why
  await expect(editor.row("pt")).toHaveClass(/undrawn/);
  await expect(editor.row("pt")).toHaveAttribute("title", /no pt|pt to draw/);

  // The card as changed is the one the template opens with next time
  await page.reload();
  await page.locator("html.booted").waitFor();
  await page.locator('button[data-mode="editor"]').click();
  await editor.waitForPreview();
  await expect(editor.cardLine).toHaveText("My Test Land · MBS");
  await editor.openFine();
  await expect(editor.cardField("typeLine")).toHaveValue("Land");
  await expect(editor.row("pt")).toHaveClass(/undrawn/);
  await expect(editor.presets.filter({ has: page.locator("xpath=self::*[@aria-pressed='true']") })).toHaveCount(0);

  // Choosing a preset again starts the fields over
  await editor.preset("Monocolored").click();
  await expect(editor.cardField("typeLine")).toHaveValue("Creature — Human Rogue");
  await expect(editor.row("pt")).not.toHaveClass(/undrawn/);
});

test("a searched card's artwork replaces the preset's in the art window, and an edit keeps it", async ({ editor, page }) => {
  await editor.create("Art window", 100);
  // A preset is a real card, so it comes with art and the window shows it
  await expect(editor.row("Art window")).not.toHaveClass(/undrawn/);
  const pixel = () =>
    editor.preview.evaluate((img) => {
      const c = document.createElement("canvas");
      c.width = img.naturalWidth;
      c.height = img.naturalHeight;
      const ctx = c.getContext("2d");
      ctx.drawImage(img, 0, 0);
      const at = (fx, fy) => [...ctx.getImageData(Math.round(fx * c.width), Math.round(fy * c.height), 1, 1).data].slice(0, 3);
      return { art: at(0.5, 0.35), edge: at(0.03, 0.03) };
    });
  const preset = await pixel();
  expect(preset.art).not.toEqual(preset.edge);

  await editor.search("llanowar elves");
  await editor.results.filter({ has: page.locator(".r-name", { hasText: "Llanowar Elves" }) }).first().click();
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Llanowar Elves");
  await expect(page.locator("#ed-activity")).toHaveAttribute("data-state", "done");
  await expect(editor.row("Art window")).not.toHaveClass(/undrawn/);
  // The art is the searched card's and not the preset's, which the fake draws differently
  const found = await pixel();
  expect(found.art).not.toEqual(found.edge);
  expect(found.art).not.toEqual(preset.art);

  // Changing a field of the card keeps the art in the window
  await editor.openFine();
  await editor.cardField("flavor").fill("Roots first.");
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Llanowar Elves");
  await expect.poll(async () => (await pixel()).art.join(",")).toBe(found.art.join(","));
});


test("with the proportion kept the canvas keeps its shape, and one undo takes both sides back", async ({ editor, scratch, page }) => {
  await editor.create("Ratio", 100);
  const file = draftFile(scratch, "ratio", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  const canvas = JSON.parse(original).canvas;
  await editor.openCanvas();
  // The proportion is kept to begin with
  await expect(page.locator("#ed-canvas-levers [data-ratio='lock']")).toHaveAttribute("aria-pressed", "true");
  const width = Math.round(canvas.width * 0.8);
  await editor.lever("canvas-width").fill(String(width));
  await editor.lever("canvas-width").blur();
  const want = Math.round(width / (canvas.width / canvas.height));
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).canvas.height).toBe(want);
  expect(JSON.parse(await readFile(file, "utf8")).canvas.width).toBe(width);
  await expect(editor.lever("canvas-height")).toHaveValue(String(want));

  await editor.undoButton.click();
  await expect.poll(() => readFile(file, "utf8")).toBe(original);
  await expect(editor.undoButton).toBeDisabled();
});

test("the template's undo and redo are in its section and take back a change made anywhere", async ({ editor, scratch, page }) => {
  await editor.create("Global undo", 100);
  const file = draftFile(scratch, "global-undo", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  await expect(page.locator(".ed-draft #ed-undo")).toBeVisible();
  await editor.layers.filter({ hasText: "title" }).click();
  await editor.lever("lever-fontSize").fill("99");
  await editor.lever("lever-fontSize").blur();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).textBoxes.title.text.fontSize).toBe(99);
  await page.locator(".ed-draft #ed-undo").click();
  await expect.poll(() => readFile(file, "utf8")).toBe(original);
  await page.locator(".ed-draft #ed-redo").click();
  await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).textBoxes.title.text.fontSize).toBe(99);
});

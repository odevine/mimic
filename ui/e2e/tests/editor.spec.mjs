import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { test, expect } from "../fixtures.mjs";
import { solidPng } from "../png.mjs";

// Drafts are the app's own files, so these tests read and change them where the
// scratch folder keeps them
const draftFile = (scratch, id, ...parts) => path.join(scratch.home, "mimic", "drafts", id, ...parts);
const json = async (file) => JSON.parse(await readFile(file, "utf8"));

// reopen loads the page again and goes back to the editor, which reads the files as they are
async function reopen(editor) {
  await editor.page.reload();
  await editor.page.locator("html.booted").waitFor();
  await editor.show();
}

async function selectTitle(editor) {
  await expect(editor.layers.first()).toBeVisible();
  await editor.layers.filter({ hasText: "title" }).click();
  await expect(editor.selectionName).toHaveText("title");
}

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

test("the new template dialog checks what it is given, and a template is drawn as soon as it is made", async ({ editor }) => {
  await expect(editor.empty).toBeVisible();
  await editor.openNewDialog();
  await expect(editor.dialogDpi).toHaveValue("800");
  await expect(editor.page.locator("#draft-dpi-size")).toHaveText("A canvas of 2176 × 2960 pixels");
  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toHaveText("Give the template a name");
  await editor.dialogName.fill("Frame");
  await editor.dialogDpi.fill("5000");
  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toContainText("72 to 1200");
  await editor.page.keyboard.press("Escape");
  await expect(editor.empty).toBeVisible();

  await editor.create("Test frame", 150);
  await expect(editor.aboutName).toHaveText("Test frame");
  await expect(editor.aboutId).toHaveText("user.test-frame");
  await expect(editor.layouts).toHaveText(["normal · 408 × 555 px · 150 dpi"]);
  await expect(editor.problems).toHaveCount(0);
  await expect(editor.cardLine).toHaveText("Mirran Spy · MBS");
  await expect(editor.page.locator("#ed-activity-title")).toHaveText("Drew Mirran Spy");
  expect(await editor.preview.evaluate((img) => [img.naturalWidth, img.naturalHeight])).toEqual([408, 555]);
  // Every button that finishes a template is live, with no gate on it
  for (const id of ["#ed-use", "#ed-export", "#ed-folder", "#ed-json"]) await expect(editor.page.locator(id)).not.toHaveAttribute("data-gate-reason", /.*/);
});

test("templates are switched between, renamed without changing the id, and deleted only on a yes", async ({ editor, scratch }) => {
  await editor.create("One", 100);
  await editor.create("Two", 100);
  await expect(editor.picker.locator("option")).toHaveCount(2);
  await editor.picker.selectOption({ label: "One" });
  await expect(editor.aboutName).toHaveText("One");
  await editor.waitForPreview();

  await editor.page.locator("#ed-rename").click();
  await expect(editor.dialogName).toHaveValue("One");
  await editor.dialogName.fill("First");
  await editor.dialogConfirm.click();
  await expect(editor.aboutName).toHaveText("First");
  await expect(editor.aboutId).toHaveText("user.one");
  await expect(editor.picker.locator("option:checked")).toHaveText("First");

  await scratch.decline();
  await editor.page.locator("#ed-delete").click();
  await expect(editor.aboutName).toHaveText("First");
  await scratch.reset();
  await editor.page.locator("#ed-delete").click();
  await expect(editor.picker.locator("option")).toHaveCount(1);
});

test("a copy of an installed template is drawn, keeps the original's credit out of reach, and exports it", async ({ editor, templates, page }) => {
  await templates.show();
  await templates.entry("Test frame").getByRole("button", { name: "Edit a copy" }).click();
  await expect(editor.dialogName).toHaveValue("Test frame copy");
  await expect(page.locator("#draft-dpi-row")).toBeHidden();
  await editor.dialogName.fill("My copy");
  await editor.dialogConfirm.click();
  await editor.waitForPreview();
  await expect(editor.aboutId).toHaveText("user.my-copy");
  await expect(editor.aboutFork).toHaveText("Copy of mimic.m15");
  await expect(editor.problems).toHaveCount(0);

  await editor.openSection("about");
  const credits = page.locator("#ed-credits .ed-credit");
  await expect(credits.first()).toContainText("Kept from the template this was copied from");
  await expect(credits.first().getByRole("button")).toHaveCount(0);
  await expect(credits.first().locator("input").nth(2)).toHaveValue(/Forked from mimic\.m15/);
  await editor.exportButton.click();
  await expect(editor.exportCredits.first()).toContainText("Forked from mimic.m15");
});

test("dragging and nudging a text box change only its place in the file, and undo puts the file back byte for byte", async ({ editor, scratch, page }) => {
  await editor.create("Edits", 100);
  const file = draftFile(scratch, "edits", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  const src = await editor.preview.getAttribute("src");
  // The starter's canvas at 100 dpi is 272 pixels across, drawn at that size
  const per = 272 / (await editor.overlay.boundingBox()).width;

  await selectTitle(editor);
  const x = Number(await page.locator("#ed-sel-x").textContent());
  const ink = await editor.selectionBox.boundingBox();
  const from = { x: ink.x + ink.width / 2, y: ink.y + ink.height / 2 };
  await page.keyboard.down("Alt");
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + 30, from.y, { steps: 5 });
  await page.mouse.up();
  await page.keyboard.up("Alt");
  const moved = Math.round(30 * per);
  await expect(page.locator("#ed-sel-x")).toHaveText(String(x + moved));

  // Only the title's x changed, and the file kept its order and layout
  const edited = await readFile(file, "utf8");
  const was = JSON.parse(original);
  expect(Object.keys(JSON.parse(edited))).toEqual(Object.keys(was));
  was.textBoxes.title.rect.x += moved;
  expect(JSON.parse(edited)).toEqual(was);
  expect(edited.split("\n").length).toBe(original.split("\n").length);
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(src);
  await editor.waitForPreview();

  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowRight");
  await expect.poll(async () => (await json(file)).textBoxes.title.rect.x).toBe(was.textBoxes.title.rect.x + 2);

  await page.keyboard.press("ControlOrMeta+z");
  await expect.poll(() => readFile(file, "utf8")).toBe(edited);
  await editor.undoButton.click();
  await expect(page.locator("#ed-sel-x")).toHaveText(String(x));
  expect(await readFile(file, "utf8")).toBe(original);
  await editor.redoButton.click();
  await expect.poll(() => readFile(file, "utf8")).toBe(edited);
});

test("levers write one field of the file each, the engine refuses what it cannot read, and the canvas keeps its proportion", async ({ editor, scratch, page }) => {
  await editor.create("Levers", 100);
  const file = draftFile(scratch, "levers", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  const src = await editor.preview.getAttribute("src");
  await selectTitle(editor);

  const was = JSON.parse(original).textBoxes.title.text.fontSize;
  await editor.lever("lever-fontSize").fill(String(was + 4));
  await editor.lever("lever-fontSize").blur();
  await expect.poll(async () => (await json(file)).textBoxes.title.text.fontSize).toBe(was + 4);
  const edited = JSON.parse(original);
  edited.textBoxes.title.text.fontSize = was + 4;
  expect(await json(file)).toEqual(edited);
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(src);
  await editor.undoButton.click();
  await expect(editor.lever("lever-fontSize")).toHaveValue(String(was));
  expect(await readFile(file, "utf8")).toBe(original);

  await editor.lever("lever-condition").fill("made_up");
  await editor.lever("lever-condition").blur();
  await expect(page.locator("#toasts")).toContainText("made_up");
  await expect(editor.lever("lever-condition")).toHaveValue("");
  expect(await readFile(file, "utf8")).toBe(original);
  await expect(editor.undoButton).toBeDisabled();
  await editor.lever("lever-condition").fill("creature");
  await editor.lever("lever-condition").blur();
  await expect.poll(async () => (await json(file)).textBoxes.title.condition).toBe("creature");

  await editor.layers.filter({ hasText: "mana" }).first().click();
  await page.locator("#lever-shadow-on").uncheck();
  await expect.poll(async () => (await json(file)).textBoxes.mana.text.shadow).toBeUndefined();
  await editor.layers.filter({ hasText: "background" }).click();
  await editor.lever("lever-mode").selectOption("multiply");
  await expect.poll(async () => (await json(file)).stack[0].mode).toBe("multiply");

  const canvas = JSON.parse(original).canvas;
  await editor.openCanvas();
  const width = Math.round(canvas.width * 0.8);
  await editor.lever("canvas-width").fill(String(width));
  await editor.lever("canvas-width").blur();
  const want = Math.round(width / (canvas.width / canvas.height));
  await expect.poll(async () => (await json(file)).canvas.height).toBe(want);
  expect((await json(file)).canvas.width).toBe(width);
  await expect(editor.layouts.first()).toContainText(`${width} × ${want} px`);
});

test("the template's name, credits and owner are written to its file, and the owner can come from the author", async ({ editor, scratch, page }) => {
  await editor.create("Named", 100);
  const file = draftFile(scratch, "named", "template.json");
  await editor.openSection("about");
  await editor.lever("template-name").fill("Better name");
  await editor.lever("template-name").blur();
  await expect.poll(async () => (await json(file)).name).toBe("Better name");
  await expect(editor.picker.locator("option:checked")).toHaveText("Better name");

  await page.locator("#ed-credit-add").click();
  await expect.poll(async () => (await json(file)).credits).toEqual([{ name: "New credit" }]);
  await page.locator("#ed-credits .ed-credit input").first().fill("Someone");
  await expect.poll(async () => (await json(file)).credits).toEqual([{ name: "Someone" }]);

  await editor.lever("template-owner").fill("jane");
  await editor.lever("template-owner").blur();
  await expect.poll(async () => (await json(file)).id).toBe("jane.named");
  await expect(page.locator("#ed-about-id")).toHaveText("jane.named");
  await editor.undoButton.click();
  await expect.poll(async () => (await json(file)).id).toBe("user.named");
  await editor.lever("template-authorName").fill("Jane Q. Doe");
  await editor.lever("template-authorName").blur();
  await page.getByRole("button", { name: "Use the author's name" }).click();
  await expect.poll(async () => (await json(file)).id).toBe("jane-q-doe.named");
});

test("a template that does not load says why, is fixed in the JSON editor, and a mistake there is not saved", async ({ editor, scratch, page }) => {
  await editor.create("Fixme", 100);
  const file = draftFile(scratch, "fixme", "layouts", "normal.json");
  const good = await readFile(file, "utf8");
  await writeFile(file, good.replace('"align": "left"', '"align": "diagonal"'));
  await reopen(editor);
  await expect(editor.problems.first()).toContainText("align");
  await expect(page.locator("#ed-activity-title")).toHaveText("This template does not load");
  await expect(editor.preview).toBeHidden();

  await editor.jsonButton.click();
  await editor.jsonTabs.filter({ hasText: "normal" }).click();
  await editor.jsonText.fill(good);
  await expect.poll(() => readFile(file, "utf8")).toBe(good);
  await expect(editor.problems).toHaveCount(0);
  await editor.waitForPreview();

  const edited = good.replace(/"dpi": \d+/, '"dpi": 95');
  await editor.jsonText.fill(edited);
  await expect.poll(() => readFile(file, "utf8")).toBe(edited);
  await expect(editor.layouts.first()).toContainText("95 dpi");
  await editor.jsonText.fill(edited.replace('"serves": [', '"serves": ,['));
  await expect(editor.jsonProblems.first()).toContainText(/^Line \d+, column \d+:/);
  await editor.jsonText.fill(edited.replace('"align": "left"', '"align": "diagonal"'));
  await expect(editor.jsonProblems.first()).toContainText("align");
  await expect(editor.jsonStatus).toHaveText("Not saved, fix the problems first");
  expect(await readFile(file, "utf8")).toBe(edited);
});

test("a layout that builds on another can be looked at and not changed", async ({ editor, scratch, page }) => {
  await editor.create("Built", 100);
  const dir = path.join(scratch.home, "mimic", "drafts", "built", "layouts");
  const raw = await readFile(path.join(dir, "normal.json"), "utf8");
  await writeFile(path.join(dir, "base.json"), raw.replace('"role": "single"', '"role": "split"'));
  await writeFile(path.join(dir, "normal.json"), JSON.stringify({ serves: [{ role: "single", kind: "standard" }], extends: ["base"] }));
  await reopen(editor);
  await editor.waitForPreview();

  await expect(page.locator("#ed-edit-note")).toContainText("extends another one");
  await selectTitle(editor);
  await expect(editor.handles).toHaveCount(0);
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(500);
  await expect(editor.undoButton).toBeDisabled();
  expect((await json(path.join(dir, "normal.json"))).extends).toEqual(["base"]);
});

test("an image and a font are brought in and drawn with, a file that is not a PNG is refused, and undo brings each back", async ({ editor, scratch, page }) => {
  await editor.create("Images", 100);
  const dir = draftFile(scratch, "images");
  const file = path.join(dir, "parts", "background", "any.png");
  const original = await readFile(file);
  await editor.layers.filter({ hasText: "background" }).click();
  await expect(editor.cells).toHaveCount(1);

  await scratch.offer("fake.png", "this is not an image");
  await editor.cells.first().locator(".ed-cell-main").click();
  await expect(page.locator("#toasts")).toContainText("not a PNG");
  expect((await readFile(file)).equals(original)).toBe(true);
  await expect(editor.undoButton).toBeDisabled();

  const red = solidPng(272, 370, [200, 20, 20, 255]);
  // The open dialog answers with the first file it is offered, so each offer starts afresh
  await scratch.reset();
  await scratch.offer("red.png", red);
  const src = await editor.preview.getAttribute("src");
  await editor.cells.first().locator(".ed-cell-main").click();
  await expect.poll(async () => (await readFile(file)).equals(red)).toBe(true);
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(src);
  await editor.waitForPreview();
  const [r, g] = await previewPixel(editor, 2, 2);
  expect(r).toBeGreaterThan(150);
  expect(g).toBeLessThan(60);
  await editor.undoButton.click();
  await expect.poll(async () => (await readFile(file)).equals(original)).toBe(true);
  await editor.redoButton.click();
  await expect.poll(async () => (await readFile(file)).equals(red)).toBe(true);

  await scratch.reset();
  await scratch.offer("Plantin.ttf", { path: path.resolve(import.meta.dirname, "../../../engine/fonts/embedded/m15/plantin-body.ttf") });
  await editor.openSection("about");
  await editor.fontAdd.click();
  await expect(editor.fonts).toHaveCount(1);
  const [id, entry] = Object.entries((await json(path.join(dir, "template.json"))).fonts)[0];
  expect(entry.file).toBe(`font/${id}.ttf`);
  expect(entry.sha256).toHaveLength(64);
  await page.locator("#ed-role-body").selectOption(`font:${id}`);
  await expect.poll(async () => (await json(path.join(dir, "template.json"))).roles?.body?.font).toBe(id);
  await editor.waitForPreview();
  await expect(editor.problems).toHaveCount(0);
  // Removing the font frees the role first, so the template still loads
  await editor.fonts.first().getByRole("button", { name: /Remove the font/ }).click();
  await expect(editor.fonts).toHaveCount(0);
  const gone = await json(path.join(dir, "template.json"));
  expect(gone.fonts ?? {}).toEqual({});
  expect(gone.roles?.body).toBeUndefined();
  await expect(editor.problems).toHaveCount(0);
});

test("layers and text boxes are added, copied and deleted, and undoing all of it restores the files", async ({ editor, scratch }) => {
  await editor.create("Adds", 100);
  const dir = draftFile(scratch, "adds");
  const file = path.join(dir, "layouts", "normal.json");
  const template = await json(path.join(dir, "template.json"));
  const layout = await readFile(file, "utf8");
  await scratch.offer("glow.png", solidPng(272, 370, [10, 200, 10, 90]));

  await editor.addButton.click();
  await editor.addKind.selectOption("layer");
  await editor.addPart.selectOption({ label: "A new part from a PNG image" });
  await editor.addPartName.fill("glow");
  await editor.addConfirm.click();
  await expect(editor.selectionName).toHaveText("glow");
  expect((await json(path.join(dir, "template.json"))).parts.glow).toEqual({ dir: "parts/glow" });
  expect((await readFile(path.join(dir, "parts", "glow", "any.png"))).length).toBeGreaterThan(0);
  expect((await json(file)).stack.map((e) => e.id || e.part)).toEqual(["background", "glow"]);
  await editor.waitForPreview();
  await expect(editor.problems).toHaveCount(0);

  await editor.addButton.click();
  await editor.addKind.selectOption("text");
  await editor.addDraws.selectOption("artist");
  await editor.addConfirm.click();
  await expect.poll(async () => Object.keys((await json(file)).textBoxes)).toContain("artist_2");
  await editor.choose("oracle", "Duplicate");
  await expect(editor.selectionName).toHaveText("oracle_copy");
  const boxes = (await json(file)).textBoxes;
  expect(boxes.oracle_copy.draws).toBe("oracle");
  expect(boxes.oracle_copy.rect.x).toBe(boxes.oracle.rect.x + 8);
  await editor.choose("oracle_copy", "Delete");
  await expect.poll(async () => Object.keys((await json(file)).textBoxes)).not.toContain("oracle_copy");

  for (let i = 0; i < 4; i++) await editor.undoButton.click();
  await expect(editor.undoButton).toBeDisabled();
  // The undos are made one after another, so the files are read until they are as they were
  await expect.poll(() => readFile(file, "utf8")).toBe(layout);
  expect((await json(path.join(dir, "template.json"))).parts).toEqual(template.parts);
  await expect.poll(async () => readFile(path.join(dir, "parts", "glow", "any.png")).then(() => true, () => false)).toBe(false);
});

test("the test card's fields change what the template draws, and the card and a saved test card are kept across a reload", async ({ editor, page }) => {
  await editor.create("Fine tune", 100);
  // The first preset is a creature, so its power and toughness box is drawn
  await expect(editor.row("pt")).not.toHaveClass(/undrawn/);
  const src = await editor.preview.getAttribute("src");
  await editor.openFine();
  await editor.cardField("typeLine").fill("Land");
  await editor.cardField("power").fill("");
  await editor.cardField("toughness").fill("");
  await editor.cardField("name").fill("My Test Land");
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew My Test Land");
  await expect.poll(() => editor.preview.getAttribute("src")).not.toBe(src);
  // A land has no power or toughness, so the box has nothing to draw
  await expect(editor.row("pt")).toHaveClass(/undrawn/);

  await reopen(editor);
  await editor.waitForPreview();
  await expect(editor.cardLine).toHaveText("My Test Land · MBS");
  await expect(editor.row("pt")).toHaveClass(/undrawn/);

  await editor.preset("Land").click();
  await editor.waitForPreview();
  await editor.savePreset.click();
  await expect(editor.dialogName).toHaveValue("Flooded Strand");
  await editor.dialogName.fill("My strand");
  await editor.dialogConfirm.click();
  await expect(editor.userPresets).toHaveCount(1);
  await reopen(editor);
  await expect(editor.userPresets.locator(".chip").first()).toHaveText("My strand");
  await editor.preset("Monocolored").click();
  await expect(editor.row("pt")).not.toHaveClass(/undrawn/);
  await editor.userPresets.locator(".chip").first().click();
  await expect(editor.cardLine).toHaveText("Flooded Strand · KTK");
  await editor.userPresets.getByRole("button", { name: "Remove the test card My strand" }).click();
  await expect(editor.userPresets).toHaveCount(0);
  await reopen(editor);
  await expect(editor.userPresets).toHaveCount(0);
});

test("a searched card's artwork replaces the preset's in the art window, and an edit keeps it", async ({ editor, page }) => {
  await editor.create("Art window", 100);
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
  // The art is the searched card's and not the preset's, which the fake draws differently
  const found = await pixel();
  expect(found.art).not.toEqual(found.edge);
  expect(found.art).not.toEqual(preset.art);

  await editor.openFine();
  await editor.cardField("flavor").fill("Roots first.");
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Llanowar Elves");
  await expect.poll(async () => (await pixel()).art.join(",")).toBe(found.art.join(","));
});

test("a text box's flow and its baseline's width are written to the file, and undo takes each back", async ({ editor, scratch, page }) => {
  await editor.create("Flowing", 100);
  const file = draftFile(scratch, "flowing", "layouts", "normal.json");
  const original = await readFile(file, "utf8");
  await selectTitle(editor);
  // The title is narrowed by the engine to clear the mana cost for the card
  await expect(page.locator("#ed-selection-note")).toHaveText("Narrowed to stay clear of mana for this card");
  await expect(editor.lever("lever-clearOf")).toHaveValue("mana");
  await editor.lever("lever-avoidLayer").selectOption("background");
  await expect.poll(async () => (await json(file)).textBoxes.title.flow).toEqual({ clearOf: "mana", avoidLayer: "background" });
  await expect(page.locator("#ed-selection-note")).toContainText("Steps around background");
  await editor.undoButton.click();
  await expect(editor.lever("lever-avoidLayer")).toHaveValue("");
  expect(await readFile(file, "utf8")).toBe(original);

  const was = JSON.parse(original).textBoxes.title.rect;
  await expect(page.locator(".ed-readout-grid dt").nth(2)).toHaveText("Cap center");
  await expect(page.locator("#ed-sel-w")).toHaveText(String(was.width));
  const handle = page.locator("#ed-overlay .ed-baseline-handle[data-handle='e']");
  await expect(handle).toBeVisible();
  const end = await handle.boundingBox();
  await page.mouse.move(end.x + end.width / 2, end.y + end.height / 2);
  await page.mouse.down();
  await page.mouse.move(end.x + end.width / 2 - 30, end.y + end.height / 2, { steps: 5 });
  await page.mouse.up();
  await expect.poll(async () => (await json(file)).textBoxes.title.rect.width).toBeLessThan(was.width - 20);
  const after = (await json(file)).textBoxes.title.rect;
  expect({ x: after.x, y: after.y, height: after.height }).toEqual({ x: was.x, y: was.y, height: was.height });
  await editor.undoButton.click();
  await expect.poll(() => readFile(file, "utf8")).toBe(original);
});

test("export writes a bundle and show folder opens the template's own folder", async ({ editor, scratch }) => {
  await editor.create("Mine", 100);
  await editor.exportButton.click();
  await expect(editor.exportFacts).toContainText("user.mine");
  await expect(editor.exportFacts).toContainText("0.1.0");
  await expect(editor.exportProblems).toHaveCount(0);
  expect(await scratch.saved()).toEqual([]);
  await editor.exportConfirm.click();
  await expect(editor.toasts.last()).toContainText("Saved user.mine-0.1.0.mimic");
  expect(await scratch.saved()).toEqual(["user.mine-0.1.0.mimic"]);
  const bytes = await readFile(scratch.savedPath("user.mine-0.1.0.mimic"));
  expect(bytes.subarray(0, 2).toString()).toBe("PK");
  expect(bytes.includes("bundle.json") && bytes.includes("manifest.json")).toBe(true);

  await editor.folderButton.click();
  await expect.poll(async () => (await scratch.events()).filter(([kind]) => kind === "path").map(([, detail]) => detail)).toContain(path.join(scratch.home, "mimic", "drafts", "mine"));
});

test("the coverage strip draws the six test cards through the template", async ({ editor }) => {
  await editor.create("Mine", 100);
  await editor.coverage.locator("summary").click();
  await editor.coverageRun.click();
  await expect(editor.coverageLine).toHaveText("6 of 6 drawn", { timeout: 60_000 });
  const sizes = await editor.tiles.locator("img").evaluateAll((imgs) => imgs.map((i) => [i.naturalWidth, i.naturalHeight]));
  expect(sizes).toHaveLength(6);
  // A strip tile is smaller than the card drawn for the preview at 100 dpi
  for (const [w] of sizes) expect(w).toBeLessThan(272);
});

test("a hidden layer is left out of the card and the PNG saved of it, which is named for the card", async ({ editor, scratch }) => {
  await editor.create("Hidden", 100);
  const eye = editor.row("background").locator('[data-toggle="visible"]');
  expect((await previewPixel(editor, 2, 2))[3]).toBe(255);
  await eye.click();
  await expect.poll(async () => (await previewPixel(editor, 2, 2))[3], { timeout: 30_000 }).toBe(0);
  await editor.waitForPreview();
  // Nothing was saved to the template for it
  await expect(editor.undoButton).toBeDisabled();

  await editor.page.locator("#ed-save-image").click();
  await expect(editor.toasts.last()).toContainText("Saved Mirran Spy.png");
  expect(await scratch.saved()).toEqual(["Mirran Spy.png"]);
  const bytes = await readFile(scratch.savedPath("Mirran Spy.png"));
  expect([...bytes.subarray(0, 4)]).toEqual([0x89, 0x50, 0x4e, 0x47]);
  expect([bytes.readUInt32BE(16), bytes.readUInt32BE(20)]).toEqual(await editor.preview.evaluate((img) => [img.naturalWidth, img.naturalHeight]));
  // The file has what the card shows: the corner the background painted is not painted in it
  const alpha = await editor.page.evaluate(async (b64) => {
    const img = new Image();
    img.src = `data:image/png;base64,${b64}`;
    await img.decode();
    const c = document.createElement("canvas");
    c.width = img.naturalWidth;
    c.height = img.naturalHeight;
    const ctx = c.getContext("2d");
    ctx.drawImage(img, 0, 0);
    return ctx.getImageData(2, 2, 1, 1).data[3];
  }, bytes.toString("base64"));
  expect(alpha).toBe(0);
});

test("saving at full size draws the card at the template's own resolution and saves it", async ({ editor, scratch }) => {
  // A template authored at 400 dpi is larger than the preview, whose resolution is a setting
  await editor.create("Full size", 400);
  const { canvas } = await json(draftFile(scratch, "full-size", "layouts", "normal.json"));
  const [w, h] = await editor.preview.evaluate((img) => [img.naturalWidth, img.naturalHeight]);
  expect(w).toBeLessThan(canvas.width);
  await editor.page.locator("#ed-save-image").click();
  await editor.page.getByRole("menuitem", { name: "At full size" }).click();
  await expect(editor.toasts.last()).toContainText("Saved Mirran Spy.png", { timeout: 60_000 });
  const bytes = await readFile(scratch.savedPath("Mirran Spy.png"));
  expect([bytes.readUInt32BE(16), bytes.readUInt32BE(20)]).toEqual([canvas.width, canvas.height]);
  // The card on show is still the preview
  expect(await editor.preview.evaluate((img) => [img.naturalWidth, img.naturalHeight])).toEqual([w, h]);
});

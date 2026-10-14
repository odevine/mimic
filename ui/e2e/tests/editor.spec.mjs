import { writeFile } from "node:fs/promises";
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

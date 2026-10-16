import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, describeView, standardItems, drawn, at, assetsView, standardAssets } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The editor on one template with its card drawn, as the other editor page tests have it
async function ready(bridge, page) {
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.layers).toHaveCount(7);
  await editor.sizeCanvas(400);
  return editor;
}

const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);
const select = (editor, name) => editor.row(name).click();
const plantin = { id: "plantin-regular", file: "font/plantin-regular.ttf", family: "Plantin", style: "Regular" };

test("a layer shows a cell for each material of its part, and says what covers the ones with no image", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(editor.images).toBeHidden();
  await select(editor, "frame");
  await expect(editor.images.locator(".ed-images-note")).toContainText("5 of 21 materials have an image, 6 would leave a hole");
  await expect(editor.cells.locator("xpath=self::*[@data-state='image']")).toHaveCount(5);
  await expect(editor.cells.locator("xpath=self::*[@data-state='blended']")).toHaveCount(10);
  await expect(editor.cells.locator("xpath=self::*[@data-state='missing']")).toHaveCount(6);
  await expect(editor.images.locator('.ed-cell[data-material="wu"] .ed-cell-note')).toHaveText("blended from White and Blue");
  // The images that exist are drawn small
  await expect(editor.images.locator('.ed-cell[data-material="w"] img')).toHaveCount(1);
  expect(bridge.calledWith("Studio.Thumbs")[0][0].files).toEqual(["parts/frame/w.png", "parts/frame/u.png", "parts/frame/b.png", "parts/frame/r.png", "parts/frame/g.png"]);

  // A part with no slot has the one cell, and a text box has none
  await select(editor, "background");
  await expect(editor.cells).toHaveCount(1);
  await select(editor, "title");
  await expect(editor.images).toBeHidden();
});

test("an image is replaced from a file and can be undone and redone, and no file or a refused one changes nothing", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await select(editor, "frame");
  const cell = editor.images.locator('.ed-cell[data-material="w"] .ed-cell-main');

  bridge.reply("System.PickFile", "");
  await cell.click();
  await page.waitForTimeout(200);
  expect(bridge.calledWith("Studio.ImportImage")).toEqual([]);

  bridge.reply("System.PickFile", "/art/not.png");
  bridge.fail("Studio.ImportImage", "That is not a PNG image. Export the layer as a PNG");
  await cell.click();
  await expect(page.locator(".toast.err")).toContainText("That is not a PNG image");
  await expect(editor.undoButton).toBeDisabled();

  bridge.reply("System.PickFile", "/art/white.png");
  bridge.reply("Studio.ImportImage", { rev: 1, inverse: [{ op: "put", file: "parts/frame/w.png", stash: "old" }], valid: true, diagnostics: [], id: "frame", warnings: [] });
  const renders = bridge.calledWith("Studio.Render").length;
  await cell.click();
  await expect.poll(() => bridge.calledWith("Studio.ImportImage").length).toBe(2);
  expect(bridge.calledWith("Studio.ImportImage")[1][0]).toEqual({ id: "mine", part: "frame", material: "w", slot: "", path: "/art/white.png" });
  // The card is drawn again and what the draft holds is read again
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBeGreaterThan(renders);
  await expect.poll(() => bridge.calledWith("Studio.Assets").length).toBeGreaterThan(1);

  // Undo puts back what the app said undoes it, and redo makes it again with what undo said
  bridge.reply("Studio.Apply", () => ({ rev: 2, inverse: [{ op: "put", file: "parts/frame/w.png", stash: "new" }], valid: true, diagnostics: [] }));
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "put", file: "parts/frame/w.png", stash: "old" }]);
  await editor.redoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "put", file: "parts/frame/w.png", stash: "new" }]);
});

test("an image is removed with its file and any variant naming it, and undo puts it back", async ({ bridge, page }) => {
  bridge.source.template.parts.frame.variants = { u: "parts/frame/u.png" };
  const editor = await ready(bridge, page);
  await select(editor, "frame");
  // Only an image has one to remove
  await expect(editor.images.locator('.ed-cell[data-material="wu"] .ed-cell-remove')).toHaveCount(0);
  bridge.reply("Studio.Apply", { rev: 1, inverse: [{ op: "put", file: "parts/frame/u.png", stash: "kept" }], valid: true, diagnostics: [] });
  await editor.images.locator('.ed-cell[data-material="u"] .ed-cell-remove').click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([
    { op: "unset", kind: "part", key: "frame", path: ["variants", "u"] },
    { op: "drop", file: "parts/frame/u.png" },
  ]);
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "put", file: "parts/frame/u.png", stash: "kept" }]);
});

test("a row's menu duplicates or deletes that row, and selects a copy once it is drawn", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  // The menu is for the row it is in, whatever is selected
  await select(editor, "oracle");
  await editor.choose("frame", "Delete");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "delete", layout: "normal", kind: "layer", key: "frame" }]);
  await expect(editor.selectionName).toHaveText("oracle");

  bridge.reply("Studio.Describe", () => describeView([...standardItems, drawn("text", "title_copy", at(120, 100, 600, 80))]));
  await editor.choose("title", "Duplicate");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "duplicate", layout: "normal", kind: "text", key: "title", dx: 20, dy: 20, as: "title_copy" }]);
  await expect(editor.selectionName).toHaveText("title_copy");

  // Deleting the selected row lets go of the selection
  await editor.choose("title_copy", "Delete");
  await expect(editor.selection).toBeHidden();
});

test("the add dialog adds a text box, an art window or a layer for a part the template has", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.addButton.click();
  await expect(editor.addKind).toHaveValue("text");
  await editor.addDraws.selectOption("artist");
  bridge.reply("Studio.Describe", () => describeView([...standardItems, drawn("text", "artist", at(150, 644, 700, 112))]));
  await editor.addConfirm.click();
  await expect(editor.addDialog).toBeHidden();
  await expect.poll(() => applied(bridge).length).toBe(1);
  // It is named for what it draws and placed in the middle, and selected
  expect(applied(bridge)[0].ops[0]).toMatchObject({ op: "add", layout: "normal", kind: "text", key: "artist", rect: { x: 150, y: 644, width: 700, height: 112 }, value: { draws: "artist" } });
  await expect(editor.selectionName).toHaveText("artist");

  await editor.addButton.click();
  await editor.addKind.selectOption("art");
  await expect(editor.addDraws).toBeHidden();
  await editor.addAfter.selectOption("background");
  await editor.addConfirm.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "add", kind: "art", key: "1", value: { after: "background" } });

  // A layer for a part that has one already takes an id of its own
  await editor.addButton.click();
  await editor.addKind.selectOption("layer");
  await editor.addPart.selectOption("frame");
  await editor.addConfirm.click();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ op: "add", kind: "layer", key: "frame-2", to: 2, value: { id: "frame-2", part: "frame" } });
  expect(bridge.calledWith("System.PickFile")).toEqual([]);
});

test("a layer for a new part checks its name, takes a PNG for its first image, and is one step of undo", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.addButton.click();
  await editor.addKind.selectOption("layer");
  await editor.addPart.selectOption({ label: "A new part from a PNG image" });
  await editor.addPartName.fill("");
  await editor.addConfirm.click();
  await expect(editor.addError).toContainText("lowercase letters, digits, underscores and hyphens");
  await editor.addPartName.fill("frame");
  await editor.addConfirm.click();
  await expect(editor.addError).toContainText("has a part named frame already");
  // Cancelling the file chooser leaves the dialog open with nothing said
  bridge.reply("System.PickFile", "");
  await editor.addPartName.fill("glow");
  await editor.addConfirm.click();
  await expect(editor.addDialog).toBeVisible();
  await expect(editor.addError).toBeHidden();
  expect(bridge.calledWith("Studio.ImportImage")).toEqual([]);

  bridge.reply("System.PickFile", "/art/glow.png");
  bridge.reply("Studio.ImportImage", { rev: 1, inverse: [{ op: "drop", file: "parts/glow/any.png" }, { op: "unset", kind: "template", path: ["parts", "glow"] }], valid: true, diagnostics: [], id: "glow", warnings: [] });
  bridge.reply("Studio.Apply", { rev: 2, inverse: [{ op: "delete", layout: "normal", kind: "layer", key: "glow" }], valid: true, diagnostics: [] });
  await editor.addSlot.selectOption("pinlines");
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

test("a font is given to roles, keeps its license, and removing it frees the roles in one step", async ({ bridge, page }) => {
  const roles = assetsView([]).roles.map((r) => (r.role === "title" ? { ...r, font: "plantin-regular" } : r));
  bridge.assets = assetsView(standardAssets().parts, { roles, fonts: [{ ...plantin, license: "OFL 1.1" }] });
  const editor = await ready(bridge, page);
  await editor.openSection("about");
  await expect(editor.fonts).toHaveCount(1);
  await expect(page.locator("#ed-role-title")).toHaveValue("font:plantin-regular");
  const body = page.locator("#ed-role-body");
  await expect(body).toHaveValue("");
  await expect(body.locator("option").first()).toHaveText("Default (Built-in m15 body)");
  await body.selectOption("font:plantin-regular");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", kind: "template", path: ["roles", "body"], value: { font: "plantin-regular" } }]);
  await body.selectOption("");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "unset", kind: "template", path: ["roles", "body"] }]);

  const license = page.getByLabel("License of the font Plantin Regular");
  await expect(license).toHaveValue("OFL 1.1");
  await license.fill("");
  await license.blur();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops).toEqual([{ op: "unset", kind: "template", path: ["fonts", "plantin-regular", "license"] }]);

  await editor.fonts.first().getByRole("button", { name: /Remove the font Plantin Regular/ }).click();
  await expect.poll(() => applied(bridge).length).toBe(4);
  expect(applied(bridge)[3].ops).toEqual([
    { op: "unset", kind: "template", path: ["roles", "title"] },
    { op: "unset", kind: "template", path: ["fonts", "plantin-regular"] },
    { op: "drop", file: "font/plantin-regular.ttf" },
  ]);
});

test("a font is added from a file and undo takes it away, and one the app refuses changes nothing", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openSection("about");
  await expect(page.locator("#ed-fonts")).toContainText("ships no fonts");

  bridge.reply("System.PickFile", "/fonts/broken.ttf");
  bridge.fail("Studio.ImportFont", "That is not a font the engine can read");
  await editor.fontAdd.click();
  await expect(page.locator(".toast.err")).toContainText("not a font the engine can read");
  await expect(editor.undoButton).toBeDisabled();

  bridge.reply("System.PickFile", "/fonts/Plantin.ttf");
  bridge.reply("Studio.ImportFont", { rev: 1, inverse: [{ op: "unset", kind: "template", path: ["fonts", "plantin-regular"] }, { op: "drop", file: "font/plantin-regular.ttf" }], valid: true, diagnostics: [], id: "plantin-regular", warnings: [] });
  bridge.assets = assetsView(standardAssets().parts, { fonts: [plantin] });
  await editor.fontAdd.click();
  await expect.poll(() => bridge.calledWith("Studio.ImportFont").length).toBe(2);
  expect(bridge.calledWith("Studio.ImportFont")[1][0]).toEqual({ id: "mine", path: "/fonts/Plantin.ttf", assign: [] });
  await expect(editor.fonts).toHaveCount(1);
  await expect(page.locator(".toast").last()).toContainText("Choose where it is used");

  bridge.assets = standardAssets();
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([
    { op: "unset", kind: "template", path: ["fonts", "plantin-regular"] },
    { op: "drop", file: "font/plantin-regular.ttf" },
  ]);
  await expect(editor.fonts).toHaveCount(0);
});

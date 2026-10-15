import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, describeView, standardItems } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

async function ready(bridge, page) {
  bridge.reply("Studio.Describe", () => describeView(standardItems));
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

const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);
const dialog = (page) => page.locator("#ed-drop-dialog");
const imported = (name) => ({ rev: 1, inverse: [{ op: "drop", file: `parts/${name}/any.png` }, { op: "unset", kind: "template", path: ["parts", name] }], valid: true, diagnostics: [], id: name, warnings: [] });

test("a PNG dropped on the editor asks what it is for, then adds it as a part and a layer", async ({ bridge, page }) => {
  bridge.reply("Studio.ImportImage", imported("frame_glow"));
  bridge.reply("Studio.Apply", { rev: 2, inverse: [{ op: "delete", layout: "normal", kind: "layer", key: "frame_glow" }], valid: true, diagnostics: [] });
  const editor = await ready(bridge, page);
  await bridge.emit("files-dropped", ["/art/Frame Glow.PNG"]);
  await expect(dialog(page)).toBeVisible();
  await expect(page.locator("#ed-drop-file")).toHaveText("Frame Glow.PNG");
  // The part is named from the file, in the letters a part's name takes
  await expect(page.locator("#ed-drop-name")).toHaveValue("frame_glow");
  await expect(page.locator("#ed-drop-slot option").first()).toHaveText("Nothing, one image for every card");
  await page.locator("#ed-drop-slot").selectOption("pinlines");
  await page.locator("#ed-drop-confirm").click();
  await expect(dialog(page)).toBeHidden();
  // The file the window reported is the one brought in, with no file chooser
  expect(bridge.calledWith("System.PickFile")).toEqual([]);
  expect(bridge.calledWith("Studio.ImportImage")[0][0]).toEqual({ id: "mine", part: "frame_glow", material: "any", slot: "pinlines", path: "/art/Frame Glow.PNG" });
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0]).toMatchObject({ op: "add", kind: "layer", key: "frame_glow" });

  // The layer and the image are one step of undo
  bridge.reply("Studio.Apply", { rev: 3, inverse: [], valid: true, diagnostics: [] });
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([
    { op: "delete", layout: "normal", kind: "layer", key: "frame_glow" },
    { op: "drop", file: "parts/frame_glow/any.png" },
    { op: "unset", kind: "template", path: ["parts", "frame_glow"] },
  ]);
});

test("a dropped image whose part name is wrong or taken is said and the dialog stays", async ({ bridge, page }) => {
  await ready(bridge, page);
  await bridge.emit("files-dropped", ["/art/glow.png"]);
  await expect(dialog(page)).toBeVisible();
  await page.locator("#ed-drop-name").fill("Not A Name");
  await page.locator("#ed-drop-confirm").click();
  await expect(page.locator("#ed-drop-error")).toContainText("lowercase letters, digits, underscores and hyphens");
  await page.locator("#ed-drop-name").fill("frame");
  await page.locator("#ed-drop-confirm").click();
  await expect(page.locator("#ed-drop-error")).toContainText("has a part named frame already");
  await page.locator("#ed-drop-name").fill("glow");
  await expect(page.locator("#ed-drop-error")).toBeHidden();
  expect(bridge.calledWith("Studio.ImportImage")).toEqual([]);
});

test("several images dropped are asked about one after another, and cancelling drops the rest", async ({ bridge, page }) => {
  bridge.reply("Studio.ImportImage", (req) => imported(req.part));
  bridge.reply("Studio.Apply", { rev: 2, inverse: [], valid: true, diagnostics: [] });
  await ready(bridge, page);
  await bridge.emit("files-dropped", ["/art/one.png", "/art/notes.txt", "/art/two.png", "/art/three.png"]);
  await expect(page.locator("#ed-drop-file")).toHaveText("one.png");
  await expect(page.locator("#ed-drop-title")).toHaveText("Add this image, 3 to go");
  await page.locator("#ed-drop-confirm").click();
  await expect(page.locator("#ed-drop-file")).toHaveText("two.png");
  await expect(page.locator("#ed-drop-title")).toHaveText("Add this image, 2 to go");
  await page.getByRole("button", { name: "Cancel" }).filter({ visible: true }).click();
  await expect(dialog(page)).toBeHidden();
  expect(bridge.calledWith("Studio.ImportImage").map(([r]) => r.part)).toEqual(["one"]);
});

test("a file that is not a PNG is said and starts nothing", async ({ bridge, page }) => {
  await ready(bridge, page);
  await bridge.emit("files-dropped", ["/art/frame.psd"]);
  await expect(page.locator(".toast.warn")).toContainText("PNG");
  await expect(dialog(page)).toBeHidden();
  expect(bridge.calledWith("Studio.ImportImage")).toEqual([]);
});

test("the card and the layers list light up under a dragged file, and a drop elsewhere does nothing", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await page.locator("#ed-stage").dispatchEvent("dragover");
  await expect(page.locator("#ed-stage")).toHaveClass(/drop/);
  await page.locator("#ed-stage").dispatchEvent("dragleave");
  await expect(page.locator("#ed-stage")).not.toHaveClass(/drop/);
  await page.locator("#ed-sec-layers-body").dispatchEvent("dragover");
  await expect(page.locator("#ed-sec-layers-body")).toHaveClass(/drop/);
  await bridge.emit("files-dropped", ["/art/glow.png"]);
  await expect(page.locator("#ed-sec-layers-body")).not.toHaveClass(/drop/);
  await page.getByRole("button", { name: "Cancel" }).filter({ visible: true }).click();

  // Another mode is not the editor's to answer
  await page.keyboard.press("Escape");
  await editor.page.locator('[data-mode="single"]').first().click();
  await bridge.emit("files-dropped", ["/art/glow.png"]);
  await expect(dialog(page)).toBeHidden();
});

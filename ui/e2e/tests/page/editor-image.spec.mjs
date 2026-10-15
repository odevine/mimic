import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, describeView, standardItems } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The template is authored at 800, larger than the preview's 150, so there is a full size
// picture to offer as well as the one shown
async function ready(bridge, page) {
  bridge.reply("Studio.Describe", () => describeView(standardItems));
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine));
  bridge.reply("Render.SuggestedFilename", "Mirran Spy.png");
  bridge.reply("Render.Save", null);
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  return editor;
}

const save = (page) => page.locator("#ed-save-image");
const saveAs = async (page, name) => {
  await save(page).click();
  await page.getByRole("menuitem", { name }).click();
};

test("the picture shown is saved where the dialog says, once there is one, and not when it was cancelled or failed", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(save(page)).toBeDisabled();
  await bridge.job("job-1", [done(1)]);
  await expect(save(page)).toBeEnabled();
  await save(page).click();
  await expect(page.getByRole("menuitem")).toHaveText(["As shown", "At full size"]);
  await page.keyboard.press("Escape");

  bridge.reply("System.PickFile", "");
  await saveAs(page, "As shown");
  await expect.poll(() => bridge.calledWith("System.PickFile").length).toBe(1);
  expect(bridge.calledWith("Render.Save")).toEqual([]);

  bridge.reply("System.PickFile", "/ro/card.png");
  bridge.fail("Render.Save", "saving the image: permission denied");
  await saveAs(page, "As shown");
  await expect(page.locator(".toast.err")).toContainText("permission denied");

  // A change draws the card again, and the picture saved is the new one
  bridge.reply("Render.Save", null);
  bridge.reply("System.PickFile", "/out/Mirran Spy.png");
  await editor.row("title").locator('[data-toggle="visible"]').click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await bridge.job("job-2", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await saveAs(page, "As shown");
  await expect(page.locator("#toasts .toast").last()).toHaveText("Saved Mirran Spy.png");
  expect(bridge.calledWith("System.PickFile").at(-1)[0]).toMatchObject({ save: true, name: "Mirran Spy.png", filters: [{ name: "PNG image", pattern: "*.png" }] });
  expect(bridge.calledWith("Render.Save").at(-1)).toEqual(["job-2", "/out/Mirran Spy.png"]);

  // A render that fails leaves no picture to save
  await editor.row("frame").locator('[data-toggle="visible"]').click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(3);
  await bridge.job("job-3", [{ seq: 1, done: true, error: "a layer image is missing" }]);
  await expect(save(page)).toBeDisabled();
});

test("full size draws the card at the layout's own resolution and saves it, leaving the preview, or says why it failed", async ({ bridge, page }) => {
  bridge.reply("System.PickFile", "/out/Mirran Spy.png");
  await ready(bridge, page);
  await bridge.job("job-1", [done(1)]);
  const shown = await page.locator("#ed-img").getAttribute("src");
  await saveAs(page, "At full size");
  await expect(save(page)).toBeDisabled();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  expect(bridge.calledWith("Studio.Render")[1][0]).toMatchObject({ full: true, coverage: false });
  expect(bridge.calledWith("Studio.Render")[0][0].full).toBe(false);
  await bridge.job("job-2", [done(1)]);
  await expect(page.locator("#toasts .toast").last()).toHaveText("Saved Mirran Spy.png");
  expect(bridge.calledWith("Render.Save")).toEqual([["job-2", "/out/Mirran Spy.png"]]);
  await expect(page.locator("#ed-img")).toHaveAttribute("src", shown);
  await expect(save(page)).toBeEnabled();

  await saveAs(page, "At full size");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(3);
  await bridge.job("job-3", [{ seq: 1, done: true, error: "out of memory" }]);
  await expect(page.locator(".toast.err")).toContainText("out of memory");
  await expect(save(page)).toBeEnabled();
});

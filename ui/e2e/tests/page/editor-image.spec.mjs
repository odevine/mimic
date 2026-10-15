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
  return editor;
}

const full = (page) => page.locator("#ed-render-full");
const save = (page) => page.locator("#ed-save-image");

test("the card's picture can be saved and drawn at full size once there is one, and not before", async ({ bridge, page }) => {
  await ready(bridge, page);
  await expect(full(page)).toBeDisabled();
  await expect(save(page)).toBeDisabled();
  await bridge.job("job-1", [done(1)]);
  await expect(full(page)).toBeEnabled();
  await expect(save(page)).toBeEnabled();
  await expect(full(page)).toHaveAttribute("data-tip", "Render at full size");
  await expect(save(page)).toHaveAttribute("data-tip", "Save image");
  await expect(page.locator("#ed-copy-image")).toHaveCount(0);
});

test("saving asks where to put the picture and writes the render that is shown", async ({ bridge, page }) => {
  bridge.reply("Render.SuggestedFilename", "Mirran Spy.png");
  bridge.reply("System.PickFile", "/out/Mirran Spy.png");
  bridge.reply("Render.Save", null);
  await ready(bridge, page);
  await bridge.job("job-1", [done(1)]);
  await expect(save(page)).toBeEnabled();
  await save(page).click();
  await expect(page.locator("#toasts .toast").last()).toHaveText("Saved Mirran Spy.png");
  expect(bridge.calledWith("Render.SuggestedFilename")).toEqual([["job-1"]]);
  const [pick] = bridge.calledWith("System.PickFile")[0];
  expect(pick).toMatchObject({ save: true, name: "Mirran Spy.png", filters: [{ name: "PNG image", pattern: "*.png" }] });
  expect(bridge.calledWith("Render.Save")).toEqual([["job-1", "/out/Mirran Spy.png"]]);
});

test("a save that is cancelled writes nothing and one that fails says why", async ({ bridge, page }) => {
  bridge.reply("Render.SuggestedFilename", "Mirran Spy.png");
  bridge.reply("System.PickFile", "");
  bridge.reply("Render.Save", null);
  await ready(bridge, page);
  await bridge.job("job-1", [done(1)]);
  await save(page).click();
  await expect.poll(() => bridge.calledWith("System.PickFile").length).toBe(1);
  expect(bridge.calledWith("Render.Save")).toEqual([]);
  await expect(page.locator("#toasts .toast")).toHaveCount(0);

  bridge.reply("System.PickFile", "/ro/card.png");
  bridge.fail("Render.Save", "saving the image: permission denied");
  await save(page).click();
  await expect(page.locator(".toast.err")).toContainText("permission denied");
});

test("full size draws the card at the layout's own resolution, then asks where to put it", async ({ bridge, page }) => {
  bridge.reply("Render.SuggestedFilename", "Mirran Spy.png");
  bridge.reply("System.PickFile", "/out/Mirran Spy.png");
  bridge.reply("Render.Save", null);
  await ready(bridge, page);
  await bridge.job("job-1", [done(1)]);
  const shown = await page.locator("#ed-img").getAttribute("src");
  await expect(full(page)).toBeEnabled();
  await full(page).click();
  await expect(full(page)).toBeDisabled();
  await expect(full(page)).toHaveAttribute("aria-busy", "true");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  const [req] = bridge.calledWith("Studio.Render")[1];
  expect(req).toMatchObject({ full: true, coverage: false });
  // The preview's own render did not ask for it
  expect(bridge.calledWith("Studio.Render")[0][0].full).toBe(false);
  await bridge.job("job-2", [done(1)]);
  await expect(page.locator("#toasts .toast").last()).toHaveText("Saved Mirran Spy.png");
  expect(bridge.calledWith("Render.Save")).toEqual([["job-2", "/out/Mirran Spy.png"]]);
  // The card on show is still the preview, and the button is ready again
  await expect(page.locator("#ed-img")).toHaveAttribute("src", shown);
  await expect(full(page)).toBeEnabled();
  await expect(full(page)).toHaveAttribute("aria-busy", "false");
});

test("a full size render that is not saved or that fails leaves the preview and says why", async ({ bridge, page }) => {
  bridge.reply("Render.SuggestedFilename", "Mirran Spy.png");
  bridge.reply("System.PickFile", "");
  bridge.reply("Render.Save", null);
  await ready(bridge, page);
  await bridge.job("job-1", [done(1)]);
  await full(page).click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await bridge.job("job-2", [done(1)]);
  await expect.poll(() => bridge.calledWith("System.PickFile").length).toBe(1);
  expect(bridge.calledWith("Render.Save")).toEqual([]);
  await expect(page.locator("#toasts .toast")).toHaveCount(0);
  await expect(full(page)).toBeEnabled();

  await full(page).click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(3);
  await bridge.job("job-3", [{ seq: 1, done: true, error: "out of memory" }]);
  await expect(page.locator(".toast.err")).toContainText("out of memory");
  await expect(full(page)).toBeEnabled();
});

test("what is saved is the render that is shown now, and nothing when there is none", async ({ bridge, page }) => {
  bridge.reply("Render.SuggestedFilename", "Mirran Spy.png");
  bridge.reply("System.PickFile", "/out/a.png");
  bridge.reply("Render.Save", null);
  const editor = await ready(bridge, page);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  // A change draws the card again, and the picture that is saved is the new one once it is there
  await editor.row("title").locator('[data-toggle="visible"]').click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await bridge.job("job-2", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await save(page).click();
  await expect.poll(() => bridge.calledWith("Render.Save").length).toBe(1);
  expect(bridge.calledWith("Render.Save")[0][0]).toBe("job-2");
  // A render that fails leaves no picture to copy or save
  await editor.row("frame").locator('[data-toggle="visible"]').click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(3);
  await bridge.job("job-3", [{ seq: 1, done: true, error: "a layer image is missing" }]);
  await expect(editor.activity).toHaveAttribute("data-state", "error");
  await expect(full(page)).toBeDisabled();
  await expect(save(page)).toBeDisabled();
});

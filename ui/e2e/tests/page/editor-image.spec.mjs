import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, describeView, standardItems } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// A clipboard that keeps what it is given, as the page sees it, since a test cannot read the system's
test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    window.__clip = [];
    if (typeof ClipboardItem === "undefined") {
      window.ClipboardItem = class {
        constructor(map) {
          this.map = map;
          this.types = Object.keys(map);
        }
        async getType(type) {
          return await this.map[type];
        }
      };
    }
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        write: async (items) => {
          if (window.__clipFails) throw new Error(window.__clipFails);
          for (const item of items) {
            const got = {};
            for (const type of item.types) {
              const blob = await item.getType(type);
              got[type] = { size: blob.size, type: blob.type };
            }
            window.__clip.push(got);
          }
        },
      },
    });
  });
});

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

const copy = (page) => page.locator("#ed-copy-image");
const save = (page) => page.locator("#ed-save-image");

test("the card's picture can be copied and saved once there is one, and not before", async ({ bridge, page }) => {
  await ready(bridge, page);
  await expect(copy(page)).toBeDisabled();
  await expect(save(page)).toBeDisabled();
  await bridge.job("job-1", [done(1)]);
  await expect(copy(page)).toBeEnabled();
  await expect(save(page)).toBeEnabled();
  await expect(copy(page)).toHaveAttribute("data-tip", "Copy image");
  await expect(save(page)).toHaveAttribute("data-tip", "Save image");
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

test("copying puts the picture on the clipboard as a PNG", async ({ bridge, page }) => {
  await ready(bridge, page);
  await bridge.job("job-1", [done(1)]);
  await expect(copy(page)).toBeEnabled();
  await copy(page).click();
  await expect(page.locator("#toasts .toast").last()).toHaveText("Copied the image");
  const clip = await page.evaluate(() => window.__clip);
  expect(clip).toHaveLength(1);
  expect(Object.keys(clip[0])).toEqual(["image/png"]);
  expect(clip[0]["image/png"].size).toBeGreaterThan(0);
  expect(clip[0]["image/png"].type).toBe("image/png");
});

test("a copy the window refuses, or cannot make, says so and points to saving", async ({ bridge, page }) => {
  await ready(bridge, page);
  await bridge.job("job-1", [done(1)]);
  await page.evaluate(() => {
    window.__clipFails = "not allowed";
  });
  await copy(page).click();
  await expect(page.locator(".toast.err")).toContainText("Could not copy the image: not allowed");
  await page.evaluate(() => {
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: undefined });
  });
  await copy(page).click();
  await expect(page.locator(".toast.warn")).toContainText("Save image writes it to a file");
});

test("what is copied and saved is the render that is shown now, and nothing when there is none", async ({ bridge, page }) => {
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
  await expect(copy(page)).toBeDisabled();
  await expect(save(page)).toBeDisabled();
});

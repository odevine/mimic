import { test, expect } from "../../bridge-fixtures.mjs";
import { SinglePage } from "../../pages/SinglePage.mjs";
import { done, progress, result } from "../../bridge/cards.mjs";

// Single mode with one card found, ready to render
async function withACard(bridge, page) {
  bridge.reply("Cards.Search", [result("Sol Ring")]);
  bridge.reply("Cards.Printings", []);
  await bridge.boot();
  const single = new SinglePage(page);
  await single.search("sol ring");
  await single.select("Sol Ring");
  return single;
}

test("a search that fails says why and leaves nothing to edit", async ({ bridge, page }) => {
  bridge.fail("Cards.Search", "search failed: scryfall answered 503");
  await bridge.boot();
  await page.locator("#search-input").fill("sol ring");
  await page.locator("#search-input").press("Enter");
  await expect(page.locator("#results-count")).toHaveText("Search failed");
  await expect(page.locator("#status")).toHaveText("Search failed: search failed: scryfall answered 503");
  await expect(page.locator("#editor")).toBeHidden();
});

// The preview of a selected card is a render of its own, so the first job
// belongs to the selection, and a save starts the next
test("a render that fails reports the error and can be tried again", async ({ bridge, page }) => {
  const single = await withACard(bridge, page);
  await expect.poll(() => bridge.calledWith("Render.Start").length).toBe(1);
  await bridge.job("job-1", [progress(1, "Compositing", 0.4), done(2, { error: "template normal has no layer for this card" })]);

  await expect(single.activity).toHaveAttribute("data-state", "error");
  await expect(page.locator("#activity-title")).toHaveText("Render failed");
  await expect(page.locator("#activity-step")).toContainText("template normal has no layer for this card");
  await expect(single.renderButton).toBeEnabled();

  // Rendering again starts a new job, which can succeed
  await single.renderButton.click();
  await expect.poll(() => bridge.calledWith("Render.Start").length).toBe(2);
  await bridge.job("job-2", [progress(1, "Compositing", 0.9), done(2)]);
  await expect(single.activity).toHaveAttribute("data-state", "done");
});

test("a save that fails says so and writes nothing", async ({ bridge, page }) => {
  const single = await withACard(bridge, page);
  await expect.poll(() => bridge.calledWith("Render.Start").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(single.activity).toHaveAttribute("data-state", "done");

  bridge.reply("Render.SuggestedFilename", "Sol Ring");
  bridge.reply("System.PickFile", "/tmp/Sol Ring.png");
  bridge.fail("Render.Save", "saving the image: permission denied");
  // The preview is already at the output size, so the save writes it as it is
  await single.save();

  await expect(page.locator("#activity-title")).toHaveText("Save failed");
  await expect(page.locator("#activity-step")).toContainText("permission denied");
  expect(bridge.calledWith("Render.Save")).toHaveLength(1);
  expect(bridge.calledWith("Render.Start")).toHaveLength(1);
});

test("a save dialog that is cancelled leaves the card unsaved and quiet", async ({ bridge, page }) => {
  const single = await withACard(bridge, page);
  await expect.poll(() => bridge.calledWith("Render.Start").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(single.activity).toHaveAttribute("data-state", "done");

  bridge.reply("Render.SuggestedFilename", "Sol Ring");
  bridge.reply("System.PickFile", "");
  await single.save();

  await expect(single.activity).toHaveAttribute("data-state", "idle");
  expect(bridge.calledWith("Render.Save")).toHaveLength(0);
});

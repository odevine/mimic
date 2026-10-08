import { readdir } from "node:fs/promises";
import { test, expect } from "../../bridge-fixtures.mjs";
import { scenes } from "../../bridge/scenes.mjs";
import { snapshotDir } from "../../visual.mjs";

// Every scene is compared with its baseline in both themes. The pages are drawn
// from replies set in the test, so only the browser's own drawing can change a
// screenshot, which is why the baselines are made on one platform (Linux in CI)
// and the comparison runs only there. The card image is a stand-in and anything
// that shows a time is masked
const baselines = await readdir(snapshotDir).catch(() => []);
const hasBaselines = baselines.some((f) => f.endsWith(".png"));
test.beforeEach(({}, testInfo) => {
  test.skip(!hasBaselines && testInfo.config.updateSnapshots !== "all", "no baselines yet: run the Update visual baselines workflow and commit what it makes to ui/testassets/visual");
});

for (const theme of ["dark", "light"]) {
  for (const [name, scene] of Object.entries(scenes)) {
    test(`${name} looks the same in the ${theme} theme`, async ({ bridge, page }) => {
      await page.emulateMedia({ reducedMotion: "reduce", colorScheme: theme });
      bridge.reply("Settings.Get", { theme });
      await scene.run({ bridge, page });
      await page.evaluate(() => document.fonts.ready);
      await expect(page).toHaveScreenshot(`${name}-${theme}.png`, {
        animations: "disabled",
        caret: "hide",
        mask: (scene.mask || []).map((selector) => page.locator(selector)),
      });
    });
  }
}

test("the page at its smallest size looks the same", async ({ bridge, page }) => {
  await page.setViewportSize({ width: 720, height: 520 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await scenes["single-card"].run({ bridge, page });
  await page.evaluate(() => document.fonts.ready);
  await expect(page).toHaveScreenshot("single-card-smallest.png", {
    animations: "disabled",
    caret: "hide",
    mask: scenes["single-card"].mask.map((selector) => page.locator(selector)),
  });
});

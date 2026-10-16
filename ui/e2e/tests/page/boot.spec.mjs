import { test, expect, noTemplate } from "../../bridge-fixtures.mjs";

test("the page boots from the replies of the real app", async ({ bridge, page }) => {
  noTemplate(bridge);
  await bridge.boot();
  await expect(page.locator("#template-label")).toHaveText("No template installed");
  await expect(page.locator("#search-input")).toBeFocused();
  await expect(page.locator("#single-empty")).toBeVisible();
  await expect(page.locator("#boot-error")).toBeHidden();
});

test("scripts that cannot be loaded leave a message and not a blank page", async ({ bridge, page }) => {
  await page.route("http://app.test/js/app.js", (route) => route.fulfill({ status: 404, body: "not found" }));
  await page.goto("http://app.test/");
  await expect(page.locator("#boot-error")).toBeVisible();
  await expect(page.locator("#boot-error-detail")).toContainText("could not be loaded");
  await expect(page.locator("#shell")).toBeHidden();
});

test("settings and capabilities that cannot be read leave the defaults and every keyed control closed", async ({ bridge, page }) => {
  bridge.fail("Settings.Get", "the settings file is unreadable");
  bridge.fail("Settings.Capabilities", "no capabilities");
  await bridge.boot();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.locator("#palette-btn")).toHaveClass(/gated/);
  await expect(page.locator("#settings-btn")).not.toHaveClass(/gated/);
});


import { test, expect } from "../../bridge-fixtures.mjs";

test("the page boots from the replies of the real app", async ({ bridge, page }) => {
  await bridge.boot();
  await expect(page.locator("#template-label")).toHaveText("normal (placeholder)");
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

test("settings that cannot be read leave the defaults", async ({ bridge, page }) => {
  bridge.fail("Settings.Get", "the settings file is unreadable");
  await bridge.boot();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});

test("capabilities that cannot be read leave every keyed control closed", async ({ bridge, page }) => {
  bridge.fail("Settings.Capabilities", "no capabilities");
  await bridge.boot();
  await expect(page.locator("#palette-btn")).toHaveClass(/gated/);
  await expect(page.locator("#settings-btn")).not.toHaveClass(/gated/);
});

test("a feature the app has not built is shown as closed with its reason", async ({ bridge, page }) => {
  bridge.reply("Settings.Capabilities", {
    ...bridge.defaults["Settings.Capabilities"],
    palette: { state: "planned", reason: "The command palette is planned. Tracked in #82" },
  });
  await bridge.boot();
  const palette = page.locator("#palette-btn");
  await expect(palette).toHaveClass(/gated/);
  await expect(palette).toHaveAttribute("aria-disabled", "true");
  await expect(palette).toHaveAttribute("data-gate-reason", /command palette is planned/);
});

test("the theme comes from the settings the app keeps", async ({ bridge, page }) => {
  bridge.reply("Settings.Get", { theme: "light" });
  await bridge.boot();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
});

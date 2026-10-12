import { test, expect } from "../../fixtures.mjs";

// These tests install templates, which the app keeps for good, so they run on
// the isolated server and in the order of this file

test("a catalog with no templates lists none", async ({ templates, fake }) => {
  await templates.show();
  await expect(templates.libraryList.locator(".tp-template")).toHaveCount(0);
  await expect(templates.label).toHaveText("No template installed");
});

test("downloading a template installs it, makes it active and renders with it", async ({ templates, single, fake, page }) => {
  await fake.catalog([{ name: "test.normal", version: "1.0.0" }, { name: "test.transform", version: "1.0.0" }]);
  await templates.show();
  await expect(templates.libraryList.locator(".tp-template")).toHaveCount(2);
  const normal = templates.entry("test.normal");
  await expect(normal).toContainText("Flat colors where a frame's art goes");
  // The catalog says which layouts each one renders, before either is installed
  await expect(normal.locator(".tp-chip")).toHaveText(["Standard"]);
  await expect(templates.entry("test.transform").locator(".tp-chip")).toHaveText(["Transform"]);
  await expect(normal.getByRole("button", { name: /^Download · \d+ KB$/ })).toBeVisible();

  await normal.getByRole("button", { name: "Download" }).click();
  await expect(templates.label).toHaveText("Test frame · 1.0.0");
  await expect(normal).toContainText("active");
  await expect(normal.locator(".tp-chip.on")).toHaveText(["Standard"]);

  // The bundle is a real template, so a card renders with it
  await single.show();
  await single.search("sol ring");
  await single.select("Sol Ring");
  await single.render();
  await expect(single.preview).toBeVisible();
  expect((await fake.requests()).some((r) => r.endsWith(".mimic"))).toBe(true);
});

test("a template for transform cards makes them supported", async ({ templates, list, fake }) => {
  await fake.catalog([{ name: "test.normal", version: "1.0.0" }, { name: "test.transform", version: "1.0.0" }]);
  await list.paste("1 Delver of Secrets");
  await list.resolve();
  await expect(list.row("Delver of Secrets")).toContainText("unsupported");

  await templates.show();
  await templates.entry("test.transform").getByRole("button", { name: "Download" }).click();
  await expect(templates.entry("test.transform")).toContainText("installed");

  await list.show();
  await list.paste("1 Delver of Secrets");
  await list.resolve();
  await expect(list.row("Delver of Secrets")).toContainText("matched");
});

test("an installed template offers the choices it declares", async ({ templates, fake, page }) => {
  await fake.catalog([{ name: "test.featured", version: "1.0.0", features: true }]);
  await templates.show();
  const entry = templates.entry("test.featured");
  await expect(entry.locator(".tp-feature")).toHaveCount(0);

  await entry.getByRole("button", { name: "Download" }).click();
  await expect(entry).toContainText("active");
  await expect(entry.locator(".tp-feature")).toHaveCount(3);
  await expect(entry).toContainText("Test frame");

  // A choice is kept as soon as it is made, and is still there on a reload of the list
  await entry.getByLabel("Banner").check();
  await entry.getByLabel("Tone").selectOption("dark");
  await templates.showDefaults();
  await page.locator("#tm-tab-library").click();
  await expect(templates.entry("test.featured").getByLabel("Banner")).toBeChecked();
  await expect(templates.entry("test.featured").getByLabel("Tone")).toHaveValue("dark");
});

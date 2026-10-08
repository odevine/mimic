import { test, expect } from "../../fixtures.mjs";

// These tests install templates, which the app keeps for good, so they run on
// the isolated server and in the order of this file

test("a catalog with no templates lists none", async ({ templates, fake }) => {
  await templates.show();
  await expect(templates.libraryList.locator(".tp-template")).toHaveCount(0);
  await expect(templates.label).toHaveText("normal (placeholder)");
});

test("downloading a template installs it, makes it active and renders with it", async ({ templates, single, fake, page }) => {
  await fake.catalog([{ name: "normal", version: "1.0.0" }, { name: "transform", version: "1.0.0" }]);
  await templates.show();
  await expect(templates.libraryList.locator(".tp-template")).toHaveCount(2);
  const normal = templates.entry("normal");
  await expect(normal).toContainText("The standard, modern Magic card frame");

  await normal.getByRole("button", { name: "Download" }).click();
  await expect(templates.label).toHaveText("normal · 1.0.0");
  await expect(normal).toContainText("active");

  // The bundle is a real template, so a card renders with it
  await single.show();
  await single.search("sol ring");
  await single.select("Sol Ring");
  await single.render();
  await expect(single.preview).toBeVisible();
  expect((await fake.requests()).some((r) => r.endsWith(".mimic"))).toBe(true);
});

test("a template for transform cards makes them supported", async ({ templates, list, fake }) => {
  await fake.catalog([{ name: "normal", version: "1.0.0" }, { name: "transform", version: "1.0.0" }]);
  await list.paste("1 Delver of Secrets");
  await list.resolve();
  await expect(list.row("Delver of Secrets")).toContainText("unsupported");

  await templates.show();
  await templates.entry("transform").getByRole("button", { name: "Download" }).click();
  await expect(templates.entry("transform")).toContainText("installed");

  await list.show();
  await list.paste("1 Delver of Secrets");
  await list.resolve();
  await expect(list.row("Delver of Secrets")).toContainText("matched");
});

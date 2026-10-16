import { test, expect } from "../../bridge-fixtures.mjs";
import { SettingsDialog } from "../../pages/SettingsDialog.mjs";

test("settings that cannot be saved say so and stay open, and save once they can", async ({ bridge, page }) => {
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await settings.show();
  await settings.theme.selectOption("light");

  bridge.fail("Settings.Put", "the settings file is read only");
  await page.locator("#settings-save").click();
  await expect(page.locator("#settings-status")).toHaveText("Failed: the settings file is read only");
  await expect(settings.dialog).toBeVisible();
  await expect(settings.theme).toHaveValue("light");

  bridge.reply("Settings.Put", (s) => s);
  await settings.save();
  await expect(page.locator(".toast.ok")).toContainText("Settings saved");
  expect(bridge.calledWith("Settings.Put").at(-1)[0]).toMatchObject({ theme: "light" });
});

test("fonts that cannot be read are said so in place of the list, and saving leaves the assignments alone", async ({ bridge, page }) => {
  bridge.reply("Settings.Get", { fontOverrides: { body: "Gotham Medium" } });
  bridge.fail("Data.Fonts", "the fonts folder cannot be read");
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await settings.show();
  await expect(settings.fontsSource).toHaveText("Could not read the fonts: the fonts folder cannot be read");
  await expect(settings.fontRoles).toHaveCount(0);
  await settings.save();
  expect(bridge.calledWith("Settings.Put").at(-1)[0].fontOverrides).toEqual({ body: "Gotham Medium" });
});

// A Fonts reply with two roles and one font in the folder, which a test changes
const role = (name, label, own, extra) => ({ role: name, label, source: "template", family: own.split(" ")[0], own, ...extra });
const GOTHAM = { name: "Gotham Medium", family: "Gotham", style: "Medium", file: "G.otf", removable: true };
const fontsReply = (extra) => ({
  kind: "managed",
  path: "/fonts",
  writable: true,
  roles: [role("title", "Card name", "Beleren Proxy Bold"), role("body", "Rules text", "Plantin MT Pro Regular")],
  pool: [GOTHAM],
  rejected: [],
  duplicates: [],
  ...extra,
});

test("a role whose assigned font is gone says so and keeps the assignment", async ({ bridge, page }) => {
  bridge.reply("Settings.Get", { fontOverrides: { body: "Lost Font Regular" } });
  bridge.reply(
    "Data.Fonts",
    fontsReply({ roles: [role("title", "Card name", "Beleren Proxy Bold"), role("body", "Rules text", "Plantin MT Pro Regular", { source: "template-missing-override", override: "Lost Font Regular" })], pool: [] }),
  );
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await settings.show();
  await expect(settings.fontPick("Rules text")).toHaveValue("Lost Font Regular");
  await expect(settings.fontRole("Rules text").locator(".fonts-role-state.warn")).toContainText("Lost Font Regular is not in the folder");

  await settings.save();
  expect(bridge.calledWith("Settings.Put").at(-1)[0].fontOverrides).toEqual({ body: "Lost Font Regular" });
});

test("the font picked for a role is saved with the settings, beside the roles not shown", async ({ bridge, page }) => {
  bridge.reply("Settings.Get", { fontOverrides: { "saga-chapter": "Gotham Medium", title: "Gotham Medium" } });
  bridge.reply("Data.Fonts", fontsReply({ roles: [role("title", "Card name", "Beleren Proxy Bold", { source: "override", override: "Gotham Medium", file: "G.otf" }), role("body", "Rules text", "Plantin MT Pro Regular")] }));
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await settings.show();
  await expect(settings.fontPick("Card name")).toHaveValue("Gotham Medium");
  await expect(settings.fontPick("Rules text")).toHaveValue("");

  await settings.fontPick("Card name").selectOption("");
  await settings.fontPick("Rules text").selectOption("Gotham Medium");
  await settings.save();
  expect(bridge.calledWith("Settings.Put").at(-1)[0].fontOverrides).toEqual({ "saga-chapter": "Gotham Medium", body: "Gotham Medium" });
});


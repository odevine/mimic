import { test, expect } from "../../bridge-fixtures.mjs";
import { SettingsDialog } from "../../pages/SettingsDialog.mjs";

test("settings that cannot be saved say so and stay open with the choices made", async ({ bridge, page }) => {
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await settings.show();
  await settings.theme.selectOption("light");

  bridge.fail("Settings.Put", "the settings file is read only");
  await page.locator("#settings-save").click();
  await expect(page.locator("#settings-status")).toHaveText("Failed: the settings file is read only");
  await expect(settings.dialog).toBeVisible();
  await expect(settings.theme).toHaveValue("light");
});

test("settings that save are kept and acknowledged", async ({ bridge, page }) => {
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await settings.show();
  await settings.theme.selectOption("light");
  await settings.save();
  await expect(page.locator(".toast.ok")).toContainText("Settings saved");
  expect(bridge.calledWith("Settings.Put").at(-1)[0]).toMatchObject({ theme: "light" });
});

test("fonts that cannot be read are said so in place of the list", async ({ bridge, page }) => {
  bridge.fail("Data.Fonts", "the fonts folder cannot be read");
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await settings.show();
  await expect(settings.fontsSource).toHaveText("Could not read the fonts: the fonts folder cannot be read");
  await expect(settings.fontRoles).toHaveCount(0);
});

test("a font role that cannot be read is flagged with the reason", async ({ bridge, page }) => {
  const fonts = bridge.defaults["Data.Fonts"];
  const [first, ...rest] = fonts.roles;
  bridge.reply("Data.Fonts", { ...fonts, roles: [{ ...first, source: "user", rejected: 1, error: "not a font file" }, ...rest] });
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await settings.show();
  await expect(page.locator(".fonts-note.err").first()).toContainText("1 could not be read: not a font file");
});

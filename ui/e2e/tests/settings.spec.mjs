import { readFile } from "node:fs/promises";
import { test, expect } from "../fixtures.mjs";

const FONT = { path: new URL("../../../engine/fonts/embedded/m15/Gotham-Medium.otf", import.meta.url).pathname };

test("the theme is applied, kept across a reload and discarded on cancel", async ({ settings, page }) => {
  const html = page.locator("html");
  await expect(html).toHaveAttribute("data-theme", "dark");

  await settings.show();
  await settings.theme.selectOption("light");
  await settings.cancel();
  await expect(html).toHaveAttribute("data-theme", "dark");

  await settings.show();
  await settings.theme.selectOption("light");
  await settings.save();
  await expect(html).toHaveAttribute("data-theme", "light");

  await page.reload();
  await page.locator("html.booted").waitFor();
  await expect(html).toHaveAttribute("data-theme", "light");
});

test("the image format setting decides the files a run writes", async ({ settings, list, scratch }) => {
  await settings.show();
  const png = await settings.imageFormat.locator("option", { hasText: /^PNG/ }).getAttribute("value");
  await settings.imageFormat.selectOption(png);
  await settings.save();

  await list.paste("1 Sol Ring");
  await list.resolve();
  await list.chooseOutput();
  await list.render();
  await expect.poll(async () => (await scratch.chosenFolder()).filter((f) => /\.(png|jpg)$/.test(f))).toEqual(["Sol Ring [C21-263].png"]);
  const head = (await readFile(scratch.folderPath("Sol Ring [C21-263].png"))).subarray(0, 4);
  expect(head.equals(Buffer.from([0x89, 0x50, 0x4e, 0x47]))).toBe(true);
});

test("a font added to the folder can be given to a role, and says so when it goes", async ({ settings, scratch }) => {
  await settings.show();
  await expect(settings.fontsSource).toContainText("kept in the app's fonts folder");
  const rules = settings.fontRole("Rules text");
  await expect(rules).toContainText("Built in");
  await expect(settings.fontPick("Rules text").locator("option")).toHaveCount(1);

  await scratch.offer("Gotham-Medium.otf", FONT);
  await settings.addFont.click();
  await expect(settings.fontFile("Gotham Medium")).toBeVisible();
  await expect(settings.fontPick("Rules text").locator("option")).toHaveCount(2);

  await settings.fontPick("Rules text").selectOption("Gotham Medium");
  await expect(rules).toContainText("Your font, Gotham-Medium.otf");
  await settings.save();

  await settings.show();
  await expect(settings.fontPick("Rules text")).toHaveValue("Gotham Medium");
  await expect(rules).toContainText("Your font, Gotham-Medium.otf");

  await settings.fontFile("Gotham Medium").getByRole("button", { name: "Remove Gotham Medium" }).click();
  await expect(settings.fontsPool).toHaveCount(0);
  await expect(rules).toContainText("Gotham Medium is not in the folder");
  await expect(settings.resetFonts).toBeHidden();
});

test("a font dialog that is cancelled changes nothing, and a font picked for a role is dropped when the settings are", async ({ settings, scratch }) => {
  await settings.show();
  await settings.addFont.click();
  await expect(settings.fontsPool).toHaveCount(0);
  await expect(settings.fontPick("Rules text").locator("option")).toHaveCount(1);

  await scratch.offer("Gotham-Medium.otf", FONT);
  await settings.addFont.click();
  await settings.fontPick("Rules text").selectOption("Gotham Medium");
  await settings.cancel();

  await settings.show();
  await expect(settings.fontPick("Rules text")).toHaveValue("");
});

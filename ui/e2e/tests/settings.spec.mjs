import { readFile } from "node:fs/promises";
import { test, expect } from "../fixtures.mjs";

const FONT = { path: new URL("../../testassets/fonts/BigShoulders-Bold.ttf", import.meta.url).pathname };

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

test("a font added for a role is kept and can be removed", async ({ settings, scratch }) => {
  await settings.show();
  await expect(settings.fontsSource).toContainText("kept in the app's fonts folder");
  const title = settings.fontRole("Title");
  await expect(title).toContainText("Default");

  await scratch.offer("BigShoulders-Bold.ttf", FONT);
  await title.getByRole("button", { name: "Add" }).click();
  await expect(title).toContainText("Yours");
  await expect(title).toContainText("Big Shoulders");

  await title.getByRole("button", { name: "Remove" }).click();
  await expect(title).toContainText("Default");
  await expect(settings.resetFonts).toBeHidden();
});

test("a font dialog that is cancelled changes nothing", async ({ settings }) => {
  await settings.show();
  const title = settings.fontRole("Title");
  await title.getByRole("button", { name: "Add" }).click();
  await expect(title).toContainText("Default");
});

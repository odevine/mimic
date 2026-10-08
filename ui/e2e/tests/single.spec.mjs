import { readFile, stat } from "node:fs/promises";
import { test, expect } from "../fixtures.mjs";
import { saved, savedPath, events } from "../scratch.mjs";

const PNG = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

test("a search finds every printing and groups them under one card", async ({ single }) => {
  await single.search("lightning bolt");
  await expect(single.results).toHaveCount(1);
  await expect(single.results.first().locator(".r-name")).toHaveText("Lightning Bolt");
  await expect(single.results.first().locator(".prints")).toHaveText("2 printings");
});

test("a search with no match says so and offers nothing to edit", async ({ single }) => {
  await single.search("zzzz");
  await expect(single.page.locator("#results .results-note")).toContainText("No cards match");
  await expect(single.editor).toBeHidden();
});

test("search, edit, render and save writes a PNG", async ({ single }) => {
  await single.search("llanowar elves");
  await single.select("Llanowar Elves");

  await expect(single.title).toHaveText("Llanowar Elves");
  await expect(single.field("name")).toHaveValue("Llanowar Elves");
  await expect(single.field("typeLine")).toHaveValue("Creature — Elf Druid");
  await expect(single.field("power")).toHaveValue("1");

  // An edit marks its field and leads the preview to redraw with it
  await single.field("flavor").fill("Roots first, then everything else.");
  await expect(single.fieldRow("flavor")).toHaveClass(/dirty/);

  await single.render();
  await expect(single.preview).toBeVisible();

  await single.save();
  await expect(single.activity).toHaveAttribute("data-state", "done", { timeout: 120_000 });
  await expect(single.page.locator("#activity-title")).toHaveText("Saved Llanowar Elves.png");

  expect(await saved()).toEqual(["Llanowar Elves.png"]);
  const file = savedPath("Llanowar Elves.png");
  const head = (await readFile(file)).subarray(0, 8);
  expect(head.equals(PNG)).toBe(true);
  expect((await stat(file)).size).toBeGreaterThan(10_000);
  expect((await events()).map(([kind]) => kind)).toContain("save");
});

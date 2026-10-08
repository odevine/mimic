import { test, expect } from "../fixtures.mjs";

// An id must belong to one element, or a label's for points at either of two
// inputs and an accessibility scan reports it. The Single editor and the list
// inspector once built their fields with the same ids, so the page is checked
// with both of them present, and with the other panels the page builds
const duplicates = (page) =>
  page.evaluate(() => {
    const seen = new Map();
    for (const el of document.querySelectorAll("[id]")) seen.set(el.id, (seen.get(el.id) || 0) + 1);
    return [...seen].filter(([, n]) => n > 1).map(([id, n]) => `${id} x${n}`);
  });

test("no id is used twice on the page", async ({ single, list, page }) => {
  expect(await duplicates(page), "on load").toEqual([]);

  // The list fixture opens the list, so Single is shown again to use it
  await single.show();
  await single.search("llanowar elves");
  await single.select("Llanowar Elves");
  expect(await duplicates(page), "with the Single editor filled").toEqual([]);

  await list.show();
  await list.paste("1 Llanowar Elves\n1 Sol Ring");
  await list.resolve();
  await list.edit("Llanowar Elves");
  expect(await duplicates(page), "with the list inspector open beside the Single editor").toEqual([]);

  await page.locator("#settings-btn").click();
  expect(await duplicates(page), "with the settings dialog open").toEqual([]);
});

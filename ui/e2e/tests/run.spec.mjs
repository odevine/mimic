import { readFile } from "node:fs/promises";
import { test, expect } from "../fixtures.mjs";

const JPEG = Buffer.from([0xff, 0xd8, 0xff]);

test("a list renders to the chosen folder and the run console reports every card", async ({ list, run, scratch }) => {
  await list.paste("3 Lightning Bolt\n1 Sol Ring\n2 Llanowar Elves");
  await list.resolve();
  await list.chooseOutput();
  await list.render();

  await expect(run.body).toBeVisible();
  await run.waitForState("finished");
  await expect(run.title).toHaveText("Run · Pasted list");
  await expect(run.summary).toContainText("3 done");
  await expect(run.count).toHaveText("3 / 3");
  await expect(run.rows).toHaveCount(3);
  await expect(run.chip("Done")).toContainText("3");
  await expect(run.chip("Failed")).toContainText("0");

  // Each card is a file named for its printing, beside the run's report
  const files = await scratch.chosenFolder();
  expect(files.filter((f) => f.endsWith(".jpg"))).toEqual(["Lightning Bolt [2X2-117].jpg", "Llanowar Elves [DOM-168].jpg", "Sol Ring [C21-263].jpg"]);
  expect(files.some((f) => /^mimic-run-.*\.json$/.test(f))).toBe(true);
  const head = (await readFile(scratch.folderPath("Sol Ring [C21-263].jpg"))).subarray(0, 3);
  expect(head.equals(JPEG)).toBe(true);
});

test("the run console filters its rows and shows a card's image", async ({ list, run }) => {
  await list.paste("1 Lightning Bolt\n1 Sol Ring");
  await list.resolve();
  await list.chooseOutput();
  await list.render();
  await run.waitForState("finished");

  await run.filterBox.fill("sol");
  await expect(run.row("Sol Ring")).toBeVisible();
  await expect(run.row("Lightning Bolt")).toBeHidden();

  await run.filterBox.fill("");
  await run.row("Lightning Bolt").click();
  const image = run.page.locator("#run-image");
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate((img) => img.complete && img.naturalWidth > 0)).toBe(true);
});

test("opening the run's folder asks the file manager for it", async ({ list, run, scratch }) => {
  await list.paste("1 Sol Ring");
  await list.resolve();
  await list.chooseOutput();
  await list.render();
  await run.waitForState("finished");

  await run.openFolder.click();
  await expect.poll(async () => (await scratch.events()).filter(([kind]) => kind === "path").map(([, detail]) => detail)).toContain(scratch.folderPath());
});

test("a run that is stopped skips the cards it had not reached", async ({ list, run, fake, scratch }) => {
  // Each card's art takes three seconds to arrive, which leaves time to stop
  await fake.delay("cards.scryfall.io", 3000);
  await list.paste("1 Lightning Bolt\n1 Sol Ring\n1 Llanowar Elves\n1 Delver of Secrets");
  await list.resolve();
  await list.chooseOutput();
  await list.render();

  await run.waitForState("running");
  await expect(run.stopButton).toBeVisible();
  await run.stopButton.click();
  await run.waitForState("stopped");
  await expect(run.stopButton).toBeHidden();
  expect((await scratch.chosenFolder()).filter((f) => f.endsWith(".jpg")).length).toBeLessThan(3);
});

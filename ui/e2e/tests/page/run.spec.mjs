import { test, expect } from "../../bridge-fixtures.mjs";
import { RunPage } from "../../pages/RunPage.mjs";

const view = (cards, extra = {}) => ({
  id: "run-1",
  label: "Pasted list",
  outDir: "/tmp/out",
  template: "normal",
  dpi: 274,
  format: "jpeg",
  concurrency: 4,
  started: "2026-10-08T12:00:00Z",
  finished: "2026-10-08T12:00:05Z",
  cards,
  ...extra,
});
const cardOf = (index, name, status, extra) => ({ index, name, status, ...extra });

async function showRun(page) {
  await page.locator('button[data-mode="run"]').click();
  return new RunPage(page);
}

test("with no run yet the console offers the list", async ({ bridge, page }) => {
  bridge.reply("Run.Latest", null);
  await bridge.boot();
  const run = await showRun(page);
  await expect(run.empty).toBeVisible();
  await expect(run.body).toBeHidden();
  await run.empty.getByRole("button", { name: "Go to List" }).click();
  await expect(page.locator("#list-input")).toBeVisible();
});

test("a run with failures says which failed and why, and retries only those", async ({ bridge, page }) => {
  bridge.reply("Run.Latest", view([
    cardOf(0, "Sol Ring", "done", { file: "Sol Ring.jpg", ms: 40 }),
    cardOf(1, "Lightning Bolt", "failed", { stage: "art", error: "fetching art: 503" }),
    cardOf(2, "Llanowar Elves", "done", { file: "Llanowar Elves.jpg", ms: 41 }),
  ]));
  await bridge.boot();
  const run = await showRun(page);

  await run.waitForState("finished with failures");
  await expect(page.locator("#run-failed")).toHaveText("1 failed");
  await expect(run.chip("Done")).toContainText("2");
  await expect(run.chip("Failed")).toContainText("1");
  await expect(run.retryButton).toBeVisible();
  await expect(page.locator("#run-retry-label")).toHaveText("Retry 1 failed");

  await run.row("Lightning Bolt").click();
  await expect(page.getByText("fetching art: 503")).toBeVisible();

  bridge.reply("Run.Retry", view([cardOf(0, "Lightning Bolt", "queued")], { id: "run-2", finished: undefined }));
  await run.retryButton.click();
  await expect.poll(() => bridge.calledWith("Run.Retry")).toEqual([["run-1"]]);
});

test("a run that was stopped shows the cards it skipped", async ({ bridge, page }) => {
  bridge.reply("Run.Latest", view([
    cardOf(0, "Sol Ring", "done", { file: "Sol Ring.jpg", ms: 40 }),
    cardOf(1, "Lightning Bolt", "skipped"),
    cardOf(2, "Llanowar Elves", "skipped"),
  ], { stopped: true }));
  await bridge.boot();
  const run = await showRun(page);
  await run.waitForState("stopped");
  await expect(run.retryButton).toBeHidden();
  await expect(run.row("Lightning Bolt")).toBeVisible();
  await expect(run.summary).toContainText("2 skipped");
});

test("a run that cannot start is reported and the list stays", async ({ bridge, page }) => {
  await bridge.boot();
  await page.locator('button[data-mode="list"]').click();
  bridge.reply("List.Resolve", { jobId: "r1", format: "names", rows: [{ line: 1, text: "Sol Ring", qty: 1, name: "Sol Ring" }] });
  await page.locator("#list-input").fill("Sol Ring");
  await page.locator("#list-resolve").click();
  await expect.poll(() => bridge.calledWith("List.Resolve").length).toBe(1);
  await bridge.job("r1", [{ seq: 1, frac: 1, row: { index: 0, status: "matched", card: { Name: "Sol Ring", SetCode: "c21", CollectorNumber: "263", shapes: [{ role: "single", kind: "standard" }] } } }, { seq: 2, frac: 1, done: true }]);

  bridge.reply("System.PickFolder", "/tmp/out");
  await page.locator("#list-output").click();
  bridge.fail("Run.Start", "the folder cannot be written to");
  await page.locator("#list-render").click();
  await expect(page.locator(".toast.err")).toContainText("the folder cannot be written to");
  await expect(page.locator("#list-review-body")).toBeVisible();
});

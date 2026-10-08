import { test, expect } from "../../bridge-fixtures.mjs";
import { ListPage } from "../../pages/ListPage.mjs";
import { card, done, matched } from "../../bridge/cards.mjs";

const row = (line, text, extra) => ({ line, text, qty: 1, name: text, ...extra });

// A list pasted and resolved, with the rows and events the test gives
async function resolveWith(bridge, page, rows, events, jobId = "r1") {
  await bridge.boot();
  const list = new ListPage(page);
  await list.show();
  bridge.reply("List.Resolve", { jobId, format: "names", rows });
  await list.paste(rows.map((r) => r.text).join("\n"));
  await page.locator("#list-resolve").click();
  await expect.poll(() => bridge.calledWith("List.Resolve").length).toBeGreaterThan(0);
  await bridge.job(jobId, events);
  return list;
}

test("an ambiguous row offers its candidates and the one chosen is kept", async ({ bridge, page }) => {
  const bolt = card("Lightning Bolt", { SetCode: "2x2", CollectorNumber: "117" });
  const other = card("Lightning Bolt", { SetCode: "m11", CollectorNumber: "149" });
  const list = await resolveWith(bridge, page, [row(1, "Lightning Bolt")], [
    { seq: 1, frac: 1, row: { index: 0, status: "ambiguous", candidates: [bolt, other], note: "Two printings fit" } },
    done(2),
  ]);

  await expect(list.row("Lightning Bolt")).toContainText("ambiguous");
  await expect(list.chip("Needs attention")).toHaveAttribute("aria-pressed", "true");
  const choices = page.getByRole("radiogroup", { name: "Choices for Lightning Bolt" });
  await expect(choices.getByRole("radio")).toHaveCount(2);

  await choices.getByRole("radio", { name: /M11/ }).click();
  await expect(list.row("Lightning Bolt")).toContainText("matched");
  await expect(list.row("Lightning Bolt")).toContainText("M11 149");
  await expect(list.renderLabel).toHaveText("Render 1 card");
});

test("a lookup that failed can be tried again on its own", async ({ bridge, page }) => {
  const list = await resolveWith(bridge, page, [row(1, "Sol Ring")], [
    { seq: 1, frac: 1, row: { index: 0, status: "error", note: "the lookup timed out" } },
    done(2),
  ]);
  await expect(list.row("Sol Ring")).toContainText("lookup failed");
  await expect(page.getByText("the lookup timed out")).toBeVisible();

  bridge.reply("List.Resolve", { jobId: "r2", format: "names", rows: [row(1, "Sol Ring")] });
  await page.getByRole("button", { name: "Try again" }).click();
  await expect.poll(() => bridge.calledWith("List.Resolve").length).toBe(2);
  await bridge.job("r2", [{ seq: 1, frac: 1, row: matched(0, "Sol Ring") }, done(2)]);
  await expect(list.row("Sol Ring")).toContainText("matched");
});

test("a lookup that stops partway says so and keeps the rows it reached", async ({ bridge, page }) => {
  const list = await resolveWith(bridge, page, [row(1, "Sol Ring"), row(2, "Lightning Bolt")], [
    { seq: 1, frac: 0.5, row: matched(0, "Sol Ring") },
    done(2, { error: "scryfall answered 503" }),
  ]);
  await expect(page.locator(".toast.err")).toContainText("Resolving stopped: scryfall answered 503");
  await expect(list.row("Sol Ring")).toContainText("matched");
  await expect(list.row("Lightning Bolt")).toContainText("resolving");
});

test("a list that cannot be started is reported", async ({ bridge, page }) => {
  await bridge.boot();
  const list = new ListPage(page);
  await list.show();
  bridge.fail("List.Resolve", "the list is too long");
  await list.paste("1 Sol Ring");
  await page.locator("#list-resolve").click();
  await expect(page.locator(".toast.err")).toContainText("the list is too long");
  await expect(page.locator("#list-empty")).toBeVisible();
});

test("resolving an empty box asks for nothing", async ({ bridge, page }) => {
  await bridge.boot();
  const list = new ListPage(page);
  await list.show();
  await expect(page.locator("#list-empty")).toBeVisible();
  await page.locator("#list-resolve").click();
  await expect(list.input).toBeFocused();
  expect(bridge.calledWith("List.Resolve")).toHaveLength(0);
});

test("a list file that cannot be read is reported and leaves the box alone", async ({ bridge, page }) => {
  await bridge.boot();
  const list = new ListPage(page);
  await list.show();
  await list.paste("1 Sol Ring");
  bridge.reply("System.PickFile", "/tmp/deck.txt");
  bridge.fail("List.ReadFile", "reading the file: permission denied");
  await page.locator("#list-open").click();
  await expect(page.locator(".toast.err")).toContainText("permission denied");
  await expect(list.input).toHaveValue("1 Sol Ring");
});

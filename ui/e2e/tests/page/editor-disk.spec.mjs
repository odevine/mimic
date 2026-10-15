import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The editor with a template open and drawn, and a fingerprint of its files that a test changes
// to stand for another program writing them
async function ready(bridge, page) {
  bridge.stamp = "stamp-1";
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", () => draftView(mine, { stamp: bridge.stamp }));
  bridge.reply("Studio.Stamp", () => bridge.stamp);
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  return editor;
}

const regainFocus = (page) => page.evaluate(() => window.dispatchEvent(new Event("focus")));
const opens = (bridge) => bridge.calledWith("Studio.Open").length;

test("a window that comes back to files that are as they were leaves the template alone", async ({ bridge, page }) => {
  await ready(bridge, page);
  const before = opens(bridge);
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(1);
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(2);
  expect(opens(bridge)).toBe(before);
  expect(bridge.calledWith("Studio.Render")).toHaveLength(1);
});

test("files changed outside the app are read again when the window comes back", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  const before = opens(bridge);
  bridge.stamp = "stamp-2";
  await regainFocus(page);
  await expect.poll(() => opens(bridge)).toBe(before + 1);
  await expect(page.locator("#toasts .toast").last()).toHaveText("This template was changed outside the app, so it was read again");
  // The card is drawn again from the files
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await bridge.job("job-2", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  // What it was read at is now the stamp, so the same files do not read again
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(2);
  expect(opens(bridge)).toBe(before + 1);
});

test("text in the JSON editor that was not applied is asked about before the files are read again", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.jsonButton.click();
  await editor.jsonText.fill('{"half": ');
  const before = opens(bridge);

  bridge.reply("System.Confirm", false);
  bridge.stamp = "stamp-2";
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("System.Confirm").length).toBe(1);
  const [ask] = bridge.calledWith("System.Confirm")[0];
  expect(ask).toMatchObject({ title: "Read the files again?", accept: "Read again" });
  // A no keeps the text and the template as they are
  expect(opens(bridge)).toBe(before);
  await expect(editor.jsonText).toHaveValue('{"half": ');
  // It is asked once for a change, and not again for the same files
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(2);
  expect(bridge.calledWith("System.Confirm")).toHaveLength(1);

  // Another change, and a yes reads them and drops the text
  bridge.reply("System.Confirm", true);
  bridge.stamp = "stamp-3";
  await regainFocus(page);
  await expect.poll(() => opens(bridge)).toBe(before + 1);
  await expect(editor.jsonText).not.toHaveValue('{"half": ');
});

test("the app's own edits are not taken for a change made outside it", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  bridge.reply("Studio.Apply", () => {
    bridge.stamp = "stamp-own";
    return { rev: 1, inverse: [{ op: "move", layout: "normal", kind: "text", key: "title", dx: -10, dy: 0 }], valid: true, diagnostics: [], stamp: "stamp-own" };
  });
  const before = opens(bridge);
  await editor.layers.first().click();
  await page.keyboard.press("ArrowRight");
  await expect.poll(() => bridge.calledWith("Studio.Apply").length).toBe(1);
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(1);
  expect(opens(bridge)).toBe(before);
  expect(bridge.calledWith("System.Confirm")).toHaveLength(0);
});

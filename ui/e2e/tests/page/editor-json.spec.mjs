import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The editor with a template open and its card drawn, and Apply doing what the app
// does to the files, for a whole-file write and for a lever
async function ready(bridge, page) {
  bridge.stamp = "stamp-1";
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", () => draftView(mine, { stamp: bridge.stamp }));
  bridge.reply("Studio.Stamp", () => bridge.stamp);
  bridge.reply("Studio.Apply", (req) => {
    const op = req.ops[0];
    if (op.op !== "replace") {
      bridge.source.layout.textBoxes[op.key].text.align = op.value;
      return { rev: 1, inverse: [{ ...op, value: "left" }], valid: true, diagnostics: [] };
    }
    const was = bridge.source.files().files.find((f) => f.name === op.file).content;
    bridge.source.extra[op.file] = op.content;
    return { rev: 1, inverse: [{ op: "replace", file: op.file, content: was }], valid: true, diagnostics: [] };
  });
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  return editor;
}

const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);
const fileText = (bridge, name) => bridge.source.files().files.find((f) => f.name === name).content;
const regainFocus = (page) => page.evaluate(() => window.dispatchEvent(new Event("focus")));
const opens = (bridge) => bridge.calledWith("Studio.Open").length;

test("the JSON editor shows each file, saves checked text, follows undo and a lever, and keeps typed text", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(editor.jsonPane).toBeHidden();
  await editor.jsonButton.click();
  await expect(editor.jsonTabs).toHaveText(["template.json", "normal"]);
  await expect(editor.jsonText).toHaveValue(fileText(bridge, "template.json"));
  await editor.jsonTabs.nth(1).click();
  const before = fileText(bridge, "layouts/normal.json");
  await expect(editor.jsonText).toHaveValue(before);

  const text = before.replace('"fontSize": 60', '"fontSize": 66');
  await editor.jsonText.fill(text);
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(bridge.calledWith("Studio.CheckFile").at(-1)[0]).toEqual({ id: "mine", file: "layouts/normal.json", content: text });
  expect(applied(bridge)[0]).toEqual({ id: "mine", ops: [{ op: "replace", file: "layouts/normal.json", content: text }] });
  await expect(editor.jsonStatus).toHaveText("Saved");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);

  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toEqual({ op: "replace", file: "layouts/normal.json", content: before });
  await expect(editor.jsonText).toHaveValue(before);

  // A lever's change shows in the open file, unless text typed there is not saved yet
  await editor.row("title").click();
  await editor.lever("lever-align").selectOption("right");
  await expect(editor.jsonText).toHaveValue(/"align": "right"/);
  bridge.reply("Studio.CheckFile", () => ({ writable: false, problems: [{ severity: "error", message: "not yet" }] }));
  await editor.jsonText.fill("{ my own text");
  await editor.lever("lever-align").selectOption("center");
  await expect.poll(() => applied(bridge).length).toBe(4);
  await expect(editor.jsonText).toHaveValue("{ my own text");

  await page.locator("#ed-json-close").click();
  await expect(editor.jsonPane).toBeHidden();
});

test("JSON with a mistake or that would not load is not saved, and a write that breaks the template stops the picture", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  bridge.reply("Studio.CheckFile", () => ({ writable: false, problems: [{ severity: "error", line: 3, column: 5, message: "invalid character '}' looking for beginning of object key string" }] }));
  await editor.jsonButton.click();
  await editor.jsonText.fill('{\n  "a": 1,\n  }\n');
  await expect(editor.jsonProblems).toHaveText(["Line 3, column 5: invalid character '}' looking for beginning of object key string"]);
  await expect(editor.jsonStatus).toHaveText("Not saved, fix the problems first");
  // Pressing the problem puts the caret where it is
  await editor.jsonProblems.first().click();
  await expect(editor.jsonText).toBeFocused();
  expect(await editor.jsonText.evaluate((ta) => ta.selectionStart)).toBe('{\n  "a": 1,\n'.length + 4);

  bridge.reply("Studio.CheckFile", () => ({
    writable: false,
    problems: [
      { severity: "error", path: "layouts.normal.textBoxes.title.text.align", message: "\"diagonal\" is not left, center or right" },
      { severity: "warning", path: "template.json", message: "the template has no description" },
    ],
  }));
  await editor.jsonTabs.nth(1).click();
  await editor.jsonText.fill(fileText(bridge, "layouts/normal.json").replace('"left"', '"diagonal"'));
  await expect(editor.jsonProblems).toHaveText(["layouts.normal.textBoxes.title.text.align: \"diagonal\" is not left, center or right", "template.json: the template has no description"]);
  await page.waitForTimeout(300);
  expect(applied(bridge)).toEqual([]);

  bridge.reply("Studio.CheckFile", () => ({ writable: true, problems: [] }));
  bridge.reply("Studio.Apply", (req) => ({
    rev: 1,
    inverse: [{ op: "replace", file: req.ops[0].file, content: fileText(bridge, req.ops[0].file) }],
    valid: false,
    diagnostics: [{ severity: "error", path: "layouts.normal.serves", message: "serves no shape" }],
  }));
  await editor.jsonText.fill("{}");
  await expect.poll(() => applied(bridge).length).toBe(1);
  await expect(editor.problems).toHaveText(["layouts.normal.serves: serves no shape"]);
  await expect(page.locator("#ed-activity-title")).toHaveText("This template does not load");
  await expect(editor.preview).toBeHidden();
});

test("files changed outside the app are read again when the window comes back, and the app's own edits are not", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  const before = opens(bridge);
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(1);
  expect(opens(bridge)).toBe(before);

  // An edit of its own gives the new stamp with it
  bridge.reply("Studio.Apply", () => {
    bridge.stamp = "stamp-own";
    return { rev: 1, inverse: [{ op: "move", layout: "normal", kind: "text", key: "title", dx: -10, dy: 0 }], valid: true, diagnostics: [], stamp: "stamp-own" };
  });
  await editor.layers.first().click();
  await page.keyboard.press("ArrowRight");
  await expect.poll(() => applied(bridge).length).toBe(1);
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(2);
  expect(opens(bridge)).toBe(before);

  bridge.stamp = "stamp-2";
  await regainFocus(page);
  await expect.poll(() => opens(bridge)).toBe(before + 1);
  await expect(page.locator("#toasts .toast").last()).toHaveText("This template was changed outside the app, so it was read again");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBeGreaterThan(1);
  // What it was read at is now the stamp, so the same files do not read again
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(4);
  expect(opens(bridge)).toBe(before + 1);
});

test("text in the JSON editor that was not applied is asked about once before the files are read again", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  bridge.reply("Studio.CheckFile", () => ({ writable: false, problems: [{ severity: "error", message: "not yet" }] }));
  await editor.jsonButton.click();
  await editor.jsonText.fill('{"half": ');
  const before = opens(bridge);

  bridge.reply("System.Confirm", false);
  bridge.stamp = "stamp-2";
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("System.Confirm").length).toBe(1);
  expect(bridge.calledWith("System.Confirm")[0][0]).toMatchObject({ title: "Read the files again?", accept: "Read again" });
  expect(opens(bridge)).toBe(before);
  await expect(editor.jsonText).toHaveValue('{"half": ');
  await regainFocus(page);
  await expect.poll(() => bridge.calledWith("Studio.Stamp").length).toBe(2);
  expect(bridge.calledWith("System.Confirm")).toHaveLength(1);

  bridge.reply("System.Confirm", true);
  bridge.stamp = "stamp-3";
  await regainFocus(page);
  await expect.poll(() => opens(bridge)).toBe(before + 1);
  await expect(editor.jsonText).not.toHaveValue('{"half": ');
});

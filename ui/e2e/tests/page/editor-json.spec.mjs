import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The editor with a template open and its card drawn, and Apply doing what the app
// does to the files for a whole-file write
async function ready(bridge, page) {
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine));
  bridge.reply("Studio.Apply", (req) => {
    const op = req.ops[0];
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
const checked = (bridge) => bridge.calledWith("Studio.CheckFile").map(([req]) => req);
const fileText = (bridge, name) => bridge.source.files().files.find((f) => f.name === name).content;

test("the JSON editor is closed until it is asked for, then lists the files", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(editor.jsonPane).toBeHidden();
  await expect(editor.jsonButton).toHaveAttribute("aria-expanded", "false");
  await editor.jsonButton.click();
  await expect(editor.jsonPane).toBeVisible();
  await expect(editor.jsonButton).toHaveAttribute("aria-expanded", "true");
  await expect(editor.jsonTabs).toHaveText(["template.json", "normal"]);
  await expect(editor.jsonTabs.first()).toHaveAttribute("aria-selected", "true");
  await expect(editor.jsonText).toHaveValue(fileText(bridge, "template.json"));
  // One number in the margin for each line
  const lines = fileText(bridge, "template.json").split("\n").length;
  await expect(page.locator("#ed-json-gutter")).toHaveText(Array.from({ length: lines }, (_, i) => i + 1).join("\n"));

  await editor.jsonTabs.nth(1).click();
  await expect(editor.jsonText).toHaveValue(fileText(bridge, "layouts/normal.json"));
  await expect(editor.jsonTabs.nth(1)).toHaveAttribute("aria-selected", "true");

  await page.locator("#ed-json-close").click();
  await expect(editor.jsonPane).toBeHidden();
  await expect(editor.jsonButton).toHaveAttribute("aria-expanded", "false");
});

test("text typed in is checked, and saved when the app says it can be", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.jsonButton.click();
  await editor.jsonTabs.nth(1).click();
  const text = fileText(bridge, "layouts/normal.json").replace('"fontSize": 60', '"fontSize": 66');
  await editor.jsonText.fill(text);

  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(checked(bridge).at(-1)).toEqual({ id: "mine", file: "layouts/normal.json", content: text });
  expect(applied(bridge)[0]).toEqual({ id: "mine", ops: [{ op: "replace", file: "layouts/normal.json", content: text }] });
  await expect(editor.jsonStatus).toHaveText("Saved");
  await expect(editor.undoButton).toBeEnabled();
  // The card is drawn again with it
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
});

test("a mistake in the JSON is shown with its line and where to go, and nothing is saved", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  bridge.reply("Studio.CheckFile", () => ({ writable: false, problems: [{ severity: "error", line: 3, column: 5, message: "invalid character '}' looking for beginning of object key string" }] }));
  await editor.jsonButton.click();
  await editor.jsonText.fill('{\n  "a": 1,\n  }\n');
  await expect(editor.jsonProblems).toHaveText(["Line 3, column 5: invalid character '}' looking for beginning of object key string"]);
  await expect(editor.jsonStatus).toHaveText("Not saved, fix the problems first");
  await page.waitForTimeout(300);
  expect(applied(bridge)).toEqual([]);

  // Pressing the problem puts the caret where it is
  await editor.jsonProblems.first().click();
  await expect(editor.jsonText).toBeFocused();
  const at = await editor.jsonText.evaluate((ta) => ta.selectionStart);
  expect(at).toBe('{\n  "a": 1,\n'.length + 4);
});

test("text that is JSON but would not load is not saved, and the problems say where", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  bridge.reply("Studio.CheckFile", () => ({
    writable: false,
    problems: [
      { severity: "error", path: "layouts.normal.textBoxes.title.text.align", message: "\"diagonal\" is not left, center or right" },
      { severity: "warning", path: "template.json", message: "the template has no description" },
    ],
  }));
  await editor.jsonButton.click();
  await editor.jsonTabs.nth(1).click();
  await editor.jsonText.fill(fileText(bridge, "layouts/normal.json").replace('"left"', '"diagonal"'));
  await expect(editor.jsonProblems).toHaveText([
    "layouts.normal.textBoxes.title.text.align: \"diagonal\" is not left, center or right",
    "template.json: the template has no description",
  ]);
  await expect(editor.jsonProblems.first()).toHaveClass(/error/);
  await expect(editor.jsonProblems.nth(1)).toHaveClass(/warning/);
  await page.waitForTimeout(300);
  expect(applied(bridge)).toEqual([]);
});

test("a change made with a lever shows in the open file, and typed text is not written over", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  bridge.reply("Studio.Apply", (req) => {
    const op = req.ops[0];
    bridge.source.layout.textBoxes[op.key].text.align = op.value;
    return { rev: 1, inverse: [{ ...op, value: "left" }], valid: true, diagnostics: [] };
  });
  await editor.jsonButton.click();
  await editor.jsonTabs.nth(1).click();
  await editor.layers.filter({ hasText: "title" }).click();
  await editor.lever("lever-align").selectOption("right");
  await expect(editor.jsonText).toHaveValue(/"align": "right"/);

  // With text typed that is not saved, a change elsewhere leaves it as it is
  bridge.reply("Studio.CheckFile", () => ({ writable: false, problems: [{ severity: "error", message: "not yet" }] }));
  await editor.jsonText.fill("{ my own text");
  await editor.lever("lever-align").selectOption("center");
  await expect.poll(() => applied(bridge).length).toBe(2);
  await expect(editor.jsonText).toHaveValue("{ my own text");
});

test("a write that leaves the template unable to load shows its problems and stops the picture", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  bridge.reply("Studio.Apply", (req) => ({
    rev: 1,
    inverse: [{ op: "replace", file: req.ops[0].file, content: fileText(bridge, req.ops[0].file) }],
    valid: false,
    diagnostics: [{ severity: "error", path: "layouts.normal.serves", message: "serves no shape" }],
  }));
  await editor.jsonButton.click();
  await editor.jsonTabs.nth(1).click();
  await editor.jsonText.fill("{}");
  await expect.poll(() => applied(bridge).length).toBe(1);
  await expect(editor.problems).toHaveText(["layouts.normal.serves: serves no shape"]);
  await expect(page.locator("#ed-activity-title")).toHaveText("This template does not load");
  await expect(editor.activity).toHaveAttribute("data-state", "error");
  await expect(editor.preview).toBeHidden();
});

test("undo takes a whole-file edit back and the open text follows", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.jsonButton.click();
  await editor.jsonTabs.nth(1).click();
  const before = fileText(bridge, "layouts/normal.json");
  await editor.jsonText.fill(before.replace('"fontSize": 60', '"fontSize": 61'));
  await expect.poll(() => applied(bridge).length).toBe(1);
  await expect(editor.jsonStatus).toHaveText("Saved");

  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toEqual({ op: "replace", file: "layouts/normal.json", content: before });
  await expect(editor.jsonText).toHaveValue(before);
});

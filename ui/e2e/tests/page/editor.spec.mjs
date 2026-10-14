import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, progress, draftSummary, draftView, card } from "../../bridge/cards.mjs";

const newer = draftSummary("newer", "Newer frame", { updated: "2026-10-12T09:00:00Z" });
const older = draftSummary("older", "Older frame", { updated: "2026-10-01T09:00:00Z" });

// The editor with two templates, which opens the first of them
async function withDrafts(bridge, page, { views = {} } = {}) {
  bridge.reply("Studio.List", [newer, older]);
  bridge.reply("Studio.Open", (id) => views[id] || draftView(id === "newer" ? newer : older));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  return editor;
}

// The first draft's render is the first job, and the page numbers them from one
const finishJob = (bridge, id, extra) => bridge.job(id, [progress(1, "Compositing frame", 0.5), done(2, extra)]);

test("the editor opens the newest template and draws a test card with it", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect(editor.workspace).toBeVisible();
  await expect(editor.picker.locator("option")).toHaveText(["Newer frame", "Older frame"]);
  await expect.poll(() => bridge.calledWith("Studio.Open")).toEqual([["newer"]]);

  // With nothing remembered, the first preset is the card
  await expect(editor.cardLine).toHaveText("Dawnwatch Sentinel · TST");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  const [request] = bridge.calledWith("Studio.Render")[0];
  expect(request.id).toBe("newer");
  expect(request.card.Name).toBe("Dawnwatch Sentinel");
  expect(request.face).toBe(0);

  await expect(editor.activity).toHaveAttribute("data-state", "working");
  await finishJob(bridge, "job-1");
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Dawnwatch Sentinel");
  await expect(editor.preview).toHaveAttribute("src", /\/img\/render\/job-1/);
  await expect(editor.preview).toBeVisible();
});

test("a template that was tested with a card opens with it", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page, { views: { newer: draftView(newer, { meta: { testCard: card("Lightning Bolt", { SetCode: "2x2" }), face: 0 } }) } });
  await expect(editor.cardLine).toHaveText("Lightning Bolt · 2X2");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  expect(bridge.calledWith("Studio.Render")[0][0].card.Name).toBe("Lightning Bolt");
});

test("a render that fails says why and leaves no image", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await finishJob(bridge, "job-1", { error: "rendering \"Dawnwatch Sentinel\": a layer image is missing" });

  await expect(editor.activity).toHaveAttribute("data-state", "error");
  await expect(page.locator("#ed-activity-title")).toHaveText("Could not draw the card");
  await expect(page.locator("#ed-activity-step")).toContainText("a layer image is missing");
  await expect(editor.preview).toBeHidden();
});

test("a render the app refuses to start says why", async ({ bridge, page }) => {
  bridge.fail("Studio.Render", "This template does not render transform front faces");
  const editor = await withDrafts(bridge, page);
  await expect(editor.activity).toHaveAttribute("data-state", "error");
  await expect(page.locator("#ed-activity-step")).toHaveText("This template does not render transform front faces");
  await expect(editor.preview).toBeHidden();
});

test("choosing a preset remembers it and draws it, and a newer choice replaces an older one", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await finishJob(bridge, "job-1");
  await expect(editor.activity).toHaveAttribute("data-state", "done");

  await editor.preset("Land").click();
  await expect(editor.cardLine).toHaveText("Mistral Shore · TST");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  // Choosing again before the first is drawn abandons it
  await editor.preset("Instant").click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(3);
  await expect.poll(() => bridge.calledWith("Render.Cancel")).toContainEqual(["job-2"]);

  const remembered = bridge.calledWith("Studio.SetTestCard");
  expect(remembered.map(([req]) => [req.id, req.card.Name, req.face])).toEqual([
    ["newer", "Mistral Shore", 0],
    ["newer", "Stormbound Reply", 0],
  ]);

  // Only the newest job's image is shown
  await finishJob(bridge, "job-2");
  await expect(editor.activity).not.toHaveAttribute("data-state", "done");
  await finishJob(bridge, "job-3");
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.preview).toHaveAttribute("src", /\/img\/render\/job-3/);
});

test("a template that does not load lists its problems and draws nothing", async ({ bridge, page }) => {
  const broken = draftView(newer, {
    valid: false,
    layouts: [],
    diagnostics: [
      { severity: "error", path: "layouts/normal.json", message: "unexpected end of JSON input" },
      { severity: "warning", path: "template.json", message: "the template has no description" },
    ],
  });
  const editor = await withDrafts(bridge, page, { views: { newer: broken } });

  await expect(editor.problems).toHaveText(["layouts/normal.json: unexpected end of JSON input", "template.json: the template has no description"]);
  await expect(editor.problems.first()).toHaveClass(/error/);
  await expect(editor.problems.nth(1)).toHaveClass(/warning/);
  await expect(page.locator("#ed-problems-count")).toHaveText("2");
  await expect(editor.activity).toHaveAttribute("data-state", "error");
  await expect(page.locator("#ed-activity-title")).toHaveText("This template does not load");
  await expect(editor.preview).toBeHidden();
  expect(bridge.calledWith("Studio.Render")).toHaveLength(0);
});

test("a template that opens slowly is dropped when another was picked since", async ({ bridge, page }) => {
  let release;
  const gate = new Promise((resolve) => (release = resolve));
  const editor = await withDrafts(bridge, page);
  bridge.reply("Studio.Open", async (id) => {
    if (id === "older") await gate;
    return draftView(id === "newer" ? newer : older);
  });

  await editor.picker.selectOption("older");
  await expect.poll(() => bridge.calledWith("Studio.Open").map(([id]) => id)).toEqual(["newer", "older"]);
  await editor.picker.selectOption("newer");
  await expect(editor.aboutName).toHaveText("Newer frame");
  release();
  await page.waitForTimeout(100);
  await expect(editor.aboutName).toHaveText("Newer frame");
  await expect(editor.picker).toHaveValue("newer");
});

test("with no templates the editor offers a new one, which opens as soon as it is made", async ({ bridge, page }) => {
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect(editor.empty).toBeVisible();

  const made = draftSummary("my-frame", "My frame");
  bridge.reply("Studio.Create", made);
  bridge.reply("Studio.List", [made]);
  bridge.reply("Studio.Open", draftView(made));
  await editor.openNewDialog();
  await editor.dialogName.fill("  My frame ");
  await editor.dialogDpi.fill("300");
  await expect(page.locator("#draft-dpi-size")).toHaveText("A canvas of 816 × 1110 pixels");
  await editor.dialogConfirm.click();

  await expect(editor.dialog).toBeHidden();
  expect(bridge.calledWith("Studio.Create")).toEqual([[{ name: "My frame", dpi: 300 }]]);
  await expect(editor.aboutName).toHaveText("My frame");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
});

test("a template the app will not make keeps the dialog open with the reason", async ({ bridge, page }) => {
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  bridge.fail("Studio.Create", "A name is at most 80 characters");
  await editor.openNewDialog();
  await editor.dialogName.fill("A fine name");
  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toHaveText("A name is at most 80 characters");
  await expect(editor.dialog).toBeVisible();
  await expect(editor.dialogConfirm).toBeEnabled();
});

test("the application menu's Editor choice opens the editor", async ({ bridge, page }) => {
  await bridge.boot();
  await bridge.emit("menu", "mode:editor");
  await expect(page.locator("#mode-editor")).toBeVisible();
  await expect(page.locator('button[data-mode="editor"]')).toHaveAttribute("aria-current", "page");
});

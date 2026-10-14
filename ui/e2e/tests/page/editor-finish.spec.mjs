import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, presetCards } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The editor on one template with its card drawn
async function ready(bridge, page, view = {}) {
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine, view));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  return editor;
}

const release = (extra = {}) => ({
  templateId: "user.mine",
  name: "Mine",
  version: "0.1.0",
  author: "",
  license: "CC-BY-4.0",
  credits: [{ name: "Jane Doe", note: "frame art", url: "https://example.com/jane" }],
  filename: "user.mine-0.1.0.mimic",
  installed: false,
  active: false,
  diagnostics: [],
  ready: true,
  ...extra,
});

test("using the template makes it available and says what it is called", async ({ bridge, page }) => {
  bridge.reply("Studio.Use", { template: "user.mine", version: "draft", replaced: false, active: false });
  const editor = await ready(bridge, page);
  const before = bridge.calledWith("Templates.Active").length;
  await editor.useButton.click();
  await expect(editor.toasts.last()).toHaveText("Mine is ready in Single, List and Run as user.mine. Choose it in Templates to draw with it.");
  expect(bridge.calledWith("Studio.Use")).toEqual([["mine"]]);
  // The top bar is told what is in use now
  await expect.poll(() => bridge.calledWith("Templates.Active").length).toBeGreaterThan(before);
  await expect(editor.useButton).toBeEnabled();
});

test("using it again says it is up to date and that the template in use changed", async ({ bridge, page }) => {
  bridge.reply("Studio.Use", { template: "user.mine", version: "draft", replaced: true, active: true });
  const editor = await ready(bridge, page);
  await editor.useButton.click();
  await expect(editor.toasts.last()).toContainText("Mine is up to date in Single, List and Run as user.mine. It is the template in use");
});

test("a template that cannot be made into a template says why", async ({ bridge, page }) => {
  bridge.fail("Studio.Use", "The template cannot be made into a bundle: layouts/normal.json: the stack names a part that is not there");
  const editor = await ready(bridge, page);
  await editor.useButton.click();
  await expect(page.locator(".toast.err")).toContainText("the stack names a part that is not there");
  await expect(editor.useButton).toBeEnabled();
});

test("a template that does not load can be neither used nor exported", async ({ bridge, page }) => {
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine, { valid: false, layouts: [], diagnostics: [{ severity: "error", path: "template.json", message: "is not valid JSON" }] }));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect(editor.problems).toHaveCount(1);
  await expect(editor.useButton).toBeDisabled();
  await expect(editor.exportButton).toBeDisabled();
});

test("export states the license and credits, then asks where to save and writes the bundle", async ({ bridge, page }) => {
  bridge.reply("Studio.Release", release());
  bridge.reply("System.PickFile", "/Users/o/Desktop/user.mine-0.1.0.mimic");
  bridge.reply("Studio.Export", { path: "/Users/o/Desktop/user.mine-0.1.0.mimic", bytes: 2_400_000 });
  const editor = await ready(bridge, page);
  await editor.exportButton.click();
  await expect(editor.exportDialog).toBeVisible();
  expect(bridge.calledWith("Studio.Release")).toEqual([["mine"]]);
  await expect(editor.exportFacts).toContainText("License");
  await expect(editor.exportFacts).toContainText("CC-BY-4.0");
  await expect(editor.exportFacts).toContainText("user.mine");
  await expect(editor.exportFacts).not.toContainText("Author");
  await expect(editor.exportCredits).toHaveText(["Jane Doe, frame art https://example.com/jane"]);
  await expect(editor.exportNoCredits).toBeHidden();
  // Nothing is written until the save dialog answers
  expect(bridge.calledWith("Studio.Export")).toEqual([]);
  await editor.exportConfirm.click();
  await expect(editor.toasts.last()).toHaveText("Saved user.mine-0.1.0.mimic (2.4 MB)");
  await expect(editor.exportDialog).toBeHidden();
  const [pick] = bridge.calledWith("System.PickFile")[0];
  expect(pick).toMatchObject({ save: true, name: "user.mine-0.1.0.mimic", filters: [{ name: "mimic templates", pattern: "*.mimic" }] });
  expect(bridge.calledWith("Studio.Export")).toEqual([[{ id: "mine", path: "/Users/o/Desktop/user.mine-0.1.0.mimic" }]]);
});

test("cancelling the save dialog leaves the export open and writes nothing", async ({ bridge, page }) => {
  bridge.reply("Studio.Release", release({ credits: [] }));
  bridge.reply("System.PickFile", "");
  const editor = await ready(bridge, page);
  await editor.exportButton.click();
  await expect(editor.exportNoCredits).toBeVisible();
  await expect(editor.exportCredits).toHaveCount(0);
  await editor.exportConfirm.click();
  await expect.poll(() => bridge.calledWith("System.PickFile").length).toBe(1);
  await expect(editor.exportDialog).toBeVisible();
  await expect(editor.exportConfirm).toBeEnabled();
  expect(bridge.calledWith("Studio.Export")).toEqual([]);
});

test("an export that fails says why and stays open", async ({ bridge, page }) => {
  bridge.reply("Studio.Release", release());
  bridge.reply("System.PickFile", "/ro/x.mimic");
  bridge.fail("Studio.Export", "Could not write x.mimic: permission denied");
  const editor = await ready(bridge, page);
  await editor.exportButton.click();
  await editor.exportConfirm.click();
  await expect(editor.exportError).toHaveText("Could not write x.mimic: permission denied");
  await expect(editor.exportDialog).toBeVisible();
  await expect(editor.exportConfirm).toBeEnabled();
});

test("a template that does not build lists what stops it and cannot be saved", async ({ bridge, page }) => {
  bridge.reply("Studio.Release", release({ ready: false, diagnostics: [{ severity: "error", path: "template.json: version", message: '"x" is not a version such as 1.2.3' }, { severity: "warning", message: "the frame image is larger than the canvas" }] }));
  const editor = await ready(bridge, page);
  await editor.exportButton.click();
  await expect(editor.exportProblems).toHaveText(['template.json: version: "x" is not a version such as 1.2.3', "the frame image is larger than the canvas"]);
  await expect(editor.exportConfirm).toBeDisabled();
});

test("show folder asks the app to open the template's folder", async ({ bridge, page }) => {
  bridge.reply("Studio.ShowFolder", null);
  const editor = await ready(bridge, page);
  await editor.folderButton.click();
  await expect.poll(() => bridge.calledWith("Studio.ShowFolder").length).toBe(1);
  expect(bridge.calledWith("Studio.ShowFolder")).toEqual([["mine"]]);
});

test("a card is saved as a test card with the changes made to it, and then chosen", async ({ bridge, page }) => {
  const saved = { id: "user-spy-with-flying", label: "Spy with flying", name: "Mirran Spy", set: "mbs", number: "26", user: true };
  bridge.reply("Studio.SavePreset", saved);
  bridge.reply("Studio.ResolvePreset", (id) => presetCards[id] || presetCards.monocolored);
  const editor = await ready(bridge, page);
  await editor.savePreset.click();
  await expect(editor.dialog).toBeVisible();
  await expect(editor.dialogName).toHaveValue("Mirran Spy");
  await editor.dialogName.fill("Spy with flying");
  await editor.dialogConfirm.click();
  await expect(editor.userPresets).toHaveCount(1);
  await expect(editor.userPresets.locator(".chip").first()).toHaveText("Spy with flying");
  await expect(editor.userPresets.locator(".chip").first()).toHaveAttribute("aria-pressed", "true");
  const [req] = bridge.calledWith("Studio.SavePreset")[0];
  expect(req).toMatchObject({ label: "Spy with flying", face: 0 });
  expect(req.card.Name).toBe("Mirran Spy");
  expect(req.edits.name).toBe("Mirran Spy");
  expect(bridge.calledWith("Studio.ResolvePreset").some(([id]) => id === saved.id)).toBe(true);
});

test("a test card's name is checked before it is sent, and the app's refusal is shown", async ({ bridge, page }) => {
  bridge.fail("Studio.SavePreset", 'A test card is already called "Mine"');
  const editor = await ready(bridge, page);
  await editor.savePreset.click();
  await editor.dialogName.fill("   ");
  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toHaveText("Give the test card a name");
  await editor.dialogName.fill("x".repeat(41));
  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toContainText("at most 40");
  expect(bridge.calledWith("Studio.SavePreset")).toEqual([]);
  await editor.dialogName.fill("Mine");
  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toHaveText('A test card is already called "Mine"');
  await expect(editor.dialog).toBeVisible();
});

test("a saved test card can be removed, and a built-in one has no button for it", async ({ bridge, page }) => {
  const saved = { id: "user-mine", label: "Mine", name: "Mirran Spy", set: "mbs", number: "26", user: true };
  const base = bridge.defaults["Studio.Presets"];
  bridge.reply("Studio.Presets", [...base, saved]);
  bridge.reply("System.Confirm", true);
  bridge.reply("Studio.DeletePreset", null);
  const editor = await ready(bridge, page);
  await expect(editor.userPresets).toHaveCount(1);
  await expect(editor.presets).toHaveCount(base.length + 2);
  await editor.userPresets.getByRole("button", { name: "Remove the test card Mine" }).click();
  await expect(editor.userPresets).toHaveCount(0);
  expect(bridge.calledWith("Studio.DeletePreset")).toEqual([["user-mine"]]);
  await expect(editor.presets).toHaveCount(base.length);
});

test("declining to remove a test card keeps it", async ({ bridge, page }) => {
  const saved = { id: "user-mine", label: "Mine", name: "Mirran Spy", set: "mbs", number: "26", user: true };
  bridge.reply("Studio.Presets", [...bridge.defaults["Studio.Presets"], saved]);
  bridge.reply("System.Confirm", false);
  const editor = await ready(bridge, page);
  await editor.userPresets.getByRole("button", { name: "Remove the test card Mine" }).click();
  await expect.poll(() => bridge.calledWith("System.Confirm").length).toBe(1);
  await expect(editor.userPresets).toHaveCount(1);
  expect(bridge.calledWith("Studio.DeletePreset")).toEqual([]);
});

// The coverage renders are answered one at a time, each as the page asks. The card's own
// render is the test's setup to answer, and an edit's is left, since the strip does not wait
// for it
function coverageRenders(bridge) {
  let issued = 0;
  const strip = [];
  let answered = 0;
  bridge.reply("Studio.Render", (req) => {
    const id = `job-${++issued}`;
    if (req.coverage) strip.push(id);
    return { jobId: id, dpi: req.coverage ? 40 : 150 };
  });
  bridge.reply("Studio.ResolvePreset", (id) => presetCards[id]);
  return async (count, failAt = 0) => {
    for (let i = 1; i <= count; i++) {
      const at = answered++;
      await expect.poll(() => strip.length).toBeGreaterThan(at);
      await bridge.job(strip[at], [i === failAt ? { seq: 1, done: true, error: "no frame for a land with this condition" } : done(1)]);
    }
  };
}

test("the coverage strip draws every test card small and says how many were drawn", async ({ bridge, page }) => {
  const answer = coverageRenders(bridge);
  const editor = await ready(bridge, page);
  await editor.coverage.locator("summary").click();
  await expect(editor.coverageLine).toHaveText("Draws every test card small, to see the frame each one gets");
  await expect(editor.tiles).toHaveCount(6);
  await editor.coverageRun.click();
  await answer(6);
  await expect(editor.coverageLine).toHaveText("6 of 6 drawn");
  await expect(editor.tiles.locator("img")).toHaveCount(6);
  await expect(editor.tiles.first()).toContainText("Monocolored");
  await expect(editor.coverageRun).toHaveText("Draw again");
  const renders = bridge.calledWith("Studio.Render").slice(1).map(([r]) => r);
  expect(renders).toHaveLength(6);
  expect(renders.every((r) => r.coverage === true && r.id === "mine" && !r.edits)).toBe(true);
  expect(renders.map((r) => r.card.Name)).toEqual(["Mirran Spy", "Consuming Aberration", "Eerie Ultimatum", "Norin the Wary", "Sensei's Divining Top", "Flooded Strand"]);
});

test("a card the template cannot draw is named with the reason, and the rest are still drawn", async ({ bridge, page }) => {
  const answer = coverageRenders(bridge);
  const editor = await ready(bridge, page);
  await editor.coverage.locator("summary").click();
  await editor.coverageRun.click();
  await answer(6, 6);
  await expect(editor.coverageLine).toHaveText("5 of 6 drawn. Not drawn: Land");
  const land = editor.tiles.filter({ hasText: "Land" });
  await expect(land).toHaveAttribute("data-state", "failed");
  await expect(land).toContainText("no frame for a land with this condition");
  await expect(land.locator("img")).toHaveCount(0);
});

test("choosing a tile tests the template with that card", async ({ bridge, page }) => {
  const answer = coverageRenders(bridge);
  const editor = await ready(bridge, page);
  await editor.coverage.locator("summary").click();
  await editor.coverageRun.click();
  await answer(6);
  const before = bridge.calledWith("Studio.ResolvePreset").length;
  await editor.tiles.filter({ hasText: "Artifact" }).getByRole("button").click();
  await expect.poll(() => bridge.calledWith("Studio.ResolvePreset").length).toBeGreaterThan(before);
  await expect(editor.cardLine).toContainText("Sensei's Divining Top");
});

test("an edit makes the strip say it is out of date, and drawing again clears that", async ({ bridge, page }) => {
  const answer = coverageRenders(bridge);
  const editor = await ready(bridge, page);
  await editor.coverage.locator("summary").click();
  await editor.coverageRun.click();
  await answer(6);
  await expect(editor.coverageLine).toHaveText("6 of 6 drawn");
  await editor.layers.first().click();
  await page.keyboard.press("ArrowRight");
  await expect(editor.coverageLine).toHaveText("6 of 6 drawn. The template has changed since");
  await expect(editor.tiles.first()).toHaveAttribute("data-stale", "true");
  // Drawing again shows the template as it is now
  await editor.coverageRun.click();
  await answer(6);
  await expect(editor.coverageLine).toHaveText("6 of 6 drawn");
  await expect(editor.tiles.first()).toHaveAttribute("data-stale", "false");
});

test("opening another template clears the strip", async ({ bridge, page }) => {
  const other = draftSummary("other", "Other");
  const answer = coverageRenders(bridge);
  bridge.reply("Studio.List", [mine, other]);
  bridge.reply("Studio.Open", (id) => draftView(id === "other" ? other : mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await editor.coverage.locator("summary").click();
  await editor.coverageRun.click();
  await answer(6);
  await expect(editor.coverageLine).toHaveText("6 of 6 drawn");
  await editor.picker.selectOption("other");
  await expect(editor.coverageLine).toHaveText("Draws every test card small, to see the frame each one gets");
  await expect(editor.tiles.locator("img")).toHaveCount(0);
});

test("deleting a template asks the app what is in use now, since the template it gave goes with it", async ({ bridge, page }) => {
  bridge.reply("System.Confirm", true);
  bridge.reply("Studio.Delete", null);
  const editor = await ready(bridge, page);
  const before = bridge.calledWith("Templates.Active").length;
  bridge.reply("Studio.List", []);
  await page.locator("#ed-delete").click();
  await expect(editor.empty).toBeVisible();
  await expect.poll(() => bridge.calledWith("Templates.Active").length).toBeGreaterThan(before);
});

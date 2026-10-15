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

test("using the template says what it is called, then that it is up to date, or why it cannot be made", async ({ bridge, page }) => {
  bridge.reply("Studio.Use", { template: "user.mine", version: "draft", replaced: false, active: false });
  const editor = await ready(bridge, page);
  const before = bridge.calledWith("Templates.Active").length;
  await editor.useButton.click();
  await expect(editor.toasts.last()).toHaveText("Mine is ready in Single, List and Run as user.mine. Choose it in Templates to draw with it.");
  expect(bridge.calledWith("Studio.Use")).toEqual([["mine"]]);
  // The top bar is told what is in use now
  await expect.poll(() => bridge.calledWith("Templates.Active").length).toBeGreaterThan(before);

  bridge.reply("Studio.Use", { template: "user.mine", version: "draft", replaced: true, active: true });
  await editor.useButton.click();
  await expect(editor.toasts.last()).toContainText("Mine is up to date in Single, List and Run as user.mine. It is the template in use");

  bridge.fail("Studio.Use", "The template cannot be made into a bundle: layouts/normal.json: the stack names a part that is not there");
  await editor.useButton.click();
  await expect(page.locator(".toast.err")).toContainText("the stack names a part that is not there");
  await expect(editor.useButton).toBeEnabled();
});

test("export states the license and credits, writes nothing until a place is chosen, and says why a write fails", async ({ bridge, page }) => {
  bridge.reply("Studio.Release", release());
  const editor = await ready(bridge, page);
  await editor.exportButton.click();
  await expect(editor.exportDialog).toBeVisible();
  expect(bridge.calledWith("Studio.Release")).toEqual([["mine"]]);
  await expect(editor.exportFacts).toContainText("CC-BY-4.0");
  await expect(editor.exportFacts).toContainText("user.mine");
  await expect(editor.exportCredits).toHaveText(["Jane Doe, frame art https://example.com/jane"]);

  bridge.reply("System.PickFile", "");
  await editor.exportConfirm.click();
  await expect.poll(() => bridge.calledWith("System.PickFile").length).toBe(1);
  await expect(editor.exportDialog).toBeVisible();
  expect(bridge.calledWith("Studio.Export")).toEqual([]);

  bridge.reply("System.PickFile", "/ro/x.mimic");
  bridge.fail("Studio.Export", "Could not write x.mimic: permission denied");
  await editor.exportConfirm.click();
  await expect(editor.exportError).toHaveText("Could not write x.mimic: permission denied");
  await expect(editor.exportDialog).toBeVisible();

  bridge.reply("System.PickFile", "/Users/o/Desktop/user.mine-0.1.0.mimic");
  bridge.reply("Studio.Export", { path: "/Users/o/Desktop/user.mine-0.1.0.mimic", bytes: 2_400_000 });
  await editor.exportConfirm.click();
  await expect(editor.toasts.last()).toHaveText("Saved user.mine-0.1.0.mimic (2.4 MB)");
  await expect(editor.exportDialog).toBeHidden();
  expect(bridge.calledWith("System.PickFile").at(-1)[0]).toMatchObject({ save: true, name: "user.mine-0.1.0.mimic", filters: [{ name: "mimic templates", pattern: "*.mimic" }] });
  expect(bridge.calledWith("Studio.Export").at(-1)).toEqual([{ id: "mine", path: "/Users/o/Desktop/user.mine-0.1.0.mimic" }]);
});

test("a test card is saved with its changes after its name is checked, and removed only on a yes", async ({ bridge, page }) => {
  const saved = { id: "user-spy-with-flying", label: "Spy with flying", name: "Mirran Spy", set: "mbs", number: "26", user: true };
  bridge.reply("Studio.ResolvePreset", (id) => presetCards[id] || presetCards.monocolored);
  bridge.fail("Studio.SavePreset", 'A test card is already called "Mine"');
  const editor = await ready(bridge, page);
  await editor.savePreset.click();
  await expect(editor.dialogName).toHaveValue("Mirran Spy");
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

  bridge.reply("Studio.SavePreset", saved);
  await editor.dialogName.fill("Spy with flying");
  await editor.dialogConfirm.click();
  await expect(editor.userPresets).toHaveCount(1);
  await expect(editor.userPresets.locator(".chip").first()).toHaveAttribute("aria-pressed", "true");
  const [req] = bridge.calledWith("Studio.SavePreset").at(-1);
  expect(req).toMatchObject({ label: "Spy with flying", face: 0 });
  expect(req.edits.name).toBe("Mirran Spy");
  expect(bridge.calledWith("Studio.ResolvePreset").some(([id]) => id === saved.id)).toBe(true);

  const remove = editor.userPresets.getByRole("button", { name: "Remove the test card Spy with flying" });
  bridge.reply("System.Confirm", false);
  await remove.click();
  await expect.poll(() => bridge.calledWith("System.Confirm").length).toBe(1);
  await expect(editor.userPresets).toHaveCount(1);
  bridge.reply("System.Confirm", true);
  bridge.reply("Studio.DeletePreset", null);
  await remove.click();
  await expect(editor.userPresets).toHaveCount(0);
  expect(bridge.calledWith("Studio.DeletePreset")).toEqual([[saved.id]]);
});

test("the coverage strip draws every test card small, names one it cannot draw, and goes out of date after an edit", async ({ bridge, page }) => {
  // The coverage renders are answered one at a time, each as the page asks
  let issued = 0;
  const strip = [];
  let answered = 0;
  bridge.reply("Studio.Render", (req) => {
    const id = `job-${++issued}`;
    if (req.coverage) strip.push(id);
    return { jobId: id, dpi: req.coverage ? 40 : 150 };
  });
  const answer = async (count, failAt = 0) => {
    for (let i = 1; i <= count; i++) {
      const at = answered++;
      await expect.poll(() => strip.length).toBeGreaterThan(at);
      await bridge.job(strip[at], [i === failAt ? { seq: 1, done: true, error: "no frame for a land with this condition" } : done(1)]);
    }
  };
  const editor = await ready(bridge, page);
  await editor.coverage.locator("summary").click();
  await expect(editor.tiles).toHaveCount(6);
  await editor.coverageRun.click();
  await answer(6, 6);
  await expect(editor.coverageLine).toHaveText("5 of 6 drawn. Not drawn: Land");
  const land = editor.tiles.filter({ hasText: "Land" });
  await expect(land).toHaveAttribute("data-state", "failed");
  await expect(land).toContainText("no frame for a land with this condition");
  const renders = bridge.calledWith("Studio.Render").slice(1).map(([r]) => r);
  expect(renders.every((r) => r.coverage === true && r.id === "mine" && !r.edits)).toBe(true);
  expect(renders.map((r) => r.card.Name)).toEqual(["Mirran Spy", "Consuming Aberration", "Eerie Ultimatum", "Norin the Wary", "Sensei's Divining Top", "Flooded Strand"]);

  // Choosing a tile tests the template with that card
  await editor.tiles.filter({ hasText: "Artifact" }).getByRole("button").click();
  await expect(editor.cardLine).toContainText("Sensei's Divining Top");

  await editor.layers.first().click();
  await page.keyboard.press("ArrowRight");
  await expect(editor.coverageLine).toHaveText("5 of 6 drawn. Not drawn: Land. The template has changed since");
  await expect(editor.tiles.first()).toHaveAttribute("data-stale", "true");
  await editor.coverageRun.click();
  await answer(6);
  await expect(editor.coverageLine).toHaveText("6 of 6 drawn");
  await expect(editor.tiles.first()).toHaveAttribute("data-stale", "false");
});

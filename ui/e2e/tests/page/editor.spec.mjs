import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, progress, draftSummary, draftView, card, presetCards } from "../../bridge/cards.mjs";

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

const ART = "https://cards.scryfall.io/art_crop/front/1/2/12345.jpg";
const found = (name, extra = {}) => ({ text: name, card: card(name, { ArtworkURL: ART, SetCode: "c21", TypeLine: "Artifact", ...extra }) });

test("the editor opens the newest template and draws the first preset with it", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect(editor.workspace).toBeVisible();
  await expect(editor.picker.locator("option")).toHaveText(["Newer frame", "Older frame"]);
  await expect.poll(() => bridge.calledWith("Studio.Open")).toEqual([["newer"]]);

  // With nothing remembered, the first preset is looked up and is the card
  await expect(editor.presets).toHaveText(["Monocolored", "Two Colors", "Three Colors", "Legendary", "Artifact", "Land"]);
  await expect.poll(() => bridge.calledWith("Studio.ResolvePreset")).toEqual([["monocolored"]]);
  await expect(editor.cardLine).toHaveText("Mirran Spy · MBS");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  const [request] = bridge.calledWith("Studio.Render")[0];
  expect(request.id).toBe("newer");
  expect(request.card).toMatchObject({ Name: "Mirran Spy", SetCode: "mbs", ArtworkURL: /mirran-spy/ });
  expect(request.face).toBe(0);

  await expect(editor.activity).toHaveAttribute("data-state", "working");
  await finishJob(bridge, "job-1");
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Mirran Spy");
  await expect(editor.preview).toHaveAttribute("src", /\/img\/render\/job-1/);
  await expect(editor.preview).toBeVisible();
});

test("a template tested with an edited card opens with it, and no preset is in use", async ({ bridge, page }) => {
  const edited = card("Edited Card", { ArtworkURL: ART, Power: "7", Toughness: "7", SetCode: "2x2" });
  const editor = await withDrafts(bridge, page, { views: { newer: draftView(newer, { meta: { testCard: edited, face: 0 } }) } });
  await expect(editor.cardLine).toHaveText("Edited Card · 2X2");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  expect(bridge.calledWith("Studio.Render")[0][0].card.Name).toBe("Edited Card");
  await editor.openFine();
  await expect(editor.cardField("power")).toHaveValue("7");
  await expect(editor.presets.filter({ has: page.locator("xpath=self::*[@aria-pressed='true']") })).toHaveCount(0);
  await expect(page.locator("#ed-fine-revert")).toBeHidden();
});

test("a render that fails or is refused says why and leaves no image", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await finishJob(bridge, "job-1", { error: "rendering \"Mirran Spy\": a layer image is missing" });
  await expect(editor.activity).toHaveAttribute("data-state", "error");
  await expect(page.locator("#ed-activity-title")).toHaveText("Could not draw the card");
  await expect(page.locator("#ed-activity-step")).toContainText("a layer image is missing");
  await expect(editor.preview).toBeHidden();

  bridge.fail("Studio.Render", "This template does not render transform front faces");
  await editor.preset("Land").click();
  await expect(page.locator("#ed-activity-step")).toHaveText("This template does not render transform front faces");
  await expect(editor.preview).toBeHidden();
});

test("choosing a preset remembers it and draws it, and a newer choice replaces an older one", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await finishJob(bridge, "job-1");
  await expect(editor.activity).toHaveAttribute("data-state", "done");

  await editor.preset("Land").click();
  await expect(editor.cardLine).toHaveText("Flooded Strand · KTK");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  // Choosing again before the first is drawn abandons it
  await editor.preset("Artifact").click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(3);
  await expect.poll(() => bridge.calledWith("Render.Cancel")).toContainEqual(["job-2"]);

  const remembered = bridge.calledWith("Studio.SetTestCard");
  expect(remembered.map(([req]) => [req.id, req.card.Name, req.face])).toEqual([
    ["newer", "Mirran Spy", 0],
    ["newer", "Flooded Strand", 0],
    ["newer", "Sensei's Divining Top", 0],
  ]);

  // Only the newest job's image is shown
  await finishJob(bridge, "job-2");
  await expect(editor.activity).not.toHaveAttribute("data-state", "done");
  await finishJob(bridge, "job-3");
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.preview).toHaveAttribute("src", /\/img\/render\/job-3/);
});

test("a preset that cannot be looked up says why, and a slow one gives way to a newer choice", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await finishJob(bridge, "job-1");
  bridge.fail("Studio.ResolvePreset", "Could not look up Flooded Strand (KTK 233): scryfall answered 503");
  await editor.preset("Land").click();
  await expect(page.locator("#ed-activity-title")).toHaveText("Could not look up Flooded Strand");
  await expect(page.locator(".toast.err")).toContainText("KTK 233");
  // The card that was in use is still the one the template is tested with
  await expect(editor.cardLine).toHaveText("Mirran Spy · MBS");
  expect(bridge.calledWith("Studio.SetTestCard").map(([req]) => req.card.Name)).toEqual(["Mirran Spy"]);

  let release;
  const slow = new Promise((resolve) => (release = resolve));
  bridge.reply("Studio.ResolvePreset", async (id) => {
    if (id === "land") await slow;
    return presetCards[id];
  });
  await editor.preset("Land").click();
  await editor.preset("Artifact").click();
  await expect(editor.cardLine).toHaveText("Sensei's Divining Top · 2X2");
  release();
  await page.waitForTimeout(300);
  await expect(editor.cardLine).toHaveText("Sensei's Divining Top · 2X2");
  await expect(editor.preset("Artifact")).toHaveAttribute("aria-pressed", "true");
});

test("a template that does not load lists its problems and can be neither drawn, used nor exported", async ({ bridge, page }) => {
  const broken = draftView(newer, {
    valid: false,
    layouts: [],
    diagnostics: [
      { severity: "error", path: "layouts/normal.json", message: "unexpected end of JSON input" },
      { severity: "warning", path: "template.json", message: "the template has no description" },
    ],
  });
  const editor = await withDrafts(bridge, page, { views: { newer: broken } });

  await expect(editor.sectionToggle("problems")).toHaveAttribute("aria-expanded", "true");
  await expect(editor.problems).toHaveText(["layouts/normal.json: unexpected end of JSON input", "template.json: the template has no description"]);
  await expect(editor.problems.first()).toHaveClass(/error/);
  await expect(editor.problems.nth(1)).toHaveClass(/warning/);
  await expect(page.locator("#ed-problems-count")).toHaveText("2");
  await expect(page.locator("#ed-problems-count")).toHaveAttribute("data-level", "error");
  await expect(editor.activity).toHaveAttribute("data-state", "error");
  await expect(page.locator("#ed-activity-title")).toHaveText("This template does not load");
  await expect(editor.preview).toBeHidden();
  expect(bridge.calledWith("Studio.Render")).toHaveLength(0);
  await expect(editor.useButton).toBeDisabled();
  await expect(editor.exportButton).toBeDisabled();
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

test("with no templates the editor offers a new one, says why one is refused, and opens one that is made", async ({ bridge, page }) => {
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect(editor.empty).toBeVisible();

  bridge.fail("Studio.Create", "A name is at most 80 characters");
  await editor.openNewDialog();
  await editor.dialogName.fill("A fine name");
  await editor.dialogConfirm.click();
  await expect(editor.dialogError).toHaveText("A name is at most 80 characters");
  await expect(editor.dialog).toBeVisible();

  const made = draftSummary("my-frame", "My frame");
  bridge.reply("Studio.Create", made);
  bridge.reply("Studio.List", [made]);
  bridge.reply("Studio.Open", draftView(made));
  await editor.dialogName.fill("  My frame ");
  await editor.dialogDpi.fill("300");
  await expect(page.locator("#draft-dpi-size")).toHaveText("A canvas of 816 × 1110 pixels");
  await editor.dialogConfirm.click();

  await expect(editor.dialog).toBeHidden();
  expect(bridge.calledWith("Studio.Create").at(-1)).toEqual([{ name: "My frame", dpi: 300 }]);
  await expect(editor.aboutName).toHaveText("My frame");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
});

test("editing the test card's fields draws it again once typing stops, and reverting gives the preset back", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await finishJob(bridge, "job-1");
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "true");
  await editor.openFine();
  await expect(editor.cardField("oracle")).toHaveValue(/Flying/);

  await editor.cardField("typeLine").fill("Land");
  await editor.cardField("power").fill("");
  await editor.cardField("toughness").fill("");
  await editor.fineDetails.getByRole("button", { name: "White" }).click();
  // The preset lets go as soon as a field is changed, and the card line follows the name
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "false");
  await editor.cardField("name").clear();
  await editor.cardField("name").pressSequentially("My Test Land", { delay: 20 });
  await expect(editor.cardLine).toHaveText("My Test Land · MBS");

  // A run of typing is drawn once, with the changes sent with it
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await page.waitForTimeout(600);
  expect(bridge.calledWith("Studio.Render").length).toBe(2);
  const [request] = bridge.calledWith("Studio.Render")[1];
  expect(request.card.Name).toBe("Mirran Spy");
  expect(request.edits).toMatchObject({ typeLine: "Land", power: "", toughness: "", manaCost: "{1}{U}", colors: "WU" });
  // The layout is described for the changed card once it is drawn, and the card is kept
  await finishJob(bridge, "job-2");
  await expect.poll(() => bridge.calledWith("Studio.Describe").at(-1)?.[0].edits?.typeLine).toBe("Land");
  await expect.poll(() => bridge.calledWith("Studio.SetTestCard").at(-1)?.[0].edits?.typeLine).toBe("Land");

  await page.locator("#ed-fine-revert").click();
  await expect(editor.cardField("typeLine")).toHaveValue("Creature — Human Rogue");
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#ed-fine-revert")).toBeHidden();
});

test("a searched card fills the fields and is drawn with its artwork, which an edit keeps", async ({ bridge, page }) => {
  bridge.reply("Cards.Search", [found("Sol Ring")]);
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await editor.search("sol ring");
  await editor.results.filter({ has: page.locator(".r-name", { hasText: "Sol Ring" }) }).first().click();
  await editor.openFine();
  await expect(editor.cardField("name")).toHaveValue("Sol Ring");
  await expect(editor.cardField("setCode")).toHaveValue("c21");
  await expect(editor.cardLine).toHaveText("Sol Ring · C21");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  expect(bridge.calledWith("Studio.Render")[1][0].card.ArtworkURL).toBe(ART);
  // A card with art asks the app to describe the layout with an art window to fill
  await finishJob(bridge, "job-2");
  await expect.poll(() => bridge.calledWith("Studio.Describe").at(-1)?.[0].card.ArtworkURL).toBe(ART);

  await editor.cardField("name").fill("Sol Ring, Tuned");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(3);
  const [edited] = bridge.calledWith("Studio.Render")[2];
  expect(edited.card.ArtworkURL).toBe(ART);
  expect(edited.edits.name).toBe("Sol Ring, Tuned");
  expect(bridge.calledWith("Studio.SetTestCard").at(-1)[0].card.ArtworkURL).toBe(ART);
});

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

test("the editor opens the newest template and draws a test card with it", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect(editor.workspace).toBeVisible();
  await expect(editor.picker.locator("option")).toHaveText(["Newer frame", "Older frame"]);
  await expect.poll(() => bridge.calledWith("Studio.Open")).toEqual([["newer"]]);

  // With nothing remembered, the first preset is the card
  await expect(editor.cardLine).toHaveText("Mirran Spy · MBS");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  const [request] = bridge.calledWith("Studio.Render")[0];
  expect(request.id).toBe("newer");
  expect(request.card.Name).toBe("Mirran Spy");
  expect(request.face).toBe(0);

  await expect(editor.activity).toHaveAttribute("data-state", "working");
  await finishJob(bridge, "job-1");
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(page.locator("#ed-activity-title")).toHaveText("Drew Mirran Spy");
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
  await finishJob(bridge, "job-1", { error: "rendering \"Mirran Spy\": a layer image is missing" });

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

// The fields of the test card

const ART = "https://cards.scryfall.io/art_crop/front/1/2/12345.jpg";
const found = (name, extra = {}) => ({ text: name, card: card(name, { ArtworkURL: ART, SetCode: "c21", TypeLine: "Artifact", ...extra }) });

test("the card's fields are the properties of the test card, open to begin with and showing the preset's values", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  // Properties is a part of the test card section and not a section of its own
  await expect(page.locator('[data-sec="card"] #ed-fine-details')).toHaveCount(1);
  await expect(editor.fineSummary).toContainText("Properties");
  expect(await editor.fineDetails.evaluate((el) => el.open)).toBe(true);
  await expect(page.locator("#ed-card-form")).toBeVisible();
  await editor.collapseFine();
  await expect(page.locator("#ed-presets")).toBeVisible();
  await editor.openFine();
  await expect(editor.cardField("name")).toHaveValue("Mirran Spy");
  await expect(editor.cardField("manaCost")).toHaveValue("{1}{U}");
  await expect(editor.cardField("typeLine")).toHaveValue("Creature — Human Rogue");
  await expect(editor.cardField("power")).toHaveValue("1");
  await expect(editor.cardField("toughness")).toHaveValue("1");
  await expect(editor.cardField("oracle")).toHaveValue(/Flying/);
  await expect(editor.fineDetails.getByRole("button", { name: "Blue" })).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#ed-fine-revert")).toBeHidden();
});

test("changing a field draws the card again with the change, and the preset is no longer the one in use", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await finishJob(bridge, "job-1");
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "true");
  await editor.openFine();

  await editor.cardField("typeLine").fill("Land");
  await editor.cardField("power").fill("");
  await editor.cardField("toughness").fill("");
  // The preset lets go as soon as a field is changed, and the card line follows the name
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "false");
  await editor.cardField("name").fill("My Test Land");
  await expect(editor.cardLine).toHaveText("My Test Land · MBS");
  await expect(page.locator("#ed-fine-revert")).toBeVisible();

  // The card is drawn again a moment after the typing stops, with the changes sent with it
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  const [request] = bridge.calledWith("Studio.Render")[1];
  expect(request.card.Name).toBe("Mirran Spy");
  expect(request.edits).toMatchObject({ name: "My Test Land", typeLine: "Land", power: "", toughness: "", manaCost: "{1}{U}" });
  // The layout is described for the changed card once it is drawn, which is what decides
  // what draws
  await finishJob(bridge, "job-2");
  await expect.poll(() => bridge.calledWith("Studio.Describe").at(-1)?.[0].edits?.typeLine).toBe("Land");
  // And the card is kept with the changes
  await expect.poll(() => bridge.calledWith("Studio.SetTestCard").length).toBeGreaterThan(0);
  expect(bridge.calledWith("Studio.SetTestCard").at(-1)[0].edits.name).toBe("My Test Land");
});

test("a run of typing draws the card once", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await editor.openFine();
  await editor.cardField("name").pressSequentially("Quick", { delay: 40 });
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await page.waitForTimeout(600);
  expect(bridge.calledWith("Studio.Render").length).toBe(2);
});

test("reverting the fields gives the preset back", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await editor.openFine();
  await editor.cardField("oracle").fill("Something else");
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "false");
  await page.locator("#ed-fine-revert").click();
  await expect(editor.cardField("oracle")).toHaveValue(/Flying/);
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#ed-fine-revert")).toBeHidden();
  await expect.poll(() => bridge.calledWith("Studio.Render").at(-1)[0].edits.oracle).toMatch(/Flying/);
});

test("the colors are toggled like Single's, and they go with the card", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await editor.openFine();
  await editor.fineDetails.getByRole("button", { name: "White" }).click();
  await expect(editor.fineDetails.getByRole("button", { name: "White" })).toHaveAttribute("aria-pressed", "true");
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "false");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  expect(bridge.calledWith("Studio.Render")[1][0].edits.colors).toBe("WU");
});

test("choosing another card, a preset or a searched one, fills the fields with its values", async ({ bridge, page }) => {
  bridge.reply("Cards.Search", [found("Sol Ring")]);
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await editor.openFine();
  await editor.cardField("name").fill("Changed");

  await editor.preset("Land").click();
  await expect(editor.cardField("name")).toHaveValue("Flooded Strand");
  await expect(editor.preset("Land")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#ed-fine-revert")).toBeHidden();

  await editor.search("sol ring");
  await editor.results.filter({ has: page.locator(".r-name", { hasText: "Sol Ring" }) }).first().click();
  await expect(editor.cardField("name")).toHaveValue("Sol Ring");
  await expect(editor.cardField("typeLine")).toHaveValue("Artifact");
  await expect(editor.cardField("setCode")).toHaveValue("c21");
  await expect(editor.cardLine).toHaveText("Sol Ring · C21");
});

test("a searched card goes to the draft with the artwork it came with, and so does an edited one", async ({ bridge, page }) => {
  bridge.reply("Cards.Search", [found("Sol Ring")]);
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await editor.search("sol ring");
  await editor.results.filter({ has: page.locator(".r-name", { hasText: "Sol Ring" }) }).first().click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  const [chosen] = bridge.calledWith("Studio.Render")[1];
  expect(chosen.card.Name).toBe("Sol Ring");
  expect(chosen.card.ArtworkURL).toBe(ART);
  // A card with art asks the app to describe the layout with an art window to fill
  await finishJob(bridge, "job-2");
  await expect.poll(() => bridge.calledWith("Studio.Describe").at(-1)?.[0].card.ArtworkURL).toBe(ART);
  // Editing it keeps the art, which the card carries and the form does not
  await editor.openFine();
  await editor.cardField("name").fill("Sol Ring, Tuned");
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(3);
  const [edited] = bridge.calledWith("Studio.Render")[2];
  expect(edited.card.ArtworkURL).toBe(ART);
  expect(edited.edits.name).toBe("Sol Ring, Tuned");
  expect(bridge.calledWith("Studio.SetTestCard").at(-1)[0].card.ArtworkURL).toBe(ART);
});

test("a template opened with an edited test card has it, and no preset is in use", async ({ bridge, page }) => {
  const edited = card("Edited Card", { ArtworkURL: ART, Power: "7", Toughness: "7" });
  const editor = await withDrafts(bridge, page, { views: { newer: draftView(newer, { meta: { testCard: edited, face: 0 } }) } });
  await editor.openFine();
  await expect(editor.cardField("name")).toHaveValue("Edited Card");
  await expect(editor.cardField("power")).toHaveValue("7");
  await expect(editor.presets.filter({ has: page.locator("xpath=self::*[@aria-pressed='true']") })).toHaveCount(0);
  await expect(page.locator("#ed-fine-revert")).toBeHidden();
});

// The sections of the left column

test("the left column is the template, its about, its canvas, the search, and the test card with its properties, in that order", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  const top = async (sel) => (await page.locator(sel).boundingBox()).y;
  const order = [await top(".ed-draft"), await top("#ed-about-sec"), await top("#ed-canvas-sec"), await top("#ed-search-sec"), await top('[data-sec="card"]'), await top("#ed-fine-details")];
  expect(order).toEqual([...order].sort((a, b) => a - b));
  await expect(editor.fineSummary).toContainText("Properties");
  await expect(editor.searchToggle).toContainText("Search Scryfall");
  // Nothing is called fine-tuning any more
  await expect(page.locator("#mode-editor")).not.toContainText("Fine-tune");
});

test("the search starts collapsed with its arrow, and opens to the search and its results", async ({ bridge, page }) => {
  bridge.reply("Cards.Search", [found("Sol Ring")]);
  bridge.reply("Settings.Put", (s) => s);
  const editor = await withDrafts(bridge, page);
  await expect(editor.searchToggle).toHaveAttribute("aria-expanded", "false");
  await expect(editor.searchInput).toBeHidden();
  await expect(page.locator("#ed-results")).toBeHidden();
  await expect((await editor.searchToggle.locator(".icon").boundingBox()).width).toBeGreaterThanOrEqual(20);
  // The search is above the test card
  const card = await page.locator('[data-sec="card"]').boundingBox();
  const search = await page.locator("#ed-search-sec").boundingBox();
  expect(search.y).toBeLessThan(card.y);

  await editor.openSearch();
  await expect(editor.searchInput).toBeVisible();
  await expect(page.locator("#ed-results-count")).toHaveText("Pick a preset above, or search for a card");
  await editor.search("sol ring");
  await expect(editor.results).toHaveCount(1);
  await expect(page.locator("#ed-results-count")).toHaveText("1 result");
  await expect.poll(() => bridge.calledWith("Settings.Put").length).toBeGreaterThan(0);
  expect(bridge.calledWith("Settings.Put").at(-1)[0].splits.editorLeftSections.slice(0, 2)).toEqual([0, 0]);
});

test("undo and redo are in the template's section, which says they are for all of it", async ({ bridge, page }) => {
  await withDrafts(bridge, page);
  await expect(page.locator(".ed-draft #ed-undo")).toBeVisible();
  await expect(page.locator(".ed-draft #ed-redo")).toBeVisible();
  // They are not among the layers any more, where the add button stays
  await expect(page.locator('[data-sec="layers"] #ed-undo, [data-sec="layers"] #ed-redo')).toHaveCount(0);
  await expect(page.locator('[data-sec="layers"] #ed-add')).toHaveCount(1);
  // They are in the heading row of the template, beside its name
  const undo = await page.locator("#ed-undo").boundingBox();
  const select = await page.locator("#ed-draft-select").boundingBox();
  expect(undo.y).toBeLessThan(select.y);
  await expect(page.locator("#ed-undo")).toBeDisabled();
});

// The presets are real cards that are looked up

test("the presets are six real cards, and the first is looked up when a template opens with no card", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect(editor.presets).toHaveText(["Monocolored", "Two Colors", "Three Colors", "Legendary", "Artifact", "Land"]);
  await expect.poll(() => bridge.calledWith("Studio.ResolvePreset")).toEqual([["monocolored"]]);
  await expect(editor.preset("Monocolored")).toHaveAttribute("aria-pressed", "true");
  await expect(editor.cardLine).toHaveText("Mirran Spy · MBS");
  // The card the draft is drawn with is the one that was looked up, with its art
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  expect(bridge.calledWith("Studio.Render")[0][0].card).toMatchObject({ Name: "Mirran Spy", SetCode: "mbs", CollectorNumber: "26", ArtworkURL: /mirran-spy/ });
});

test("each preset is looked up by its id when it is chosen", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  const names = { "Two Colors": "Consuming Aberration · GTC", "Three Colors": "Eerie Ultimatum · IKO", Legendary: "Norin the Wary · TSP", Artifact: "Sensei's Divining Top · 2X2", Land: "Flooded Strand · KTK" };
  for (const [label, line] of Object.entries(names)) {
    await editor.preset(label).click();
    await expect(editor.cardLine).toHaveText(line);
    await expect(editor.preset(label)).toHaveAttribute("aria-pressed", "true");
  }
  expect(bridge.calledWith("Studio.ResolvePreset").map(([id]) => id)).toEqual(["monocolored", "two-colors", "three-colors", "legendary", "artifact", "land"]);
});

test("a preset that cannot be looked up says why and leaves the card as it was", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await finishJob(bridge, "job-1");
  await expect(editor.cardLine).toHaveText("Mirran Spy · MBS");
  bridge.fail("Studio.ResolvePreset", "Could not look up Flooded Strand (KTK 233): scryfall answered 503");
  await editor.preset("Land").click();
  await expect(page.locator("#ed-activity-title")).toHaveText("Could not look up Flooded Strand");
  await expect(page.locator("#ed-activity-step")).toContainText("scryfall answered 503");
  await expect(page.locator(".toast.err")).toContainText("KTK 233");
  // The card that was in use is still the one the template is tested with
  await expect(editor.cardLine).toHaveText("Mirran Spy · MBS");
  expect(bridge.calledWith("Studio.SetTestCard").map(([req]) => req.card.Name)).toEqual(["Mirran Spy"]);
  // And asking again, once it can be found, works
  bridge.reply("Studio.ResolvePreset", (id) => ({ ...presetCards[id] }));
  await editor.preset("Land").click();
  await expect(editor.cardLine).toHaveText("Flooded Strand · KTK");
});

test("a newer choice stands when an older preset is still being looked up", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
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

// The sections of the left column

test("the test card and the search are sections that collapse and open, starting with only the test card open", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  const toggles = { card: page.locator("#ed-card-toggle"), search: editor.searchToggle };
  await expect(toggles.card).toHaveAttribute("aria-expanded", "true");
  await expect(toggles.search).toHaveAttribute("aria-expanded", "false");
  await expect(page.locator("#ed-presets")).toBeVisible();
  await toggles.card.click();
  await expect(page.locator("#ed-card-body")).toBeHidden();
  await expect(toggles.card.locator(".icon")).toHaveCSS("width", "20px");
  await toggles.card.click();
  await expect(page.locator("#ed-presets")).toBeVisible();
});

test("the template, its about and the canvas stay as they are, above the sections that can be resized", async ({ bridge, page }) => {
  await withDrafts(bridge, page);
  await expect(page.locator(".ed-draft")).toBeVisible();
  await expect(page.locator("#ed-canvas-sec")).toBeVisible();
  // They are not among the sections that share the column's height
  await expect(page.locator('#ed-left-sections .ed-draft, #ed-left-sections #ed-about-sec, #ed-left-sections #ed-canvas-sec')).toHaveCount(0);
  const template = await page.locator(".ed-draft").boundingBox();
  const canvas = await page.locator("#ed-canvas-sec").boundingBox();
  const sections = await page.locator("#ed-left-sections").boundingBox();
  expect(canvas.y).toBeGreaterThanOrEqual(template.y + template.height - 1);
  expect(sections.y).toBeGreaterThanOrEqual(canvas.y + canvas.height - 1);
});

test("a line between the open sections of the left column is dragged to share their height", async ({ bridge, page }) => {
  bridge.reply("Settings.Put", (s) => s);
  const editor = await withDrafts(bridge, page);
  await editor.openSearch();
  await expect(page.locator("#ed-left-sections .ed-vsplit:visible")).toHaveCount(1);
  const height = async (sel) => (await page.locator(sel).boundingBox()).height;
  const before = [await height('[data-sec="search"]'), await height('[data-sec="card"]')];
  const split = await page.locator("#ed-left-sections .ed-vsplit:visible").first().boundingBox();
  await page.mouse.move(split.x + split.width / 2, split.y + 1);
  await page.mouse.down();
  await page.mouse.move(split.x + split.width / 2, split.y + 41, { steps: 6 });
  await page.mouse.up();
  const after = [await height('[data-sec="search"]'), await height('[data-sec="card"]')];
  expect(after[0] - before[0]).toBeGreaterThan(30);
  expect(after[0] - before[0]).toBeLessThan(50);
  expect(after[0] + after[1]).toBeCloseTo(before[0] + before[1], 0);
  await expect.poll(() => bridge.calledWith("Settings.Put").length).toBeGreaterThan(0);
  const kept = bridge.calledWith("Settings.Put").at(-1)[0].splits.editorLeftSections;
  expect(kept).toHaveLength(4);
  expect(kept.slice(0, 2)).toEqual([0, 0]);
});

test("the search and the test card share the height, and the test card takes the room when the search is collapsed", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await editor.openSearch();
  await expect(page.locator("#ed-left-sections .ed-vsplit:visible")).toHaveCount(1);
  const split = page.locator("#ed-left-sections .ed-vsplit:visible").first();
  await expect(split).toHaveAttribute("aria-label", "Resize Search Scryfall and Test card");
  // With the search collapsed again, it is at the top of the column and the test card takes the room
  await editor.searchToggle.click();
  const column = await page.locator("#ed-left-sections").boundingBox();
  const search = await page.locator('[data-sec="search"]').boundingBox();
  const card = await page.locator('[data-sec="card"]').boundingBox();
  expect(Math.abs(search.y - column.y)).toBeLessThan(2);
  expect(Math.abs(card.y - (search.y + search.height))).toBeLessThan(2);
  expect(Math.abs(card.y + card.height - (column.y + column.height))).toBeLessThan(2);
});

test("a line in the left column is moved with the arrow keys and never under the minimum", async ({ bridge, page }) => {
  const editor = await withDrafts(bridge, page);
  await editor.openSearch();
  const split = page.locator("#ed-left-sections .ed-vsplit:visible").first();
  const search = async () => (await page.locator('[data-sec="search"]').boundingBox()).height;
  const card = async () => (await page.locator('[data-sec="card"]').boundingBox()).height;
  const before = await search();
  await split.focus();
  await page.keyboard.press("ArrowDown");
  await expect.poll(async () => Math.abs((await search()) - (before + 16)) < 2).toBe(true);
  for (let i = 0; i < 60; i++) await page.keyboard.press("ArrowUp");
  await expect.poll(async () => (await search()) >= 95).toBe(true);
  for (let i = 0; i < 120; i++) await page.keyboard.press("ArrowDown");
  await expect.poll(async () => (await card()) >= 95).toBe(true);
});

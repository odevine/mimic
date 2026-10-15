import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { at, done, drawn, draftSummary, draftView, describeView, standardItems } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The standard items with a crown above the frame that the card does not draw, and does when
// the request says to show it
const crown = (shown) => (shown ? drawn("layer", "crown", at(100, 50, 200, 80), { part: "crown" }) : { kind: "layer", key: "crown", part: "crown", drawn: false, why: 'its condition "legendary" does not hold for this card', authored: at(0, 0, 0, 0), delivered: at(0, 0, 0, 0) });

async function ready(bridge, page) {
  bridge.reply("Studio.Describe", (req) => describeView([standardItems[0], standardItems[1], crown((req.show || []).includes("layer:crown")), ...standardItems.slice(2)]));
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.layers).toHaveCount(8);
  const box = await editor.sizeCanvas(400);
  return { editor, box };
}

const eye = (editor, name) => editor.row(name).locator('[data-toggle="visible"]');
const renders = (bridge) => bridge.calledWith("Studio.Render").map(([req]) => req);
const describes = (bridge) => bridge.calledWith("Studio.Describe").map(([req]) => req);

test("every row has an eye that says whether the preview shows it", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  for (const name of ["background", "frame", "title", "oracle"]) {
    await expect(eye(editor, name)).toHaveAttribute("aria-pressed", "true");
    await expect(eye(editor, name)).toHaveAttribute("data-eye", "shown");
    await expect(eye(editor, name)).toHaveAttribute("data-tip", "Show/Hide");
    await expect(eye(editor, name).locator("use")).toHaveAttribute("href", "icons.svg#i-eye");
  }
  // A layer the card does not draw has its eye off, and one that is not a layer cannot be forced
  await expect(eye(editor, "crown")).toHaveAttribute("data-eye", "idle");
  await expect(eye(editor, "crown").locator("use")).toHaveAttribute("href", "icons.svg#i-eye-off");
  await expect(eye(editor, "pt")).toBeDisabled();
});

test("hiding an item draws the card again without it, and showing it puts it back", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  expect(renders(bridge)[0]).toMatchObject({ hide: [], show: [] });
  const described = describes(bridge).length;
  await eye(editor, "title").click();
  await expect(eye(editor, "title")).toHaveAttribute("aria-pressed", "false");
  await expect(eye(editor, "title")).toHaveAttribute("data-eye", "hidden");
  await expect(editor.row("title")).toHaveClass(/hidden-item/);
  await expect.poll(() => renders(bridge).length).toBe(2);
  expect(renders(bridge)[1]).toMatchObject({ hide: ["text:title"], show: [], coverage: false });
  // What the card draws has not changed, so the layout is not described again
  expect(describes(bridge).length).toBe(described);
  await bridge.job("job-2", [done(1)]);
  await eye(editor, "frame").click();
  await expect.poll(() => renders(bridge).length).toBe(3);
  expect(renders(bridge)[2].hide.sort()).toEqual(["layer:frame", "text:title"]);
  await eye(editor, "title").click();
  await expect.poll(() => renders(bridge).length).toBe(4);
  expect(renders(bridge)[3].hide).toEqual(["layer:frame"]);
  await expect(eye(editor, "title")).toHaveAttribute("data-eye", "shown");
});

test("what is hidden is not picked on the card, and the list still reaches it", async ({ bridge, page }) => {
  const { editor, box } = await ready(bridge, page);
  const over = EditorPage.at(box, 300, 120);
  await page.mouse.click(over.x, over.y);
  await expect(editor.selectionName).toHaveText("title");
  await eye(editor, "title").click();
  await page.mouse.click(over.x, over.y);
  await expect(editor.selectionName).toHaveText("frame");
  await select(editor, "title");
  await expect(editor.selectionName).toHaveText("title");
  await expect(page.locator("#ed-selection-note")).toHaveText("Hidden in this preview only");
});

async function select(editor, name) {
  await editor.layers.filter({ has: editor.page.locator(".ed-layer-name", { hasText: new RegExp(`^${name}$`) }) }).click();
}

test("a layer the card does not draw can be forced to draw, and the layout is described again with it", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  expect(describes(bridge)[0].show).toEqual([]);
  await eye(editor, "crown").click();
  await expect.poll(() => describes(bridge).length).toBeGreaterThan(1);
  expect(describes(bridge).at(-1).show).toEqual(["layer:crown"]);
  await expect.poll(() => renders(bridge).length).toBe(2);
  expect(renders(bridge)[1]).toMatchObject({ hide: [], show: ["layer:crown"] });
  // It is drawn now, the eye says it is forced, and the layer is on the card to be picked
  await expect(eye(editor, "crown")).toHaveAttribute("data-eye", "forced");
  await expect(eye(editor, "crown")).toHaveAttribute("aria-pressed", "true");
  await expect(editor.row("crown")).not.toHaveClass(/undrawn/);
  await select(editor, "crown");
  await expect(page.locator("#ed-selection-note")).toHaveText("Drawn whatever its condition says, for this preview only");

  await eye(editor, "crown").click();
  await expect.poll(() => describes(bridge).at(-1).show).toEqual([]);
  await expect(eye(editor, "crown")).toHaveAttribute("data-eye", "idle");
  await expect(editor.row("crown")).toHaveClass(/undrawn/);
  await expect(page.locator("#ed-selection-note")).toContainText("Not drawn: its condition");
});

test("the eyes put nothing in the template and nothing is saved", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await eye(editor, "title").click();
  await eye(editor, "crown").click();
  // The two changes are drawn once, a moment after the second
  await expect.poll(() => renders(bridge).at(-1).show).toEqual(["layer:crown"]);
  expect(renders(bridge).at(-1).hide).toEqual(["text:title"]);
  expect(bridge.calledWith("Studio.Apply")).toEqual([]);
  expect(bridge.calledWith("Studio.SetMarks")).toEqual([]);
  await expect(editor.undoButton).toBeDisabled();
});

test("a coverage render ignores what the preview hides", async ({ bridge, page }) => {
  bridge.reply("Studio.ResolvePreset", () => ({ Name: "Mirran Spy", TypeLine: "Creature" }));
  const { editor } = await ready(bridge, page);
  await eye(editor, "title").click();
  await editor.coverage.locator("summary").click();
  await editor.coverageRun.click();
  await expect.poll(() => renders(bridge).some((r) => r.coverage)).toBe(true);
  const strip = renders(bridge).find((r) => r.coverage);
  expect(strip.hide).toEqual([]);
  expect(strip.show).toEqual([]);
});

test("opening another template starts with everything showing", async ({ bridge, page }) => {
  const other = draftSummary("other", "Other");
  bridge.reply("Studio.Describe", () => describeView(standardItems));
  bridge.reply("Studio.List", [mine, other]);
  bridge.reply("Studio.Open", (id) => draftView(id === "other" ? other : mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => renders(bridge).length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  await eye(editor, "title").click();
  await expect(eye(editor, "title")).toHaveAttribute("data-eye", "hidden");
  await editor.picker.selectOption("other");
  await expect.poll(() => renders(bridge).length).toBe(3);
  expect(renders(bridge).at(-1).hide).toEqual([]);
  await bridge.job("job-3", [done(1)]);
  await expect(eye(editor, "title")).toHaveAttribute("data-eye", "shown");
});

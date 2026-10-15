import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");
const NAMES = ["layers", "selection", "problems"];
const sec = (name) => `[data-sec="${name}"]`;

async function ready(bridge, page, { open, settings = {} } = {}) {
  bridge.reply("Settings.Put", (s) => s);
  if (settings.splits) bridge.reply("Settings.Get", { ...bridge.defaults["Settings.Get"], ...settings });
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", open || draftView(mine));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  return editor;
}

const box = async (page, selector) => (await page.locator(selector).boundingBox()) ?? { x: 0, y: 0, width: 0, height: 0 };
const bottom = (b) => b.y + b.height;

test("the column starts with the layers and selection open, the layers with the most room", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  for (const name of ["layers", "selection"]) await expect(editor.sectionToggle(name)).toHaveAttribute("aria-expanded", "true");
  await expect(editor.sectionToggle("problems")).toHaveAttribute("aria-expanded", "false");
  await expect(editor.sectionBody("problems")).toBeHidden();
  const layers = await box(page, sec("layers"));
  const selection = await box(page, sec("selection"));
  expect(layers.height).toBeGreaterThan(selection.height * 1.2);
});

test("every section collapses to its heading and opens again", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  for (const name of NAMES) {
    await editor.openSection(name);
    const open = await box(page, sec(name));
    await editor.collapseSection(name);
    const closed = await box(page, sec(name));
    expect(closed.height, `${name} collapsed`).toBeLessThan(50);
    expect(closed.height).toBeLessThan(open.height);
    await expect(editor.sectionToggle(name)).toHaveAttribute("aria-expanded", "false");
    await editor.openSection(name);
    await expect(editor.sectionToggle(name)).toHaveAttribute("aria-expanded", "true");
  }
});

test("a section's arrow is large, points down while open and turns when it is collapsed", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  const icon = editor.sectionToggle("layers").locator(".icon");
  const open = await icon.evaluate((el) => ({ w: el.getBoundingClientRect().width, transform: getComputedStyle(el).transform }));
  expect(open.w).toBeGreaterThanOrEqual(20);
  expect(open.transform).toBe("none");
  await editor.collapseSection("layers");
  const closed = await icon.evaluate((el) => getComputedStyle(el).transform);
  expect(closed).not.toBe("none");
  // The whole heading is the control, and it shows it can be pressed
  const toggle = await editor.sectionToggle("selection").boundingBox();
  const sectionBox = await box(page, sec("selection"));
  expect(toggle.width).toBeGreaterThan(sectionBox.width * 0.6);
});

test("the buttons stay at the bottom whatever the sections do", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  const footer = () => box(page, ".ed-footer");
  const column = await box(page, ".ed-side");
  const first = await footer();
  expect(bottom(first)).toBeCloseTo(bottom(column), 0);
  for (const name of ["use", "export", "json"]) await expect(page.locator(`#ed-${name}`)).toBeVisible();
  for (const name of NAMES) {
    await editor.openSection(name);
    expect((await footer()).y).toBeCloseTo(first.y, 0);
  }
  for (const name of NAMES) {
    await editor.collapseSection(name);
    expect((await footer()).y).toBeCloseTo(first.y, 0);
  }
  await expect(page.locator("#ed-json")).toBeVisible();
});

test("collapsed sections at the bottom rest against the buttons, and the open ones take the room", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  const footer = await box(page, ".ed-footer");
  // The problems are collapsed, and touch the buttons
  expect(bottom(await box(page, sec("problems")))).toBeCloseTo(footer.y, 0);
  expect(bottom(await box(page, sec("selection")))).toBeLessThan((await box(page, sec("problems"))).y + 2);
  const column = await box(page, "#ed-sections");
  // The open sections fill what the collapsed ones leave
  const used = (await box(page, sec("layers"))).height + (await box(page, sec("selection"))).height + (await box(page, sec("problems"))).height;
  expect(used).toBeCloseTo(column.height, -1);

  // With every section collapsed they are still against the buttons, with the room above them
  await editor.collapseSection("layers");
  await editor.collapseSection("selection");
  const layers = await box(page, sec("layers"));
  expect(bottom(await box(page, sec("problems")))).toBeCloseTo(footer.y, 0);
  expect(layers.y).toBeGreaterThan(column.y + 100);
  expect(bottom(layers)).toBeCloseTo((await box(page, sec("selection"))).y, 0);

  // A section in the middle that is collapsed stays where it is, between two that are open
  await editor.openSection("layers");
  await editor.openSection("problems");
  const middle = await box(page, sec("selection"));
  expect(middle.y).toBeCloseTo(bottom(await box(page, sec("layers"))), 0);
  expect(bottom(middle)).toBeCloseTo((await box(page, sec("problems"))).y, 0);
});

test("the template's details are always shown in full, with nothing to collapse", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openSection("about");
  expect(await page.locator("#ed-about-body details, #ed-about-body summary").count()).toBe(0);
  for (const id of ["ed-template-levers", "ed-credits", "ed-credit-add", "ed-fonts", "ed-font-add", "ed-roles"]) await expect(page.locator(`#${id}`)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Template details" })).toBeVisible();
});

test("dragging the line between two open sections shares their height differently and keeps the total", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(page.locator(".ed-vsplit:visible")).toHaveCount(1);
  const split = page.locator(".ed-vsplit:visible").first();
  const before = { layers: await box(page, sec("layers")), selection: await box(page, sec("selection")) };
  const at = await split.boundingBox();
  await page.mouse.move(at.x + at.width / 2, at.y + 1);
  await page.mouse.down();
  await page.mouse.move(at.x + at.width / 2, at.y + 61, { steps: 6 });
  await page.mouse.up();
  const after = { layers: await box(page, sec("layers")), selection: await box(page, sec("selection")) };
  expect(after.layers.height - before.layers.height).toBeGreaterThan(50);
  expect(after.layers.height - before.layers.height).toBeLessThan(70);
  expect(after.layers.height + after.selection.height).toBeCloseTo(before.layers.height + before.selection.height, 0);
  void editor;
  // It is kept with the other layout settings, as which are collapsed and how much room each has
  await expect.poll(() => bridge.calledWith("Settings.Put").length).toBeGreaterThan(0);
  const kept = bridge.calledWith("Settings.Put").at(-1)[0].splits.editorSections;
  expect(kept).toHaveLength(6);
  expect(kept.slice(0, 3)).toEqual([0, 0, 1]);
  expect(kept[3]).toBeGreaterThan(kept[4]);
});

test("a line cannot be dragged past what leaves a section its minimum", async ({ bridge, page }) => {
  await ready(bridge, page);
  const split = page.locator(".ed-vsplit:visible").first();
  const at = await split.boundingBox();
  await page.mouse.move(at.x + at.width / 2, at.y + 1);
  await page.mouse.down();
  await page.mouse.move(at.x + at.width / 2, at.y - 2000, { steps: 8 });
  await page.mouse.up();
  expect((await box(page, sec("layers"))).height).toBeGreaterThanOrEqual(95);
  const again = await page.locator(".ed-vsplit:visible").first().boundingBox();
  await page.mouse.move(again.x + again.width / 2, again.y + 1);
  await page.mouse.down();
  await page.mouse.move(again.x + again.width / 2, again.y + 2000, { steps: 8 });
  await page.mouse.up();
  expect((await box(page, sec("selection"))).height).toBeGreaterThanOrEqual(95);
});

test("the arrow keys move a line too", async ({ bridge, page }) => {
  await ready(bridge, page);
  const split = page.locator(".ed-vsplit:visible").first();
  const layers = async () => (await box(page, sec("layers"))).height;
  const before = await layers();
  await split.focus();
  await page.keyboard.press("ArrowDown");
  // Sixteen pixels, give or take what rounding a share of the room leaves
  await expect.poll(async () => Math.abs((await layers()) - (before + 16)) < 2).toBe(true);
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");
  await expect.poll(async () => Math.abs((await layers()) - (before - 16)) < 2).toBe(true);
});

test("a line is there only between two sections that are open", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(page.locator(".ed-vsplit:visible")).toHaveCount(1);
  await editor.collapseSection("selection");
  await expect(page.locator(".ed-vsplit:visible")).toHaveCount(0);
  await editor.openSection("problems");
  await expect(page.locator(".ed-vsplit:visible")).toHaveCount(1);
  await editor.openSection("selection");
  await expect(page.locator(".ed-vsplit:visible")).toHaveCount(2);
  await expect(page.locator(".ed-vsplit:visible").first()).toHaveAttribute("aria-label", "Resize Layers and Selection");
});

test("how the column was left is kept for the next time", async ({ bridge, page }) => {
  const settings = { splits: { editorSections: [1, 0, 0, 100, 400, 200] } };
  const editor = await ready(bridge, page, { settings });
  await expect(editor.sectionToggle("layers")).toHaveAttribute("aria-expanded", "false");
  await expect(editor.sectionToggle("selection")).toHaveAttribute("aria-expanded", "true");
  await expect(editor.sectionToggle("problems")).toHaveAttribute("aria-expanded", "true");
  // The selection has twice the problems' room, as it was left
  const selection = (await box(page, sec("selection"))).height;
  const problems = (await box(page, sec("problems"))).height;
  expect(selection / problems).toBeGreaterThan(1.7);
  expect(selection / problems).toBeLessThan(2.3);
});

test("a template that does not load has its problems shown, and the count says how bad", async ({ bridge, page }) => {
  const broken = draftSummary("mine", "Mine");
  bridge.reply("Studio.List", [broken]);
  bridge.reply("Settings.Put", (s) => s);
  bridge.reply(
    "Studio.Open",
    draftView(broken, { valid: false, layouts: [], diagnostics: [{ severity: "error", path: "layouts/normal.json", message: "unexpected end of JSON input" }] }),
  );
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect(editor.sectionToggle("problems")).toHaveAttribute("aria-expanded", "true");
  await expect(editor.problems).toHaveCount(1);
  await expect(editor.problems.first()).toBeVisible();
  await expect(page.locator("#ed-problems-count")).toHaveAttribute("data-level", "error");
  // And it can still be collapsed
  await editor.collapseSection("problems");
  await expect(editor.sectionToggle("problems")).toHaveAttribute("aria-expanded", "false");
});

test("warnings leave the problems collapsed with a count in the heading", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, {
    open: draftView(mine, { diagnostics: [{ severity: "warning", path: "template.json", message: "the template has no description" }] }),
  });
  await expect(editor.sectionToggle("problems")).toHaveAttribute("aria-expanded", "false");
  await expect(page.locator("#ed-problems-count")).toHaveText("1");
  await expect(page.locator("#ed-problems-count")).toHaveAttribute("data-level", "warning");
  await expect(page.locator("#ed-problems-count")).toBeVisible();
});

test("the canvas is under the template in the left column and not in the right", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(page.locator("#ed-canvas-sec")).toBeVisible();
  await expect(editor.canvasToggle).toContainText("Canvas");
  // Nothing in the right column is called Layout any more, and the canvas is in the left one
  await expect(page.locator('[data-region="2"] #ed-canvas-sec, [data-region="2"] [data-sec="layout"], #ed-sec-layout-toggle')).toHaveCount(0);
  await expect(page.locator('[data-region="0"] #ed-canvas-levers')).toHaveCount(1);
  const template = await box(page, ".ed-draft");
  const canvas = await box(page, "#ed-canvas-sec");
  const testCard = await box(page, '[data-sec="card"]');
  expect(canvas.y).toBeGreaterThanOrEqual(template.y + template.height - 1);
  expect(canvas.y + canvas.height).toBeLessThanOrEqual(testCard.y + 1);
});

test("the canvas starts collapsed, with an arrow as large as the sections', and opens to its fields", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(editor.canvasToggle).toHaveAttribute("aria-expanded", "false");
  await expect(editor.canvasBody).toBeHidden();
  const closed = await box(page, "#ed-canvas-sec");
  const icon = editor.canvasToggle.locator(".icon");
  expect((await icon.boundingBox()).width).toBeGreaterThanOrEqual(20);
  expect(await icon.evaluate((el) => getComputedStyle(el).transform)).not.toBe("none");
  await editor.openCanvas();
  // The arrow turns over a moment, so it is read until it has
  await expect.poll(() => icon.evaluate((el) => getComputedStyle(el).transform)).toBe("none");
  for (const id of ["canvas-width", "canvas-height", "canvas-dpi"]) await expect(page.locator(`#${id}`)).toBeVisible();
  expect((await box(page, "#ed-canvas-sec")).height).toBeGreaterThan(closed.height + 60);
  await editor.collapseCanvas();
});

test("whether the canvas is open is kept, as the other layout is", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  await expect.poll(() => bridge.calledWith("Settings.Put").length).toBeGreaterThan(0);
  expect(bridge.calledWith("Settings.Put").at(-1)[0].splits.editorCanvas).toEqual([0]);
  await editor.collapseCanvas();
  await expect.poll(() => bridge.calledWith("Settings.Put").at(-1)[0].splits.editorCanvas).toEqual([1]);
});

test("a canvas left open is open again", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { settings: { splits: { editorCanvas: [0] } } });
  await expect(editor.canvasToggle).toHaveAttribute("aria-expanded", "true");
  await expect(editor.canvasBody).toBeVisible();
});

test("about is under the template in the left column, above the canvas, and starts collapsed", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(page.locator('[data-region="2"] #ed-about-sec, #ed-sec-about-toggle')).toHaveCount(0);
  await expect(page.locator('[data-region="0"] #ed-about-sec')).toHaveCount(1);
  await expect(editor.sectionToggle("about")).toContainText("About");
  await expect(editor.sectionToggle("about")).toHaveAttribute("aria-expanded", "false");
  await expect(editor.sectionBody("about")).toBeHidden();
  const template = await box(page, ".ed-draft");
  const about = await box(page, "#ed-about-sec");
  const canvas = await box(page, "#ed-canvas-sec");
  expect(about.y).toBeGreaterThanOrEqual(template.y + template.height - 1);
  expect(bottom(about)).toBeLessThanOrEqual(canvas.y + 1);
  await editor.openSection("about");
  await expect(editor.aboutName).toHaveText("Mine");
  await expect(editor.aboutId).toHaveText("user.mine");
  await editor.collapseSection("about");
});

test("whether about is open is kept with the layout, as the canvas is", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openSection("about");
  await expect.poll(() => bridge.calledWith("Settings.Put").length).toBeGreaterThan(0);
  expect(bridge.calledWith("Settings.Put").at(-1)[0].splits.editorAbout).toEqual([0]);
  await editor.collapseSection("about");
  await expect.poll(() => bridge.calledWith("Settings.Put").at(-1)[0].splits.editorAbout).toEqual([1]);
});

test("the search is above the test card in the left column and starts collapsed", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await expect(editor.searchToggle).toHaveAttribute("aria-expanded", "false");
  const search = await box(page, '[data-sec="search"]');
  const card = await box(page, '[data-sec="card"]');
  expect(search.y).toBeLessThan(card.y);
  expect(bottom(search)).toBeLessThanOrEqual(card.y + 1);
  await expect(page.locator("#ed-card-toggle")).toHaveAttribute("aria-expanded", "true");
});

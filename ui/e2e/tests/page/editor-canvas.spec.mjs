import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, describeView, standardItems, drawn, at } from "../../bridge/cards.mjs";
import { solidPng } from "../../png.mjs";

const mine = draftSummary("mine", "Mine");
const other = draftSummary("other", "Other");

// The editor on one template with its test card drawn and its layout described, and
// the preview the size of a real one: 400 pixels across for a card 1000 pixels across,
// so a pixel on screen is two and a half of the finished image's. With picture set,
// the card is drawn from a picture 400 by 560, big enough to zoom
async function ready(bridge, page, { describe, meta, drafts = [mine], count = 7, picture = false } = {}) {
  if (picture) await page.route("**/img/render/**", (route) => route.fulfill({ contentType: "image/png", body: solidPng(400, 560, [60, 90, 140, 255]) }));
  bridge.reply("Studio.List", drafts);
  bridge.reply("Studio.Open", (id) => draftView(drafts.find((d) => d.id === id) || mine, meta ? { meta } : {}));
  if (describe) bridge.reply("Studio.Describe", describe);
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.layers).toHaveCount(count);
  if (picture) {
    await expect.poll(() => editor.preview.evaluate((img) => img.complete && img.naturalWidth)).toBe(400);
    return { editor };
  }
  const box = await editor.sizeCanvas(400);
  return { editor, box, point: (x, y) => EditorPage.at(box, x, y) };
}

const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);
const renders = (bridge) => bridge.calledWith("Studio.Render").map(([req]) => req);
const describes = (bridge) => bridge.calledWith("Studio.Describe").map(([req]) => req);
const select = (editor, name) => editor.row(name).click();
const eye = (editor, name) => editor.row(name).locator('[data-toggle="visible"]');
const imageBox = (page) => page.locator("#ed-img").boundingBox();
const label = (page) => page.locator("#ed-zoom-label");

// drag presses at a place on the finished image and moves by a distance on screen
async function drag(page, from, by, { steps = 6 } = {}) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + by.x, from.y + by.y, { steps });
}

// dragFree drags with snapping held off
async function dragFree(page, from, by) {
  await page.keyboard.down("Alt");
  await drag(page, from, by);
  await page.mouse.up();
  await page.keyboard.up("Alt");
}

test("the layers list and the card select the same items, and selecting edits nothing", async ({ bridge, page }) => {
  const { editor, box, point } = await ready(bridge, page);
  await expect(editor.layers.locator(".ed-layer-name")).toHaveText(["pt", "oracle", "type", "title", "Art window", "frame", "background"]);
  await expect(editor.layers.first()).toHaveClass(/undrawn/);
  await expect(editor.selection).toBeHidden();

  // A click picks what is on top there, and clicking it again keeps it
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectedRow.locator(".ed-layer-name")).toHaveText("title");
  await expect(page.locator("#ed-sel-x")).toHaveText("100");
  await expect(page.locator("#ed-sel-w")).toHaveText("600");
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");

  // An item that is not drawn is read from the list, and a click there reaches what is drawn
  const pt = point(800, 1280);
  await page.mouse.click(pt.x, pt.y);
  await expect(editor.selectionName).toHaveText("frame");
  await editor.layers.first().click();
  await expect(editor.selectionName).toHaveText("pt");
  await expect(page.locator("#ed-selection-note")).toContainText("Not drawn: the card has no pt to draw");

  // The arrow keys move through the list
  await page.keyboard.press("ArrowDown");
  await expect(editor.selectedRow.locator(".ed-layer-name")).toHaveText("oracle");

  // The item outlined on hover is the one a click selects
  await page.mouse.move(p.x, p.y);
  await expect(page.locator("#ed-overlay .ed-box.hover")).toHaveCount(1);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");

  // Off the card the selection is let go, and Escape lets go too
  await page.mouse.click(box.x - 30, box.y + box.height / 2);
  await expect(editor.selection).toBeHidden();
  await page.mouse.click(box.x + 1, box.y + 1);
  await expect(editor.selectionName).toHaveText("background");
  await page.keyboard.press("Escape");
  await expect(editor.selection).toBeHidden();
  expect(applied(bridge)).toEqual([]);
});

test("dragging moves the pressed item, snaps to the card's lines, and saves one edit on release", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  // A press that barely moves is a click
  const p = point(150, 100);
  await drag(page, p, { x: 1, y: 1 }, { steps: 1 });
  await page.mouse.up();
  await expect(editor.selectionName).toHaveText("title");
  expect(applied(bridge)).toEqual([]);

  // While it is held the box follows the pointer and nothing is saved
  await page.keyboard.down("Alt");
  await drag(page, p, { x: 40, y: 20 });
  await expect(page.locator("#ed-sel-x")).toHaveText("200");
  await expect(page.locator("#ed-sel-y")).toHaveText("130");
  expect(applied(bridge)).toEqual([]);
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0]).toEqual({ id: "mine", ops: [{ op: "move", layout: "normal", kind: "text", key: "title", dx: 100, dy: 50 }] });

  // With something beneath, the item pressed is the one that moves
  await dragFree(page, point(300, 400), { x: 20, y: 0 });
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toEqual({ op: "move", layout: "normal", kind: "art", key: "0", dx: 50, dy: 0 });

  // An edge near the card's edge snaps to it, with a guide while it is held
  await drag(page, point(150, 100), { x: -39, y: 0 });
  await expect(editor.guides).toHaveCount(1);
  await expect(editor.guides.first()).toContainText("left to ");
  await expect(page.locator("#ed-sel-x")).toHaveText("0");
  await page.mouse.up();
  await expect(editor.guides).toHaveCount(0);
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ op: "move", key: "title", dx: -100, dy: 0 });
});

test("handles resize a box down to a pixel, and a layer or a baseline box has none", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  await page.mouse.click(point(150, 100).x, point(150, 100).y);
  // The title is short on screen, so it shows its corners and the top and bottom
  await expect(editor.handles).toHaveCount(6);
  const corner = page.locator('#ed-overlay .ed-handle[data-handle="se"]');
  const c = await corner.boundingBox();
  await page.keyboard.down("Alt");
  await drag(page, { x: c.x + c.width / 2, y: c.y + c.height / 2 }, { x: 20, y: 10 });
  await expect(page.locator("#ed-sel-w")).toHaveText("650");
  await expect(page.locator("#ed-sel-h")).toHaveText("105");
  await page.mouse.up();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "resize", layout: "normal", kind: "text", key: "title", rect: { x: 100, y: 80, width: 650, height: 105 } }]);

  const again = await corner.boundingBox();
  await drag(page, { x: again.x + again.width / 2, y: again.y + again.height / 2 }, { x: -900, y: -900 });
  await expect(page.locator("#ed-sel-w")).toHaveText("1");
  await expect(page.locator("#ed-sel-h")).toHaveText("1");
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0].rect).toMatchObject({ width: 1, height: 1 });

  await select(editor, "background");
  await expect(editor.selectionBox).toHaveCount(1);
  await expect(editor.handles).toHaveCount(0);

  // A text box shown by where its text is moves and does not resize
  const type = point(280, 810);
  await page.mouse.click(type.x, type.y);
  await expect(editor.selectionName).toHaveText("type");
  await expect(page.locator("#ed-sel-y")).toHaveText("790");
  await expect(editor.handles).toHaveCount(0);
  await dragFree(page, type, { x: 0, y: 12 });
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toEqual({ op: "move", layout: "normal", kind: "text", key: "type", dx: 0, dy: 30 });
});

test("arrow keys nudge as one edit, undo and redo take it back by button and key, and a refused edit leaves nothing to undo", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("Shift+ArrowDown");
  await expect(page.locator("#ed-sel-x")).toHaveText("103");
  await expect(page.locator("#ed-sel-y")).toHaveText("90");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "move", layout: "normal", kind: "text", key: "title", dx: 3, dy: 10 }]);
  await expect(editor.undoButton).toBeEnabled();
  await expect(editor.redoButton).toBeDisabled();

  // Typing in a field leaves the selection where it is
  await editor.openSearch();
  await editor.searchInput.focus();
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(400);
  expect(applied(bridge)).toHaveLength(1);

  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "move", key: "title", dx: -3, dy: -10 });
  await expect(editor.undoButton).toBeDisabled();
  await editor.redoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ dx: 3, dy: 10 });
  await page.locator("#ed-stage").click({ position: { x: 5, y: 5 } });
  await page.keyboard.press("ControlOrMeta+z");
  await expect.poll(() => applied(bridge).length).toBe(4);
  await page.keyboard.press("ControlOrMeta+Shift+z");
  await expect.poll(() => applied(bridge).length).toBe(5);
  expect(applied(bridge)[4].ops[0].dx).toBe(3);

  // A refused edit is said, and the box goes back to where the app says it is
  bridge.fail("Studio.Apply", "That change would leave the template unable to load");
  await page.mouse.click(p.x, p.y);
  await editor.undoButton.click();
  await expect(page.locator("#toasts")).toContainText("unable to load");
});

test("each edit is followed by a fresh description and a new render, and a selection that is gone is let go", async ({ bridge, page }) => {
  let moved = 0;
  let gone = false;
  const { editor, point } = await ready(bridge, page, {
    describe: () =>
      describeView(
        standardItems
          .filter((it) => !gone || it.key !== "title")
          .map((it) => (it.key === "title" ? { ...it, delivered: at(100 + moved, 80, 600, 80), authored: at(100 + moved, 80, 600, 80) } : it)),
      ),
  });
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  const before = describes(bridge).length;
  bridge.reply("Studio.Apply", (req) => {
    moved = req.ops[0].dx;
    return { rev: 1, inverse: [{ ...req.ops[0], dx: -req.ops[0].dx }], valid: true, diagnostics: [] };
  });
  await dragFree(page, p, { x: 40, y: 0 });
  await expect(page.locator("#ed-sel-x")).toHaveText("200");
  expect(describes(bridge).length).toBeGreaterThan(before);
  // The card is drawn again once the edits stop, and the new image replaces the old
  await expect.poll(() => renders(bridge).length).toBe(2);
  await bridge.job("job-2", [done(1)]);
  await expect(editor.preview).toHaveAttribute("src", /job-2/);

  gone = true;
  await editor.preset("Land").click();
  await expect.poll(() => renders(bridge).length).toBe(3);
  await bridge.job("job-3", [done(1)]);
  await expect(editor.selection).toBeHidden();
  await expect(editor.layers).toHaveCount(6);
});

test("a layout that cannot be edited can be looked at and not changed", async ({ bridge, page }) => {
  const reason = "This layout extends another one, so its positions are not in its own file. Edit it in the JSON";
  const { editor, point } = await ready(bridge, page, { describe: () => describeView(standardItems, { editable: false, reason }) });
  await expect(page.locator("#ed-edit-note")).toHaveText(reason);
  await expect(editor.addButton).toBeDisabled();
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");
  await expect(editor.handles).toHaveCount(0);
  await expect(editor.moreButton("title")).toBeDisabled();
  await expect(editor.layers.locator(".ed-layer-grip")).toHaveCount(0);

  await drag(page, p, { x: 40, y: 0 });
  await page.mouse.up();
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(400);
  expect(applied(bridge)).toEqual([]);
});

test("a layer moves in the stack from its menu, by key and by its grip, and text has no place in it", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await editor.moreButton("background").click();
  await expect(editor.menuItem("Move down")).toHaveCount(0);
  await editor.menuItem("Move up").click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "reorder", layout: "normal", kind: "layer", key: "background", to: 1 }]);
  await editor.moreButton("oracle").click();
  await expect(editor.menuItem("Duplicate")).toBeVisible();
  await expect(editor.menuItem("Move up")).toHaveCount(0);
  await page.keyboard.press("Escape");

  await editor.row("background").locator(".ed-layer-select").focus();
  await page.keyboard.press("Alt+ArrowUp");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "reorder", key: "background", to: 1 });
  await editor.row("title").locator(".ed-layer-select").focus();
  await page.keyboard.press("Alt+ArrowUp");

  // Only layers have a grip, and a drop where the row already is changes nothing
  await expect(editor.layers.locator(".ed-layer-grip")).toHaveCount(2);
  const own = await editor.grip("frame").boundingBox();
  const ownRow = await editor.row("frame").boundingBox();
  await page.mouse.move(own.x + own.width / 2, own.y + own.height / 2);
  await page.mouse.down();
  await page.mouse.move(own.x + 4, ownRow.y + ownRow.height / 2 + 2, { steps: 4 });
  await page.mouse.up();
  await page.waitForTimeout(300);
  expect(applied(bridge)).toHaveLength(2);

  const grip = await editor.grip("background").boundingBox();
  const frame = await editor.row("frame").boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + 4, frame.y + 3, { steps: 6 });
  await expect(editor.row("frame")).toHaveClass(/drop-before/);
  await page.mouse.up();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops).toEqual([{ op: "reorder", layout: "normal", kind: "layer", key: "background", to: 1 }]);
});

test("a locked item cannot be picked or moved on the card, by its padlock or the L key", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  await select(editor, "title");
  await editor.lockToggle("title").click();
  await expect.poll(() => bridge.calledWith("Studio.SetMarks").length).toBe(1);
  expect(bridge.calledWith("Studio.SetMarks")[0][0]).toEqual({ id: "mine", locked: ["normal/text:title"], snapTo: [] });
  await expect(editor.lockToggle("title")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("#ed-selection-note")).toHaveText("Locked, so the card cannot pick or move it");
  await expect(editor.handles).toHaveCount(0);
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(400);
  expect(applied(bridge)).toEqual([]);

  // A click where it lies reaches what is beneath
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("frame");

  // The list still reaches it, and the L key unlocks it
  await select(editor, "title");
  await page.keyboard.press("l");
  await expect(editor.lockToggle("title")).toHaveAttribute("aria-pressed", "false");
  expect(bridge.calledWith("Studio.SetMarks").at(-1)[0].locked).toEqual([]);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");
});

test("a drag snaps to an item only once it is marked, and the marks come back when the template opens", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page, { meta: { locked: ["normal/text:oracle"], snapTo: [] } });
  await expect(editor.lockToggle("oracle")).toHaveAttribute("aria-pressed", "true");
  // Opening it did not write what it read
  expect(bridge.calledWith("Studio.SetMarks")).toEqual([]);

  // The type line's left edge is at 100 as the title's is, and a pixel on screen is two and a half
  await drag(page, point(300, 810), { x: -3, y: 0 });
  await expect(editor.guides).toHaveCount(0);
  await page.mouse.up();
  await expect.poll(() => applied(bridge).length).toBe(1);

  await editor.snapToggle("title").click();
  await expect.poll(() => bridge.calledWith("Studio.SetMarks").length).toBe(1);
  expect(bridge.calledWith("Studio.SetMarks")[0][0]).toEqual({ id: "mine", locked: ["normal/text:oracle"], snapTo: ["normal/text:title"] });
  await drag(page, point(300, 810), { x: -3, y: 0 });
  await expect(editor.guides.first()).toContainText("left to title left");
  await expect(page.locator("#ed-overlay .ed-box.snap")).toHaveCount(1);
  await page.mouse.up();
  await expect(page.locator("#ed-overlay .ed-box.snap")).toHaveCount(0);
});

test("the view zooms by button, label, key and wheel, and leaves keys to a field being typed in", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page, { picture: true });
  await expect(label(page)).toHaveText("Fit");
  const fit = await imageBox(page);
  await page.locator("#ed-zoom-in").click();
  expect((await imageBox(page)).width).toBeGreaterThan(fit.width);
  await page.locator("#ed-zoom-fit").click();
  await expect(label(page)).toHaveText("Fit");

  // 100% is one of the picture's pixels for one of the screen's
  const ratio = await page.evaluate(() => window.devicePixelRatio);
  await label(page).click();
  await expect(label(page)).toHaveText("100%");
  expect(Math.round((await imageBox(page)).width)).toBe(Math.round(400 / ratio));
  await page.keyboard.press("2");
  await expect(label(page)).toHaveText("200%");
  await page.keyboard.press("-");
  await expect(label(page)).toHaveText("150%");
  await page.keyboard.press("0");
  await expect(label(page)).toHaveText("Fit");

  // The wheel zooms about the pointer and stops at the most and the least
  const box = await imageBox(page);
  const at = { x: box.x + box.width * 0.25, y: box.y + box.height * 0.33 };
  await page.mouse.move(at.x, at.y);
  await page.mouse.wheel(0, -240);
  const zoomed = await imageBox(page);
  expect(zoomed.width).toBeGreaterThan(box.width);
  expect(Math.abs((at.x - zoomed.x) / zoomed.width - 0.25)).toBeLessThan(0.02);
  expect(Math.abs((at.y - zoomed.y) / zoomed.height - 0.33)).toBeLessThan(0.02);
  for (let i = 0; i < 12; i++) await page.mouse.wheel(0, -600);
  await expect(label(page)).toHaveText("800%");
  for (let i = 0; i < 40; i++) await page.mouse.wheel(0, 600);
  await expect(label(page)).toHaveText("10%");

  await page.keyboard.press("0");
  await editor.openFine();
  await editor.cardField("name").focus();
  await page.keyboard.type("a1+2-");
  await expect(label(page)).toHaveText("Fit");
  await expect(editor.cardField("name")).toHaveValue(/a1\+2-$/);
});

test("the pan tool, Space and the middle button move the card, and selecting still lands after", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page, { picture: true });
  const panBy = async (by, { button = "left" } = {}) => {
    const b = await imageBox(page);
    const from = { x: b.x + b.width / 2, y: b.y + b.height / 2 };
    await page.mouse.move(from.x, from.y);
    await page.mouse.down({ button });
    await page.mouse.move(from.x + by.x, from.y + by.y, { steps: 5 });
    await page.mouse.up({ button });
    const a = await imageBox(page);
    return { x: Math.round(a.x - b.x), y: Math.round(a.y - b.y) };
  };
  const fit = await imageBox(page);

  await page.locator("#ed-tool-pan").click();
  expect(await panBy({ x: 90, y: -40 })).toEqual({ x: 90, y: -40 });
  await expect(editor.selection).toBeHidden();
  await page.locator("#ed-tool-select").click();

  await page.mouse.move(fit.x + fit.width / 2, fit.y + fit.height / 2);
  await page.keyboard.down("Space");
  await expect(page.locator("#ed-stage")).toHaveClass(/pan-mode/);
  expect(await panBy({ x: -60, y: 30 })).toEqual({ x: -60, y: 30 });
  await page.keyboard.up("Space");
  await expect(page.locator("#ed-stage")).not.toHaveClass(/pan-mode/);

  expect(await panBy({ x: 40, y: 40 }, { button: "middle" })).toEqual({ x: 40, y: 40 });
  expect(applied(bridge)).toEqual([]);

  // The title is at 100,100 of the 1000 by 1400 card, wherever the card is on screen now
  await page.keyboard.press("1");
  const overlay = await page.locator("#ed-overlay").boundingBox();
  const p = EditorPage.at(overlay, 150, 100);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");

  await page.locator("#ed-zoom-fit").click();
  const back = await imageBox(page);
  expect(Math.abs(back.x - fit.x) + Math.abs(back.y - fit.y)).toBeLessThan(1);
});

test("the eyes hide items from the preview or force one to draw, and save nothing", async ({ bridge, page }) => {
  // A crown above the frame that the card does not draw, and does when the request says to show it
  const crown = (shown) =>
    shown
      ? drawn("layer", "crown", at(100, 50, 200, 80), { part: "crown" })
      : { kind: "layer", key: "crown", part: "crown", drawn: false, why: 'its condition "legendary" does not hold for this card', authored: at(0, 0, 0, 0), delivered: at(0, 0, 0, 0) };
  const { editor, point } = await ready(bridge, page, {
    count: 8,
    describe: (req) => describeView([standardItems[0], standardItems[1], crown((req.show || []).includes("layer:crown")), ...standardItems.slice(2)]),
  });
  await expect(eye(editor, "crown")).toHaveAttribute("data-eye", "idle");
  await expect(eye(editor, "pt")).toBeDisabled();
  const described = describes(bridge).length;

  // Hiding draws the card again without it, and the card no longer picks it
  await eye(editor, "title").click();
  await expect(eye(editor, "title")).toHaveAttribute("data-eye", "hidden");
  await expect.poll(() => renders(bridge).length).toBe(2);
  expect(renders(bridge)[1]).toMatchObject({ hide: ["text:title"], show: [], coverage: false });
  expect(describes(bridge).length).toBe(described);
  await bridge.job("job-2", [done(1)]);
  const over = point(300, 120);
  await page.mouse.click(over.x, over.y);
  await expect(editor.selectionName).toHaveText("frame");
  await select(editor, "title");
  await expect(page.locator("#ed-selection-note")).toHaveText("Hidden in this preview only");

  // A layer the card does not draw is forced to, and the layout is described with it
  await eye(editor, "crown").click();
  await expect.poll(() => describes(bridge).at(-1).show).toEqual(["layer:crown"]);
  await expect.poll(() => renders(bridge).at(-1).show).toEqual(["layer:crown"]);
  expect(renders(bridge).at(-1).hide).toEqual(["text:title"]);
  await expect(eye(editor, "crown")).toHaveAttribute("data-eye", "forced");
  await expect(editor.row("crown")).not.toHaveClass(/undrawn/);

  await eye(editor, "title").click();
  await eye(editor, "crown").click();
  await expect.poll(() => describes(bridge).at(-1).show).toEqual([]);
  await expect(eye(editor, "title")).toHaveAttribute("data-eye", "shown");
  await expect(editor.row("crown")).toHaveClass(/undrawn/);
  expect(bridge.calledWith("Studio.Apply")).toEqual([]);
  expect(bridge.calledWith("Studio.SetMarks")).toEqual([]);
  await expect(editor.undoButton).toBeDisabled();
});

test("opening another template resets the view, the selection, the undo history and what is hidden", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page, { drafts: [mine, other], picture: true });
  await page.keyboard.press("2");
  await expect(label(page)).toHaveText("200%");
  await select(editor, "oracle");
  await page.keyboard.press("ArrowRight");
  await expect.poll(() => applied(bridge).length).toBe(1);
  await expect(editor.undoButton).toBeEnabled();
  await eye(editor, "title").click();
  await expect(eye(editor, "title")).toHaveAttribute("data-eye", "hidden");

  await editor.picker.selectOption("other");
  await expect(label(page)).toHaveText("Fit");
  await expect(editor.undoButton).toBeDisabled();
  await expect(editor.selection).toBeHidden();
  await expect.poll(() => renders(bridge).at(-1).id).toBe("other");
  expect(renders(bridge).at(-1).hide).toEqual([]);
  // The newest render is answered until the other template's layers are listed, since
  // how many renders opening it starts depends on timing
  await expect
    .poll(async () => {
      await bridge.job(`job-${renders(bridge).length}`, [done(1)]);
      return eye(editor, "title").getAttribute("data-eye", { timeout: 200 }).catch(() => null);
    })
    .toBe("shown");
});

import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, describeView, standardItems, drawn, at } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// The editor on one template with its test card drawn and its layout described, and
// the preview the size of a real one: 400 pixels across for a card 1000 pixels across,
// so a pixel on screen is two and a half of the finished image's
async function ready(bridge, page, { describe, meta } = {}) {
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine, meta ? { meta } : {}));
  if (describe) bridge.reply("Studio.Describe", describe);
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.activity).toHaveAttribute("data-state", "done");
  await expect(editor.layers).toHaveCount(7);
  const box = await editor.sizeCanvas(400);
  return { editor, box, point: (x, y) => EditorPage.at(box, x, y) };
}

const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);

// drag presses at a place on the finished image and moves by a distance on screen
async function drag(page, from, by, { steps = 6 } = {}) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + by.x, from.y + by.y, { steps });
}

test("the layers are listed from the top, and the ones not drawn say why", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await expect(editor.layers.locator(".ed-layer-name")).toHaveText(["pt", "oracle", "type", "title", "Art window", "frame", "background"]);
  expect(await editor.layers.locator(".ed-layer-kind").evaluateAll((els) => els.map((el) => el.getAttribute("aria-label")))).toEqual(["Text box", "Text box", "Text box", "Text box", "Art window", "Layer", "Layer"]);
  // Each kind has an icon of its own, and a layer an image's
  expect(await editor.layers.locator(".ed-layer-kind use").evaluateAll((els) => els.map((el) => el.getAttribute("href").split("#")[1]))).toEqual(["i-text", "i-text", "i-text", "i-text", "i-select", "i-image", "i-image"]);
  await expect(editor.layers.first()).toHaveClass(/undrawn/);
  await expect(editor.layers.first()).toHaveAttribute("title", "the card has no pt to draw");
  await expect(editor.selection).toBeHidden();
});

test("clicking the card selects what is on top there, and clicking it again keeps it", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);

  await page.mouse.click(p.x, p.y);
  await expect(editor.selectedRow.locator(".ed-layer-name")).toHaveText("title");
  await expect(editor.selectionName).toHaveText("title");
  await expect(page.locator("#ed-selection-kind")).toHaveText("Text box");
  await expect(page.locator("#ed-sel-x")).toHaveText("100");
  await expect(page.locator("#ed-sel-w")).toHaveText("600");
  await expect(editor.selectionBox).toHaveCount(1);

  await page.mouse.click(p.x, p.y);
  await expect(editor.selectedRow.locator(".ed-layer-name")).toHaveText("title");
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectedRow.locator(".ed-layer-name")).toHaveText("title");

  // Nothing is edited by selecting
  expect(applied(bridge)).toEqual([]);
});

test("clicking the stage around the card lets go, and Escape does too", async ({ bridge, page }) => {
  const { editor, box } = await ready(bridge, page);
  await editor.layers.filter({ hasText: "oracle" }).click();
  await expect(editor.selectionName).toHaveText("oracle");
  await expect(editor.selectionBox).toHaveCount(1);

  // Off the card there is nothing to pick, and the selection is let go
  const stage = await page.locator("#ed-stage").boundingBox();
  expect(box.x - stage.x, "there is stage to the left of the card").toBeGreaterThan(60);
  await page.mouse.click(box.x - 30, box.y + box.height / 2);
  await expect(editor.selection).toBeHidden();
  await expect(editor.selectionBox).toHaveCount(0);

  // On the card the background fills it, so the corner picks it, and Escape lets go. The
  // top corner is used, since the card is taller than the room the stage has below it
  await page.mouse.click(box.x + 1, box.y + 1);
  await expect(editor.selectionName).toHaveText("background");
  await page.keyboard.press("Escape");
  await expect(editor.selection).toBeHidden();
  await expect(editor.selectionBox).toHaveCount(0);
});

test("a click selects the item the preview outlined on hover", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  // Select the frame, then hover where the title lies over it
  await editor.layers.filter({ hasText: "frame" }).click();
  const p = point(150, 100);
  await page.mouse.move(p.x, p.y);
  await expect(page.locator("#ed-overlay .ed-box.hover")).toHaveCount(1);
  await page.mouse.down();
  await page.mouse.up();
  await expect(editor.selectionName).toHaveText("title");
});

test("an item that is not drawn can be read in the list but not picked on the card", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(800, 1280);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("frame", "the pt box is not drawn, so the click reaches what is drawn there");

  await editor.layers.first().click();
  await expect(editor.selectionName).toHaveText("pt");
  await expect(page.locator("#ed-selection-note")).toContainText("Not drawn: the card has no pt to draw");
  await expect(page.locator("#ed-sel-x")).toHaveText("780");
  await expect(editor.selectionBox).toHaveClass(/idle/);
});

test("dragging moves the item and saves one edit when the mouse is let go", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  await page.keyboard.down("Alt");
  await drag(page, p, { x: 40, y: 20 });

  // While it is held the box follows the pointer and nothing is saved
  await expect(page.locator("#ed-sel-x")).toHaveText("200");
  await expect(page.locator("#ed-sel-y")).toHaveText("130");
  expect(applied(bridge)).toEqual([]);

  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0]).toEqual({ id: "mine", ops: [{ op: "move", layout: "normal", kind: "text", key: "title", dx: 100, dy: 50 }] });
  await expect(editor.undoButton).toBeEnabled();
});

test("dragging an item with something else under it moves the item that was pressed", async ({ bridge, page }) => {
  const { point } = await ready(bridge, page);
  const p = point(300, 400); // the art window is on top of the frame here
  await page.keyboard.down("Alt");
  await drag(page, p, { x: 20, y: 0 });
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0]).toEqual({ op: "move", layout: "normal", kind: "art", key: "0", dx: 50, dy: 0 });
});

test("an edge near a line snaps to it, and the guide shows while it is held", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  // The title's left edge is at 100, and 39 pixels left is 98 of the finished image's
  await drag(page, p, { x: -39, y: 0 });
  await expect(editor.guides).toHaveCount(1);
  await expect(editor.guides.first()).toHaveClass(/\bx\b/);
  await expect(editor.guides.first()).toContainText("left to ");
  await expect(page.locator("#ed-sel-x")).toHaveText("0");
  await page.mouse.up();
  await expect(editor.guides).toHaveCount(0);
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0]).toMatchObject({ op: "move", key: "title", dx: -100, dy: 0 });
});

test("the items a snap lines up with are outlined while it is held", async ({ bridge, page }) => {
  // The title and the type line are marked as lines to snap to, and nothing else is
  const { editor, point } = await ready(bridge, page, { meta: { snapTo: ["normal/text:title", "normal/text:type"] } });
  // The oracle box's left edge is at 100, as the title's and the type's are, and a pixel on screen is two and a half
  await drag(page, point(500, 1000), { x: -3, y: 0 });
  await expect(editor.guides.first()).toContainText("left to title left, type left");
  await expect(page.locator("#ed-overlay .ed-box.snap")).toHaveCount(2);
  await page.mouse.up();
  await expect(page.locator("#ed-overlay .ed-box.snap")).toHaveCount(0);
});

test("holding option while dragging turns snapping off", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  await page.keyboard.down("Alt");
  await drag(page, point(150, 100), { x: -39, y: 0 });
  await expect(editor.guides).toHaveCount(0);
  await expect(page.locator("#ed-sel-x")).toHaveText("3");
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  // 39 pixels on screen is 97.5 of the finished image's, which rounds up
  expect(applied(bridge)[0].ops[0].dx).toBe(-97);
});

test("a press that barely moves is a click and saves nothing", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");
  // A second press on the selected item that does not move keeps it
  await drag(page, p, { x: 1, y: 1 }, { steps: 1 });
  await page.mouse.up();
  await expect(editor.selectionName).toHaveText("title");
  expect(applied(bridge)).toEqual([]);
});

test("a handle resizes a text box, and a layer has none", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  // The title is short on screen, so it shows its corners and the top and bottom
  await expect(editor.handles).toHaveCount(6);
  await editor.layers.filter({ hasText: "oracle" }).click();
  await expect(editor.handles).toHaveCount(8);
  await editor.layers.filter({ hasText: "title" }).click();

  const corner = await page.locator('#ed-overlay .ed-handle[data-handle="se"]').boundingBox();
  const from = { x: corner.x + corner.width / 2, y: corner.y + corner.height / 2 };
  await page.keyboard.down("Alt");
  await drag(page, from, { x: 20, y: 10 });
  await expect(page.locator("#ed-sel-w")).toHaveText("650");
  await expect(page.locator("#ed-sel-h")).toHaveText("105");
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "resize", layout: "normal", kind: "text", key: "title", rect: { x: 100, y: 80, width: 650, height: 105 } }]);

  await editor.layers.filter({ hasText: "background" }).click();
  await expect(editor.selectionBox).toHaveCount(1);
  await expect(editor.handles).toHaveCount(0);
});

test("a box cannot be dragged smaller than a pixel by a handle", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  const edge = await page.locator('#ed-overlay .ed-handle[data-handle="se"]').boundingBox();
  await page.keyboard.down("Alt");
  await drag(page, { x: edge.x + edge.width / 2, y: edge.y + edge.height / 2 }, { x: -900, y: -900 });
  await expect(page.locator("#ed-sel-w")).toHaveText("1");
  await expect(page.locator("#ed-sel-h")).toHaveText("1");
  // At a pixel the box shows its corners and nothing else
  await expect(editor.handles).toHaveCount(4);
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0].rect).toMatchObject({ width: 1, height: 1 });
});

test("arrow keys nudge the selection, and a run of them is saved as one edit", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("Shift+ArrowDown");
  // The box shows it at once, before anything is saved
  await expect(page.locator("#ed-sel-x")).toHaveText("103");
  await expect(page.locator("#ed-sel-y")).toHaveText("90");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "move", layout: "normal", kind: "text", key: "title", dx: 3, dy: 10 }]);
  await expect(editor.undoButton).toBeEnabled();
});

test("typing in a field does not nudge the selection", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await editor.openSearch();
  await editor.searchInput.focus();
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(400);
  expect(applied(bridge)).toEqual([]);
});

test("each edit is followed by a fresh description and a new render", async ({ bridge, page }) => {
  let moved = 0;
  const { editor, point } = await ready(bridge, page, {
    describe: () => describeView(standardItems.map((it) => (it.key === "title" ? { ...it, delivered: at(100 + moved, 80, 600, 80), authored: at(100 + moved, 80, 600, 80) } : it))),
  });
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  const before = bridge.calledWith("Studio.Describe").length;
  bridge.reply("Studio.Apply", (req) => {
    moved = req.ops[0].dx;
    return { rev: 1, inverse: [{ ...req.ops[0], dx: -req.ops[0].dx }], valid: true, diagnostics: [] };
  });
  await page.keyboard.down("Alt");
  await drag(page, p, { x: 40, y: 0 });
  await page.mouse.up();
  await page.keyboard.up("Alt");

  await expect(page.locator("#ed-sel-x")).toHaveText("200");
  expect(bridge.calledWith("Studio.Describe").length).toBeGreaterThan(before);
  // The card is drawn again once the edits stop, and the new image replaces the old
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await bridge.job("job-2", [done(1)]);
  await expect(editor.preview).toHaveAttribute("src", /job-2/);
});

test("undo and redo take an edit back and make it again, by button and by key", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(150, 100);
  await page.keyboard.down("Alt");
  await drag(page, p, { x: 40, y: 0 });
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect(editor.undoButton).toBeEnabled();
  await expect(editor.redoButton).toBeDisabled();

  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toHaveLength(1);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "move", layout: "normal", kind: "text", key: "title", dx: -100 });
  await expect(editor.undoButton).toBeDisabled();
  await expect(editor.redoButton).toBeEnabled();

  await editor.redoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ op: "move", key: "title", dx: 100 });

  await page.keyboard.press("ControlOrMeta+z");
  await expect.poll(() => applied(bridge).length).toBe(4);
  expect(applied(bridge)[3].ops[0].dx).toBe(-100);
  await page.keyboard.press("ControlOrMeta+Shift+z");
  await expect.poll(() => applied(bridge).length).toBe(5);
  expect(applied(bridge)[4].ops[0].dx).toBe(100);
});

test("an edit the app refuses says why, and leaves nothing to undo", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  bridge.fail("Studio.Apply", "That change would leave the template unable to load");
  const p = point(150, 100);
  await page.keyboard.down("Alt");
  await drag(page, p, { x: 40, y: 0 });
  await page.mouse.up();
  await page.keyboard.up("Alt");

  await expect(page.locator("#toasts")).toContainText("unable to load");
  await expect(editor.undoButton).toBeDisabled();
  // The box goes back to where the app says the title is
  await expect(page.locator("#ed-sel-x")).toHaveText("100");
});

test("a layout that cannot be edited can be looked at and not changed", async ({ bridge, page }) => {
  const reason = "This layout extends another one, so its positions are not in its own file. Edit it in the JSON";
  const { editor, point } = await ready(bridge, page, { describe: () => describeView(standardItems, { editable: false, reason }) });
  await expect(page.locator("#ed-edit-note")).toHaveText(reason);
  const p = point(150, 100);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("title");
  await expect(editor.handles).toHaveCount(0);
  // The row's menu and grip are not there, and nothing can be copied, deleted or dragged
  await expect(editor.moreButton("title")).toBeDisabled();
  await expect(editor.layers.locator(".ed-layer-grip")).toHaveCount(0);

  await drag(page, point(150, 100), { x: 40, y: 0 });
  await page.mouse.up();
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(400);
  expect(applied(bridge)).toEqual([]);
});

test("a layer is moved up or down a place from its menu, and the ends offer only one way", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await editor.moreButton("background").click();
  await expect(editor.menuItem("Move down")).toHaveCount(0);
  await expect(editor.menuItem("Move up")).toBeVisible();
  await editor.menuItem("Move up").click();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "reorder", layout: "normal", kind: "layer", key: "background", to: 1 }]);

  await editor.moreButton("frame").click();
  await expect(editor.menuItem("Move up")).toHaveCount(0);
  await editor.menuItem("Move down").click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "reorder", key: "frame", to: 0 });

  // Text has no place in the stack, and the menu does not offer one
  await editor.moreButton("oracle").click();
  await expect(editor.menuItem("Duplicate")).toBeVisible();
  await expect(editor.menuItem("Move up")).toHaveCount(0);
  await page.keyboard.press("Escape");
});

test("alt and the arrow keys move a layer in the stack from the list", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await editor.row("background").locator(".ed-layer-select").focus();
  await page.keyboard.press("Alt+ArrowUp");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "reorder", layout: "normal", kind: "layer", key: "background", to: 1 }]);
  // At the bottom there is nowhere lower to go, and a text box is not in the stack
  await page.keyboard.press("Alt+ArrowDown");
  await editor.row("title").locator(".ed-layer-select").focus();
  await page.keyboard.press("Alt+ArrowUp");
  await page.waitForTimeout(300);
  expect(applied(bridge).length).toBe(1);
});

test("dragging a layer's grip moves it to where the line shows, and only layers have one", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await expect(editor.layers.locator(".ed-layer-grip")).toHaveCount(2);
  await expect(editor.grip("title")).toHaveCount(0);
  const grip = await editor.grip("background").boundingBox();
  const frame = await editor.row("frame").boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + 4, frame.y + 3, { steps: 6 });
  // The line is above the frame, and the row being dragged is dimmed
  await expect(editor.row("frame")).toHaveClass(/drop-before/);
  await expect(editor.row("background")).toHaveClass(/dragging/);
  await page.mouse.up();
  await expect(page.locator("#ed-layers .drop-before, #ed-layers .drop-after, #ed-layers .dragging")).toHaveCount(0);
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "reorder", layout: "normal", kind: "layer", key: "background", to: 1 }]);

  // Dropped below the last row, a layer goes to the bottom
  const top = await editor.grip("frame").boundingBox();
  const last = await editor.row("background").boundingBox();
  await page.mouse.move(top.x + top.width / 2, top.y + top.height / 2);
  await page.mouse.down();
  await page.mouse.move(top.x + 4, last.y + last.height + 20, { steps: 6 });
  await expect(editor.row("background")).toHaveClass(/drop-after/);
  await page.mouse.up();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ op: "reorder", key: "frame", to: 0 });
});

test("a layer dropped where it already is changes nothing", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  const grip = await editor.grip("frame").boundingBox();
  const own = await editor.row("frame").boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + 4, own.y + own.height / 2 + 2, { steps: 4 });
  await expect(page.locator("#ed-layers .drop-before, #ed-layers .drop-after")).toHaveCount(0);
  await page.mouse.up();
  await page.waitForTimeout(300);
  expect(applied(bridge)).toEqual([]);
});

test("the layer list moves the selection with the arrow keys", async ({ bridge, page }) => {
  const { editor } = await ready(bridge, page);
  await editor.layers.first().locator(".ed-layer-select").focus();
  await page.keyboard.press("ArrowDown");
  await expect(editor.selectedRow.locator(".ed-layer-name")).toHaveText("oracle");
  await page.keyboard.press("ArrowDown");
  await expect(editor.selectedRow.locator(".ed-layer-name")).toHaveText("type");
  await page.keyboard.press("ArrowUp");
  await expect(editor.selectedRow.locator(".ed-layer-name")).toHaveText("oracle");
  await expect(editor.layers.nth(1).locator(".ed-layer-select")).toBeFocused();
  // The arrows moved the selection and not the item
  expect(applied(bridge)).toEqual([]);
});

test("a selection that is no longer there is let go when the layout is described again", async ({ bridge, page }) => {
  let gone = false;
  const { editor } = await ready(bridge, page, {
    describe: () => describeView(gone ? standardItems.filter((it) => it.key !== "title") : standardItems),
  });
  await editor.layers.filter({ hasText: "title" }).click();
  await expect(editor.selectionName).toHaveText("title");
  gone = true;
  await editor.preset("Land").click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await bridge.job("job-2", [done(1)]);
  await expect(editor.selection).toBeHidden();
  await expect(editor.layers).toHaveCount(6);
});

test("opening another template clears the selection and the undo history", async ({ bridge, page }) => {
  const other = draftSummary("other", "Other");
  bridge.reply("Studio.List", [mine, other]);
  bridge.reply("Studio.Open", (id) => draftView(id === "mine" ? mine : other));
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await editor.sizeCanvas(400);
  await editor.layers.filter({ hasText: "oracle" }).click();
  await page.keyboard.press("ArrowRight");
  await expect.poll(() => applied(bridge).length).toBe(1);
  await expect(editor.undoButton).toBeEnabled();

  await editor.picker.selectOption("other");
  await expect(editor.undoButton).toBeDisabled();
  await expect(editor.selection).toBeHidden();
});

test("a text box shown by where its text is can be moved and has no handles to resize", async ({ bridge, page }) => {
  const { editor, point } = await ready(bridge, page);
  const p = point(280, 810);
  await page.mouse.click(p.x, p.y);
  await expect(editor.selectionName).toHaveText("type");
  await expect(page.locator("#ed-sel-y")).toHaveText("790");
  await expect(editor.selectionBox).toHaveCount(1);
  await expect(editor.handles).toHaveCount(0);

  await page.keyboard.down("Alt");
  await drag(page, p, { x: 0, y: 12 });
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0]).toEqual({ op: "move", layout: "normal", kind: "text", key: "type", dx: 0, dy: 30 });
});

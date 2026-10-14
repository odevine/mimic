import { test, expect } from "../../bridge-fixtures.mjs";
import { EditorPage } from "../../pages/EditorPage.mjs";
import { done, draftSummary, draftView, describeView, standardItems } from "../../bridge/cards.mjs";

const mine = draftSummary("mine", "Mine");

// elementOf finds what an edit's fields are in, in the files the bridge keeps
function elementOf(src, op) {
  switch (op.kind) {
    case "template":
      return src.template;
    case "part":
      return src.template.parts[op.key];
    case "canvas":
      return src.layout.canvas;
    case "text":
      return src.layout.textBoxes[op.key];
    case "art":
      return src.layout.art[Number(op.key)];
    case "symbol":
      return src.layout.symbols[op.key];
    case "layer":
      return src.layout.stack.find((e) => (e.id || e.part) === op.key);
  }
  throw new Error(`no ${op.kind}`);
}

// writes makes the bridge's Apply do what the app does to the files, so the page reads
// its own change back, and gives the operation that undoes each
function writes(bridge, { valid = true, diagnostics = [] } = {}) {
  bridge.reply("Studio.Apply", (req) => {
    const inverse = [];
    for (const op of req.ops) {
      if (op.op === "replace") {
        const name = op.file;
        const was = bridge.source.files().files.find((f) => f.name === name).content;
        bridge.source.extra[name] = op.content;
        inverse.unshift({ op: "replace", file: name, content: was });
        continue;
      }
      const node = elementOf(bridge.source, op);
      let at = node;
      for (const k of op.path.slice(0, -1)) at = at[k] ??= {};
      const last = op.path.at(-1);
      const had = last in at;
      const old = at[last];
      if (op.op === "set") at[last] = op.value;
      else delete at[last];
      inverse.unshift(had ? { ...op, op: "set", value: old } : { ...op, op: "unset", value: undefined });
    }
    return { rev: 1, inverse, valid, diagnostics };
  });
}

const applied = (bridge) => bridge.calledWith("Studio.Apply").map(([req]) => req);

// The editor on one template with its card drawn, and a text box picked
async function ready(bridge, page, { meta = {}, layout, template } = {}) {
  if (layout) layout(bridge.source.layout);
  if (template) template(bridge.source.template);
  bridge.reply("Studio.List", [mine]);
  bridge.reply("Studio.Open", draftView(mine, { meta }));
  writes(bridge);
  await bridge.boot();
  const editor = new EditorPage(page);
  await editor.show();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(1);
  await bridge.job("job-1", [done(1)]);
  await expect(editor.layers).toHaveCount(7);
  return editor;
}

const pick = async (editor, name) => {
  await editor.layers.filter({ hasText: name }).first().click();
  await expect(editor.selectionName).toHaveText(name === "Art window" ? "Art window" : name);
};

test("a text box's levers show what its file holds", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await expect(editor.lever("lever-x")).toHaveValue("100");
  await expect(editor.lever("lever-width")).toHaveValue("600");
  await expect(editor.lever("lever-fontSize")).toHaveValue("60");
  await expect(editor.lever("lever-color")).toHaveValue("#000000");
  await expect(editor.lever("lever-align")).toHaveValue("left");
  await expect(editor.lever("lever-font")).toHaveValue("title");
  await expect(editor.lever("lever-vAlign")).toHaveValue("top");
  await expect(editor.lever("lever-lineSpacing")).toHaveValue("");
  // The choices are the names the app offers, with the default first
  await expect(editor.lever("lever-draws").locator("option").first()).toHaveText("Default (the box's own name)");
  await expect(editor.lever("lever-draws").locator("option")).toHaveCount(12);
  await expect(editor.lever("lever-vAlign").locator("option")).toHaveText(["Default (top)", "top", "center", "bottom", "baseline"]);
});

test("typing a number changes the field once the typing stops, and a run is one edit", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await editor.lever("lever-fontSize").fill("72");
  await page.waitForTimeout(500);
  await editor.lever("lever-fontSize").fill("73");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[0]).toEqual({ id: "mine", ops: [{ op: "set", layout: "normal", kind: "text", key: "title", path: ["text", "fontSize"], value: 72 }] });
  await expect(editor.undoButton).toBeEnabled();
  // Undo takes both back at once, to the size before the first
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ op: "set", path: ["text", "fontSize"], value: 60 });
  await expect(editor.undoButton).toBeDisabled();
  await expect(editor.lever("lever-fontSize")).toHaveValue("60");
});

test("a value that is not valid says why and changes nothing", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await editor.lever("lever-fontSize").fill("big");
  await editor.lever("lever-fontSize").blur();
  await expect(editor.leverError("lever-fontSize")).toHaveText("Use a number");
  await expect(editor.lever("lever-fontSize")).toHaveAttribute("aria-invalid", "true");
  await editor.lever("lever-width").fill("0");
  await editor.lever("lever-width").blur();
  await expect(editor.leverError("lever-width")).toHaveText("Use 1 or more");
  await editor.lever("lever-color").fill("red");
  await editor.lever("lever-color").blur();
  await expect(editor.leverError("lever-color")).toHaveText("Use a color like #112233");
  await editor.lever("lever-x").fill("");
  await editor.lever("lever-x").blur();
  await expect(editor.leverError("lever-x")).toHaveText("This is needed");
  await page.waitForTimeout(500);
  expect(applied(bridge)).toEqual([]);

  // Typing a good value clears the complaint
  await editor.lever("lever-fontSize").fill("64");
  await editor.lever("lever-fontSize").blur();
  await expect(editor.lever("lever-fontSize")).not.toHaveAttribute("aria-invalid", "true");
  await expect.poll(() => applied(bridge).length).toBe(1);
});

test("clearing an optional field takes the setting out", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await editor.lever("lever-color").fill("");
  await editor.lever("lever-color").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "unset", layout: "normal", kind: "text", key: "title", path: ["text", "color"] }]);
  await expect(editor.lever("lever-color")).toHaveValue("");
});

test("a choice sets at once and the color is kept in capitals", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await editor.lever("lever-align").selectOption("center");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0]).toMatchObject({ op: "set", path: ["text", "align"], value: "center" });
  await editor.lever("lever-color").fill("#ab12cd");
  await editor.lever("lever-color").blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ path: ["text", "color"], value: "#AB12CD" });
  await editor.lever("lever-font").selectOption("body");
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ path: ["text", "font"], value: "body" });
});

test("the shadow is turned on with its usual numbers, edited, and turned off", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await expect(editor.lever("lever-shadowDistance")).toBeHidden();
  await page.locator("#lever-shadow-on").check();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", layout: "normal", kind: "text", key: "title", path: ["text", "shadow"], value: { distance: 0.08, angle: 25 } }]);
  await expect(editor.lever("lever-shadowDistance")).toBeVisible();
  await expect(editor.lever("lever-shadowDistance")).toHaveValue("0.08");

  await editor.lever("lever-shadowDistance").fill("0.12");
  await editor.lever("lever-shadowDistance").blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ path: ["text", "shadow", "distance"], value: 0.12 });

  await page.locator("#lever-shadow-on").uncheck();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ op: "unset", path: ["text", "shadow"] });
  await expect(editor.lever("lever-shadowDistance")).toBeHidden();
});

test("a layer's levers are its opacity, blend, condition, position and the part's slot", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "frame");
  // With no position of its own a layer is where its part puts it
  await expect(editor.lever("lever-x")).toHaveValue("10");
  await expect(editor.lever("lever-y")).toHaveValue("20");
  await expect(editor.lever("lever-condition")).toHaveValue("nonland");
  await expect(editor.lever("lever-slot")).toHaveValue("background");

  await editor.lever("lever-x").fill("30");
  await editor.lever("lever-x").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", layout: "normal", kind: "layer", key: "frame", path: ["at"], value: { x: 30, y: 20 } }]);
  await expect(editor.lever("lever-x")).toHaveValue("30");

  await editor.lever("lever-opacity").fill("0.5");
  await editor.lever("lever-opacity").blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ path: ["opacity"], value: 0.5 });
  await editor.lever("lever-mode").selectOption("multiply");
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ path: ["mode"], value: "multiply" });
  await editor.lever("lever-slot").selectOption("pinlines");
  await expect.poll(() => applied(bridge).length).toBe(4);
  expect(applied(bridge)[3].ops).toEqual([{ op: "set", layout: "normal", kind: "part", key: "frame", path: ["slot"], value: "pinlines" }]);
});

test("an art window's levers include which layer it sits above", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "Art window");
  await expect(editor.lever("lever-after").locator("option")).toHaveText(["background", "frame"]);
  await expect(editor.lever("lever-after")).toHaveValue("frame");
  await editor.lever("lever-after").selectOption("background");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0]).toMatchObject({ kind: "art", key: "0", path: ["after"], value: "background" });
  await editor.lever("lever-rotate").selectOption("90");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ path: ["rotate"], value: 90 });
  // A turn of nothing is no setting
  await editor.lever("lever-rotate").selectOption("0");
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ op: "unset", path: ["rotate"] });
});

test("the layout's canvas is edited in its own section, and the details are read again", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  await expect(editor.lever("canvas-width")).toHaveValue("1000");
  await expect(editor.lever("canvas-dpi")).toHaveValue("800");
  const opens = bridge.calledWith("Studio.Open").length;
  await editor.lever("canvas-width").fill("1100");
  await editor.lever("canvas-width").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  // The proportion is kept to begin with, so the height follows the width
  expect(applied(bridge)[0].ops).toEqual([
    { op: "set", layout: "normal", kind: "canvas", key: "", path: ["width"], value: 1100 },
    { op: "set", layout: "normal", kind: "canvas", key: "", path: ["height"], value: 1540 },
  ]);
  await expect.poll(() => bridge.calledWith("Studio.Open").length).toBeGreaterThan(opens);
});

test("the template's name and details are edited, and a forked license is kept", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { template: (t) => ((t.license = "CC BY 4.0"), (t.author = { name: "Me" })) });
  await editor.openSection("about");
  await expect(editor.lever("template-name")).toHaveValue("Mine");
  await expect(editor.lever("template-authorName")).toHaveValue("Me");
  await expect(editor.lever("template-license")).toHaveValue("CC BY 4.0");
  await expect(editor.lever("template-license")).not.toHaveAttribute("readonly", "");
  await editor.lever("template-name").fill("Renamed");
  await editor.lever("template-name").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", layout: "", kind: "template", key: "", path: ["name"], value: "Renamed" }]);
  await editor.lever("template-name").fill("");
  await editor.lever("template-name").blur();
  await expect(editor.leverError("template-name")).toHaveText("This is needed");
});

test("the license of a copy cannot be changed", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { meta: { forkedFrom: "mimic.m15 0.1.0", lockedCredits: 1, licenseLocked: true }, template: (t) => (t.license = "CC BY 4.0") });
  await editor.openSection("about");
  await expect(editor.lever("template-license")).toHaveAttribute("readonly", "");
  await expect(editor.lever("template-license")).toHaveValue("CC BY 4.0");
});

test("credits are added, edited and removed, except the ones kept from a copied template", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { meta: { forkedFrom: "mimic.m15 0.1.0", lockedCredits: 1, licenseLocked: false } });
  await editor.openSection("about");
  const credits = page.locator("#ed-credits .ed-credit");
  await expect(credits).toHaveCount(2);
  // The first came with the template that was copied
  await expect(credits.nth(0).locator("input").first()).toHaveAttribute("readonly", "");
  await expect(credits.nth(0)).toContainText("Kept from the template this was copied from");
  await expect(credits.nth(0).getByRole("button")).toHaveCount(0);
  await expect(credits.nth(1).getByRole("button", { name: "Remove credit 2" })).toBeVisible();

  await credits.nth(1).locator("input").nth(1).fill("https://example.com/me");
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops[0]).toMatchObject({ kind: "template", path: ["credits"] });
  expect(applied(bridge)[0].ops[0].value).toEqual([
    { name: "Original Artist", url: "https://example.com/art", note: "art" },
    { name: "Mine too", url: "https://example.com/me" },
  ]);

  await page.locator("#ed-credit-add").click();
  await expect.poll(() => applied(bridge).length).toBe(2);
  await expect(credits).toHaveCount(3);
  await credits.nth(2).getByRole("button", { name: "Remove credit 3" }).click();
  await expect.poll(() => applied(bridge).length).toBe(3);
  await expect(credits).toHaveCount(2);
});

test("a layout that cannot be edited shows its levers and refuses them", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  bridge.reply("Studio.Describe", () => ({
    layout: "normal",
    authored: { width: 1000, height: 1400 },
    delivered: { width: 1000, height: 1400 },
    items: [{ kind: "text", key: "title", drawn: true, authored: { x: 100, y: 80, width: 600, height: 80 }, delivered: { x: 100, y: 80, width: 600, height: 80 } }],
    editable: false,
    reason: "This layout extends another one, so its positions are not in its own file. Edit it in the JSON",
    rev: 0,
  }));
  await editor.preset("Land").click();
  await expect.poll(() => bridge.calledWith("Studio.Render").length).toBe(2);
  await bridge.job("job-2", [done(1)]);
  await expect(editor.layers).toHaveCount(1);
  await editor.layers.first().click();
  await expect(editor.lever("lever-fontSize")).toBeDisabled();
  await expect(editor.lever("lever-align")).toBeDisabled();
  await expect(editor.lever("canvas-width")).toBeDisabled();
  // The template's own fields are in template.json and stay open
  await editor.openSection("about");
  await expect(editor.lever("template-name")).toBeEnabled();
});

test("an edit the app refuses says why and the field goes back to what the file has", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  bridge.fail("Studio.Apply", "That is not allowed: layouts.normal.textBoxes.title.text.fontSize: must be above 0");
  await editor.lever("lever-fontSize").fill("5");
  await editor.lever("lever-fontSize").blur();
  await expect(page.locator("#toasts")).toContainText("not allowed");
  await expect(editor.lever("lever-fontSize")).toHaveValue("60");
  await expect(editor.undoButton).toBeDisabled();
});

test("a lever being typed in is left alone when the files are read again", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await editor.lever("lever-color").fill("#12");
  // Another edit elsewhere reads the files again while this field is in use
  await editor.lever("lever-align").selectOption("right");
  await expect.poll(() => applied(bridge).length).toBe(1);
  await expect(editor.lever("lever-align")).toHaveValue("right");
  await expect(editor.lever("lever-color")).toHaveValue("#12");
});

test("the canvas has a size and a rotation of their own", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  await expect(page.locator("#ed-canvas-levers .ed-lever-title")).toHaveText(["Size", "Rotation"]);
  await expect(page.locator("#ed-canvas-levers .ed-lever-group").first().locator("#canvas-width, #canvas-height, #canvas-dpi")).toHaveCount(3);
  await expect(editor.lever("canvas-rotate")).toBeVisible();
  // The rotation is not among the size's fields
  await expect(page.locator("#ed-canvas-levers .ed-ratio #canvas-rotate")).toHaveCount(0);
});

test("a bracket joins the width and height with a padlock that is closed to begin with", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  const lock = page.locator("#ed-canvas-levers [data-ratio='lock']");
  await expect(page.locator("#ed-canvas-levers .ed-ratio")).toHaveCount(1);
  await expect(page.locator("#ed-canvas-levers .ed-ratio-bracket")).toBeVisible();
  // The proportion is kept until it is let go of, and the padlock is the layer's: closed
  // and in the full text color while it is kept
  await expect(lock).toHaveAttribute("aria-pressed", "true");
  await expect(lock.locator("use")).toHaveAttribute("href", "icons.svg#i-lock");
  const full = await lock.evaluate((el) => getComputedStyle(el).color);

  // The bracket runs from the middle of the width to the middle of the height, and the
  // padlock is on it
  const width = await editor.lever("canvas-width").boundingBox();
  const height = await editor.lever("canvas-height").boundingBox();
  const bracket = await page.locator("#ed-canvas-levers .ed-ratio-bracket").boundingBox();
  const pad = await lock.boundingBox();
  expect(Math.abs(bracket.y - (width.y + width.height / 2))).toBeLessThan(3);
  expect(Math.abs(bracket.y + bracket.height - (height.y + height.height / 2))).toBeLessThan(3);
  expect(Math.abs(pad.y + pad.height / 2 - (bracket.y + bracket.height / 2))).toBeLessThan(2);
  expect(bracket.x).toBeGreaterThanOrEqual(width.x + width.width - 2);

  await lock.click();
  await expect(lock).toHaveAttribute("aria-pressed", "false");
  await expect(lock.locator("use")).toHaveAttribute("href", "icons.svg#i-lock-open");
  // Open, it is faint, once the pointer is off it
  await page.mouse.move(0, 0);
  await lock.evaluate((el) => el.blur());
  expect(await lock.evaluate((el) => getComputedStyle(el).color)).not.toBe(full);
  await lock.click();
  await expect(lock).toHaveAttribute("aria-pressed", "true");
});

test("with the proportion kept, changing the width changes the height with it in one edit", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  await expect(editor.lever("canvas-width")).toHaveValue("1000");
  await expect(editor.lever("canvas-height")).toHaveValue("1400");
  await editor.lever("canvas-width").fill("500");
  await editor.lever("canvas-width").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([
    { op: "set", layout: "normal", kind: "canvas", key: "", path: ["width"], value: 500 },
    { op: "set", layout: "normal", kind: "canvas", key: "", path: ["height"], value: 700 },
  ]);
  // The other way round, from the shape it had to begin with and not the size it has now
  await editor.lever("canvas-height").fill("1400");
  await editor.lever("canvas-height").blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops.map((o) => [o.path[0], o.value])).toEqual([
    ["width", 1000],
    ["height", 1400],
  ]);
});

test("with the proportion let go of, the width and height change on their own", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  await page.locator("#ed-canvas-levers [data-ratio='lock']").click();
  await editor.lever("canvas-width").fill("500");
  await editor.lever("canvas-width").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops.map((o) => o.path[0])).toEqual(["width"]);
  // Closed again, the shape kept is the one the canvas has then, 500 by 1400, which is no
  // longer 5 to 7
  await page.locator("#ed-canvas-levers [data-ratio='lock']").click();
  await editor.lever("canvas-height").fill("1000");
  await editor.lever("canvas-height").blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops.map((o) => [o.path[0], o.value])).toEqual([
    ["width", 357],
    ["height", 1000],
  ]);
  // And it stays that shape until it is let go of again
  await editor.lever("canvas-height").fill("500");
  await editor.lever("canvas-height").blur();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops.map((o) => [o.path[0], o.value])).toEqual([
    ["width", 179],
    ["height", 500],
  ]);
});

test("the resolution is never part of the proportion", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  await editor.lever("canvas-dpi").fill("600");
  await editor.lever("canvas-dpi").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops.map((o) => o.path[0])).toEqual(["dpi"]);
});

test("the padlock of the canvas is off for a layout that cannot be edited", async ({ bridge, page }) => {
  bridge.reply("Studio.Describe", () => describeView(standardItems, { editable: false, reason: "This layout extends another one" }));
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  await expect(page.locator("#ed-canvas-levers [data-ratio='lock']")).toBeDisabled();
});

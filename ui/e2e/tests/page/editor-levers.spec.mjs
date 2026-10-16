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

test("a text box's levers show its file, and a choice, a color, a cleared field and a run of typing each make one edit", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await expect(editor.lever("lever-x")).toHaveValue("100");
  await expect(editor.lever("lever-fontSize")).toHaveValue("60");
  await expect(editor.lever("lever-color")).toHaveValue("#000000");
  await expect(editor.lever("lever-vAlign")).toHaveValue("top");
  await expect(editor.lever("lever-lineSpacing")).toHaveValue("");
  await expect(editor.lever("lever-vAlign").locator("option")).toHaveText(["Default (top)", "top", "center", "bottom", "baseline", "cap-top", "cap-center"]);

  // A run of typing is one edit, which undo takes back to the size before it
  await editor.lever("lever-fontSize").fill("72");
  await page.waitForTimeout(500);
  await editor.lever("lever-fontSize").fill("73");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[0]).toEqual({ id: "mine", ops: [{ op: "set", layout: "normal", kind: "text", key: "title", path: ["text", "fontSize"], value: 72 }] });
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0]).toMatchObject({ op: "set", path: ["text", "fontSize"], value: 60 });
  await expect(editor.undoButton).toBeDisabled();
  await expect(editor.lever("lever-fontSize")).toHaveValue("60");

  await editor.lever("lever-align").selectOption("center");
  await expect.poll(() => applied(bridge).length).toBe(4);
  expect(applied(bridge)[3].ops[0]).toMatchObject({ op: "set", path: ["text", "align"], value: "center" });
  await editor.lever("lever-color").fill("#ab12cd");
  await editor.lever("lever-color").blur();
  await expect.poll(() => applied(bridge).length).toBe(5);
  expect(applied(bridge)[4].ops[0]).toMatchObject({ path: ["text", "color"], value: "#AB12CD" });
  await editor.lever("lever-color").fill("");
  await editor.lever("lever-color").blur();
  await expect.poll(() => applied(bridge).length).toBe(6);
  expect(applied(bridge)[5].ops).toEqual([{ op: "unset", layout: "normal", kind: "text", key: "title", path: ["text", "color"] }]);

  // The shadow is turned on with its usual numbers, edited, and turned off
  await page.locator("#lever-shadow-on").check();
  await expect.poll(() => applied(bridge).length).toBe(7);
  expect(applied(bridge)[6].ops[0]).toMatchObject({ path: ["text", "shadow"], value: { distance: 0.08, angle: 25 } });
  await editor.lever("lever-shadowDistance").fill("0.12");
  await editor.lever("lever-shadowDistance").blur();
  await expect.poll(() => applied(bridge).length).toBe(8);
  expect(applied(bridge)[7].ops[0]).toMatchObject({ path: ["text", "shadow", "distance"], value: 0.12 });
  await page.locator("#lever-shadow-on").uncheck();
  await expect.poll(() => applied(bridge).length).toBe(9);
  expect(applied(bridge)[8].ops[0]).toMatchObject({ op: "unset", path: ["text", "shadow"] });
});

test("a value that is not valid or that the app refuses changes nothing, and a field being typed in is left alone", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  for (const [id, value, error] of [
    ["lever-fontSize", "big", "Use a number"],
    ["lever-width", "0", "Use 1 or more"],
    ["lever-color", "red", "Use a color like #112233"],
    ["lever-x", "", "This is needed"],
  ]) {
    await editor.lever(id).fill(value);
    await editor.lever(id).blur();
    await expect(editor.leverError(id)).toHaveText(error);
  }
  await expect(editor.lever("lever-fontSize")).toHaveAttribute("aria-invalid", "true");
  await page.waitForTimeout(500);
  expect(applied(bridge)).toEqual([]);
  await editor.lever("lever-x").fill("100");
  await editor.lever("lever-width").fill("600");

  bridge.fail("Studio.Apply", "That is not allowed: layouts.normal.textBoxes.title.text.fontSize: must be above 0");
  await editor.lever("lever-fontSize").fill("5");
  await editor.lever("lever-fontSize").blur();
  await expect(page.locator("#toasts")).toContainText("not allowed");
  await expect(editor.lever("lever-fontSize")).toHaveValue("60");
  await expect(editor.undoButton).toBeDisabled();

  // Another edit reads the files again while the color is being typed in
  writes(bridge);
  await editor.lever("lever-color").fill("#12");
  await editor.lever("lever-align").selectOption("right");
  await expect(editor.lever("lever-align")).toHaveValue("right");
  await expect(editor.lever("lever-color")).toHaveValue("#12");
});

test("a layer's and an art window's levers write their own fields and the part's slot", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await pick(editor, "frame");
  // With no position of its own a layer is where its part puts it
  await expect(editor.lever("lever-x")).toHaveValue("10");
  await expect(editor.lever("lever-condition")).toHaveValue("nonland");
  await editor.lever("lever-x").fill("30");
  await editor.lever("lever-x").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", layout: "normal", kind: "layer", key: "frame", path: ["at"], value: { x: 30, y: 20 } }]);
  await editor.lever("lever-mode").selectOption("multiply");
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops[0]).toMatchObject({ path: ["mode"], value: "multiply" });
  await editor.lever("lever-slot").selectOption("pinlines");
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops).toEqual([{ op: "set", layout: "normal", kind: "part", key: "frame", path: ["slot"], value: "pinlines" }]);

  await pick(editor, "Art window");
  await expect(editor.lever("lever-after").locator("option")).toHaveText(["background", "frame"]);
  await editor.lever("lever-after").selectOption("background");
  await expect.poll(() => applied(bridge).length).toBe(4);
  expect(applied(bridge)[3].ops[0]).toMatchObject({ kind: "art", key: "0", path: ["after"], value: "background" });
  await editor.lever("lever-rotate").selectOption("90");
  await expect.poll(() => applied(bridge).length).toBe(5);
  // A turn of nothing is no setting
  await editor.lever("lever-rotate").selectOption("0");
  await expect.poll(() => applied(bridge).length).toBe(6);
  expect(applied(bridge)[5].ops[0]).toMatchObject({ op: "unset", path: ["rotate"] });
});

test("the canvas keeps its proportion until it is let go of, and the resolution is never part of it", async ({ bridge, page }) => {
  const editor = await ready(bridge, page);
  await editor.openCanvas();
  const lock = page.locator("#ed-canvas-levers [data-ratio='lock']");
  await expect(lock).toHaveAttribute("aria-pressed", "true");
  const opens = bridge.calledWith("Studio.Open").length;
  await editor.lever("canvas-width").fill("500");
  await editor.lever("canvas-width").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([
    { op: "set", layout: "normal", kind: "canvas", key: "", path: ["width"], value: 500 },
    { op: "set", layout: "normal", kind: "canvas", key: "", path: ["height"], value: 700 },
  ]);
  // The template's details are read again after a canvas edit
  await expect.poll(() => bridge.calledWith("Studio.Open").length).toBeGreaterThan(opens);

  await lock.click();
  await expect(lock).toHaveAttribute("aria-pressed", "false");
  await editor.lever("canvas-width").fill("1000");
  await editor.lever("canvas-width").blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops.map((o) => o.path[0])).toEqual(["width"]);
  // Closed again, the shape kept is the one the canvas has then, 1000 by 700
  await lock.click();
  await editor.lever("canvas-height").fill("1400");
  await editor.lever("canvas-height").blur();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops.map((o) => [o.path[0], o.value])).toEqual([
    ["width", 2000],
    ["height", 1400],
  ]);

  await editor.lever("canvas-dpi").fill("600");
  await editor.lever("canvas-dpi").blur();
  await expect.poll(() => applied(bridge).length).toBe(4);
  expect(applied(bridge)[3].ops.map((o) => o.path[0])).toEqual(["dpi"]);
});

test("the template's name and owner are edited and checked, and the owner can come from the author", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { template: (t) => (t.author = { name: "Jané Q. Doe" }) });
  await editor.openSection("about");
  await expect(editor.lever("template-name")).toHaveValue("Mine");
  await editor.lever("template-name").fill("Renamed");
  await editor.lever("template-name").blur();
  await expect.poll(() => applied(bridge).length).toBe(1);
  expect(applied(bridge)[0].ops).toEqual([{ op: "set", layout: "", kind: "template", key: "", path: ["name"], value: "Renamed" }]);
  await editor.lever("template-name").fill("");
  await editor.lever("template-name").blur();
  await expect(editor.leverError("template-name")).toHaveText("This is needed");
  await editor.lever("template-name").fill("Renamed");

  // The owner is the first half of the id, and a name the engine would not read is refused
  await expect(editor.lever("template-owner")).toHaveValue("user");
  for (const bad of ["Jane", "jane doe", "jane.doe", "-jane"]) {
    await editor.lever("template-owner").fill(bad);
    await editor.lever("template-owner").blur();
    await expect(editor.leverError("template-owner")).toContainText("lowercase letters, digits and hyphens");
  }
  await editor.lever("template-owner").fill("a".repeat(41));
  await editor.lever("template-owner").blur();
  await expect(editor.leverError("template-owner")).toContainText("40 characters or fewer");
  expect(applied(bridge)).toHaveLength(1);

  await editor.lever("template-owner").fill("jane");
  await editor.lever("template-owner").blur();
  await expect.poll(() => applied(bridge).length).toBe(2);
  expect(applied(bridge)[1].ops).toEqual([{ op: "set", layout: "", kind: "template", key: "", path: ["id"], value: "jane.mine" }]);
  await editor.undoButton.click();
  await expect.poll(() => applied(bridge).length).toBe(3);
  expect(applied(bridge)[2].ops[0].value).toBe("user.mine");

  const suggest = page.getByRole("button", { name: "Use the author's name" });
  await suggest.click();
  await expect.poll(() => applied(bridge).length).toBe(4);
  expect(applied(bridge)[3].ops[0].value).toBe("jane-q-doe.mine");
  await expect(suggest).toBeDisabled();
});

test("a copy keeps its original's license and first credit, and the other credits are added, edited and removed", async ({ bridge, page }) => {
  const editor = await ready(bridge, page, { meta: { forkedFrom: "mimic.m15 0.1.0", lockedCredits: 1, licenseLocked: true }, template: (t) => (t.license = "CC BY 4.0") });
  await editor.openSection("about");
  await expect(editor.lever("template-license")).toHaveAttribute("readonly", "");
  const credits = page.locator("#ed-credits .ed-credit");
  await expect(credits).toHaveCount(2);
  await expect(credits.nth(0).locator("input").first()).toHaveAttribute("readonly", "");
  await expect(credits.nth(0)).toContainText("Kept from the template this was copied from");
  await expect(credits.nth(0).getByRole("button")).toHaveCount(0);

  await credits.nth(1).locator("input").nth(1).fill("https://example.com/me");
  await expect.poll(() => applied(bridge).length).toBe(1);
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

test("a layout that cannot be edited shows its levers and refuses them, and the template's own stay open", async ({ bridge, page }) => {
  bridge.reply("Studio.Describe", () => describeView(standardItems, { editable: false, reason: "This layout extends another one" }));
  const editor = await ready(bridge, page);
  await pick(editor, "title");
  await expect(editor.lever("lever-fontSize")).toBeDisabled();
  await expect(editor.lever("lever-align")).toBeDisabled();
  await editor.openCanvas();
  await expect(editor.lever("canvas-width")).toBeDisabled();
  await expect(page.locator("#ed-canvas-levers [data-ratio='lock']")).toBeDisabled();
  await editor.openSection("about");
  await expect(editor.lever("template-name")).toBeEnabled();
});

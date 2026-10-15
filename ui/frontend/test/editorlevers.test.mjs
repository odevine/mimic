import test from "node:test";
import assert from "node:assert/strict";

import { fieldGroups, readPath, displayValue, parseField, editOps, shadowOps, editLabel, layerAt, creditEdit, creditAdd, creditRemove, keepRatio, sizeOps } from "../js/editorlevers.js";

const vocab = {
  draws: ["title", "oracle", "pt"],
  conditions: ["land", "creature"],
  fontRoles: ["title", "body", "mana"],
  blendModes: ["normal", "multiply"],
  slots: ["background", "pinlines"],
};
const ctx = { vocab, templateRoles: ["saga", "title"], stackIds: ["background", "frame"] };
const find = (groups, id) => groups.flatMap((g) => g.fields).find((f) => f.id === id);

test("a text box offers its place, content, text, flow and shadow", () => {
  const groups = fieldGroups("text", ctx);
  assert.deepEqual(groups.map((g) => g.title), ["Place", "Content", "Text", "Flow", "Shadow"]);
  assert.deepEqual(groups[0].fields.map((f) => f.path.join(".")), ["rect.x", "rect.y", "rect.width", "rect.height"]);
  assert.equal(groups[4].shadow, true);
  assert.deepEqual(find(groups, "draws").options.map((o) => o.value), ["title", "oracle", "pt"]);
});

test("the font roles are the standard ones and the template's own, once each", () => {
  const roles = find(fieldGroups("text", ctx), "font").options.map((o) => o.value);
  assert.deepEqual(roles, ["title", "body", "mana", "saga"]);
});

test("every kind of item has fields, and an unknown kind has none", () => {
  for (const kind of ["text", "layer", "art", "symbol", "canvas", "template"]) assert.ok(fieldGroups(kind, ctx).length > 0, kind);
  assert.deepEqual(fieldGroups("mana", ctx), []);
});

test("a field's path is where the layout file keeps it", () => {
  assert.deepEqual(find(fieldGroups("text", ctx), "fontSize").path, ["text", "fontSize"]);
  assert.deepEqual(find(fieldGroups("art", ctx), "after").options.map((o) => o.value), ["background", "frame"]);
  assert.deepEqual(find(fieldGroups("symbol", ctx), "scale").path, ["scale"]);
  assert.deepEqual(find(fieldGroups("layer", ctx), "slot").on, "part");
  assert.deepEqual(find(fieldGroups("template", ctx), "authorName").path, ["author", "name"]);
});

test("a value is read from a parsed file by its keys", () => {
  const doc = { text: { fontSize: 12, shadow: { distance: 0.1 } }, n: null };
  assert.equal(readPath(doc, ["text", "fontSize"]), 12);
  assert.equal(readPath(doc, ["text", "shadow", "distance"]), 0.1);
  assert.equal(readPath(doc, ["text", "color"]), undefined);
  assert.equal(readPath(doc, ["n", "x"]), undefined);
  assert.equal(readPath(doc, ["missing", "x"]), undefined);
  assert.equal(displayValue({}, 12), "12");
  assert.equal(displayValue({}, undefined), "");
  assert.equal(displayValue({}, null), "");
});

test("whole numbers are read as such, within their range", () => {
  const f = find(fieldGroups("text", ctx), "width");
  assert.deepEqual(parseField(f, " 40 "), { ok: true, value: 40 });
  assert.deepEqual(parseField(f, "-3").error, "Use 1 or more");
  assert.equal(parseField(f, "1.5").error, "Use a whole number");
  assert.equal(parseField(f, "abc").error, "Use a whole number");
  assert.equal(parseField(f, "").error, "This is needed", "a size is needed");
  const padding = find(fieldGroups("text", ctx), "paddingX");
  assert.deepEqual(parseField(padding, ""), { ok: true, unset: true }, "an empty optional field takes the setting out");
  assert.deepEqual(parseField(padding, "0"), { ok: true, value: 0 });
});

test("numbers with a fraction are read, and bounded where they have bounds", () => {
  const opacity = find(fieldGroups("layer", ctx), "opacity");
  assert.deepEqual(parseField(opacity, "0.5"), { ok: true, value: 0.5 });
  assert.deepEqual(parseField(opacity, ".25"), { ok: true, value: 0.25 });
  assert.equal(parseField(opacity, "1.5").error, "Use 1 or less");
  assert.equal(parseField(opacity, "-0.1").error, "Use 0 or more");
  assert.equal(parseField(opacity, "1e3").error, "Use a number");
  const tracking = find(fieldGroups("text", ctx), "tracking");
  assert.deepEqual(parseField(tracking, "-12.5"), { ok: true, value: -12.5 });
});

test("a color is six hex digits, kept in capitals", () => {
  const color = find(fieldGroups("text", ctx), "color");
  assert.deepEqual(parseField(color, "#ab12cd"), { ok: true, value: "#AB12CD" });
  for (const bad of ["red", "#abc", "ab12cd", "#12345g"]) assert.equal(parseField(color, bad).error, "Use a color like #112233", bad);
  assert.deepEqual(parseField(color, ""), { ok: true, unset: true });
});

test("a choice must be one of the options, and a turn of nothing is no setting", () => {
  const align = find(fieldGroups("text", ctx), "align");
  assert.deepEqual(parseField(align, "center"), { ok: true, value: "center" });
  assert.equal(parseField(align, "diagonal").error, "Pick one of the choices");
  assert.deepEqual(parseField(align, ""), { ok: true, unset: true });
  const turn = find(fieldGroups("art", ctx), "rotate");
  assert.deepEqual(parseField(turn, "90"), { ok: true, value: 90 });
  assert.deepEqual(parseField(turn, "0"), { ok: true, unset: true });
  const after = find(fieldGroups("art", ctx), "after");
  assert.equal(parseField(after, "").error, "This is needed");
});

test("text and conditions are kept as typed, trimmed", () => {
  const name = find(fieldGroups("template", ctx), "name");
  assert.deepEqual(parseField(name, "  My frame "), { ok: true, value: "My frame" });
  assert.equal(parseField(name, "  ").error, "This is needed");
  const cond = find(fieldGroups("text", ctx), "condition");
  assert.deepEqual(parseField(cond, "land, nonlegendary"), { ok: true, value: "land, nonlegendary" });
});

test("an edit of a field sets or unsets the path on the item", () => {
  const target = { layout: "normal", kind: "text", key: "title" };
  const size = find(fieldGroups("text", ctx), "fontSize");
  assert.deepEqual(editOps(target, size, { ok: true, value: 20 }), [{ op: "set", layout: "normal", kind: "text", key: "title", path: ["text", "fontSize"], value: 20 }]);
  const color = find(fieldGroups("text", ctx), "color");
  assert.deepEqual(editOps(target, color, { ok: true, unset: true }), [{ op: "unset", layout: "normal", kind: "text", key: "title", path: ["text", "color"] }]);
});

test("a layer's position is written whole, from where it is now", () => {
  const target = { layout: "normal", kind: "layer", key: "crown" };
  const x = find(fieldGroups("layer", ctx), "x");
  const y = find(fieldGroups("layer", ctx), "y");
  assert.deepEqual(editOps(target, x, { ok: true, value: 50 }, { at: { x: 5, y: 6 } }), [{ op: "set", ...target, path: ["at"], value: { x: 50, y: 6 } }]);
  assert.deepEqual(editOps(target, y, { ok: true, value: 9 }, { at: { x: 7, y: 3 } })[0].value, { x: 7, y: 9 });
  assert.deepEqual(layerAt({ at: { x: 5, y: 6 } }, { x: 1, y: 1 }), { x: 5, y: 6 });
  assert.deepEqual(layerAt({}, { x: 7, y: 3 }), { x: 7, y: 3 });
  assert.deepEqual(layerAt({ at: { x: "5" } }, null), { x: 0, y: 0 });
});

test("a layer's slot is the part's, so it is set on the part", () => {
  const target = { layout: "normal", kind: "layer", key: "crown" };
  const slot = find(fieldGroups("layer", ctx), "slot");
  assert.deepEqual(editOps(target, slot, { ok: true, value: "pinlines" }, { part: "crown_part" }), [{ op: "set", layout: "normal", kind: "part", key: "crown_part", path: ["slot"], value: "pinlines" }]);
  assert.deepEqual(editOps(target, slot, { ok: true, unset: true }, { part: "crown_part" })[0].op, "unset");
});

test("the shadow is turned on with the usual numbers and off by taking it out", () => {
  const target = { layout: "normal", kind: "text", key: "mana" };
  assert.deepEqual(shadowOps(target, true), [{ op: "set", ...target, path: ["text", "shadow"], value: { distance: 0.08, angle: 25 } }]);
  assert.deepEqual(shadowOps(target, false), [{ op: "unset", ...target, path: ["text", "shadow"] }]);
});

test("an edit is named for the undo history", () => {
  const size = find(fieldGroups("text", ctx), "fontSize");
  assert.equal(editLabel({ kind: "text", key: "title" }, size), "Change size of title");
  assert.equal(editLabel({ kind: "canvas" }, find(fieldGroups("canvas", ctx), "width")), "Change width of canvas");
  assert.equal(editLabel({ kind: "template" }, find(fieldGroups("template", ctx), "name")), "Change name of template");
});

test("credits copied with a template cannot be changed or removed", () => {
  const credits = [{ name: "Original", note: "art" }, { name: "Copy credit", note: "Forked from x 1" }, { name: "Mine" }];
  assert.equal(creditEdit(credits, 0, "name", "X", 2), null);
  assert.equal(creditEdit(credits, 1, "name", "X", 2), null);
  assert.deepEqual(creditEdit(credits, 2, "url", "https://e.x", 2)[2], { name: "Mine", url: "https://e.x" });
  assert.deepEqual(creditEdit(credits, 2, "name", "Me too", 2)[2], { name: "Me too" });
  assert.equal(creditRemove(credits, 1, 2), null);
  assert.deepEqual(creditRemove(credits, 2, 2), credits.slice(0, 2));
  assert.equal(creditRemove(credits, 9, 0), null);
  assert.deepEqual(creditAdd(credits).at(-1), { name: "New credit" });
  assert.deepEqual(creditAdd(undefined), [{ name: "New credit" }]);
});

test("clearing an optional field of a credit takes the key out, and the name stays", () => {
  const credits = [{ name: "A", url: "u", note: "n" }];
  assert.deepEqual(creditEdit(credits, 0, "url", "", 0), [{ name: "A", note: "n" }]);
  assert.deepEqual(creditEdit(credits, 0, "name", "", 0), [{ name: "", url: "u", note: "n" }]);
});

test("the canvas has one group of its size, resolution and rotation, with the size kept in proportion", () => {
  const groups = fieldGroups("canvas", { vocab });
  assert.deepEqual(groups.map((g) => g.title), ["Size"]);
  assert.deepEqual(groups[0].fields.map((f) => f.id), ["width", "height", "dpi", "rotate"]);
  assert.deepEqual(groups[0].ratio, ["width", "height"]);
  // The sides and the resolution say their units, and a turn is in degrees
  assert.deepEqual(groups[0].fields.map((f) => f.unit), ["px", "px", "dpi", undefined]);
  const rotate = groups[0].fields[3];
  assert.equal(rotate.label, "Rotation");
  assert.deepEqual(rotate.options.map((o) => o.label), ["0 deg", "90 deg", "180 deg", "270 deg"]);
  assert.deepEqual(rotate.options.map((o) => o.value), ["0", "90", "180", "270"]);
});

test("keeping a ratio makes the other side follow the one changed", () => {
  // A canvas 1000 by 1400, which is 5 to 7
  const ratio = 1000 / 1400;
  assert.deepEqual(keepRatio("width", 500, ratio), { width: 500, height: 700 });
  assert.deepEqual(keepRatio("height", 700, ratio), { width: 500, height: 700 });
  assert.deepEqual(keepRatio("width", 1001, ratio), { width: 1001, height: 1401 });
  // The other side is whole pixels and at least one
  assert.deepEqual(keepRatio("width", 3, ratio), { width: 3, height: 4 });
  assert.deepEqual(keepRatio("width", 1, 100), { width: 1, height: 1 });
  assert.deepEqual(keepRatio("height", 1, 0.001), { width: 1, height: 1 });
});

test("a change of size is the edits for the sides that differ, in the layout's canvas", () => {
  const target = { layout: "normal", kind: "canvas", key: "" };
  assert.deepEqual(sizeOps(target, { width: 500, height: 700 }, { width: 1000, height: 1400 }), [
    { op: "set", layout: "normal", kind: "canvas", key: "", path: ["width"], value: 500 },
    { op: "set", layout: "normal", kind: "canvas", key: "", path: ["height"], value: 700 },
  ]);
  assert.deepEqual(sizeOps(target, { width: 500, height: 1400 }, { width: 1000, height: 1400 }).map((o) => o.path), [["width"]]);
  assert.deepEqual(sizeOps(target, { width: 1000, height: 1400 }, { width: 1000, height: 1400 }), []);
});

test("a text box has a Flow group for the layer it steps around and the box it stays clear of", () => {
  const ctx = { vocab, stackIds: ["background", "frame", "pt"], textKeys: ["title", "oracle", "pt"], selfKey: "oracle" };
  const flow = fieldGroups("text", ctx).find((g) => g.title === "Flow");
  assert.deepEqual(flow.fields.map((f) => f.id), ["avoidLayer", "clearOf", "clearGap"]);
  assert.deepEqual(flow.fields.map((f) => f.path), [["flow", "avoidLayer"], ["flow", "clearOf"], ["flow", "clearGap"]]);
  assert.deepEqual(flow.fields[0].options.map((o) => o.value), ["background", "frame", "pt"]);
  // A box does not stay clear of itself
  assert.deepEqual(flow.fields[1].options.map((o) => o.value), ["title", "pt"]);
  assert.deepEqual(parseField(flow.fields[0], ""), { ok: true, unset: true });
  assert.deepEqual(parseField(flow.fields[0], "pt"), { ok: true, value: "pt" });
  assert.deepEqual(parseField(flow.fields[2], "0.75"), { ok: true, value: 0.75 });
  assert.equal(parseField(flow.fields[2], "-1").ok, false);
});

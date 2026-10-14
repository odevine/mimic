import test from "node:test";
import assert from "node:assert/strict";

import {
  materialLabel,
  groupMaterials,
  cellNote,
  partSummary,
  partOfItem,
  boxKey,
  idKey,
  partNameFor,
  newTextBox,
  newArtWindow,
  newLayer,
  duplicateOp,
  deleteOp,
  roleOps,
  roleChoice,
  removeFontOps,
  removeImageOps,
} from "../js/editorassets.js";

test("materials are named as a person would", () => {
  assert.equal(materialLabel("any"), "Any card");
  assert.equal(materialLabel("w"), "White");
  assert.equal(materialLabel("wg"), "White-Green");
  assert.equal(materialLabel("wubrg"), "White-Blue-Black-Red-Green");
  assert.equal(materialLabel("gold"), "Gold");
  assert.equal(materialLabel("sun_moon"), "Sun moon");
});

const material = (key, state, extra = {}) => ({ key, state, ...extra });

test("materials are grouped into any, colors, pairs and the others, in the order given", () => {
  const groups = groupMaterials([
    material("any", "image"),
    material("w", "image"),
    material("u", "missing"),
    material("wu", "blended", { from: ["w", "u"] }),
    material("gold", "missing"),
    material("land", "missing"),
    material("old", "extra"),
  ]);
  assert.deepEqual(groups.map((g) => g.title), ["Any card", "Colors", "Pairs and runs", "Other"]);
  assert.deepEqual(groups[1].cells.map((c) => c.label), ["White", "Blue"]);
  assert.deepEqual(groups[3].cells.map((c) => c.key), ["gold", "land", "old"]);
  // A part with only the any image has one group, and a part with nothing has none
  assert.deepEqual(groupMaterials([material("any", "image")]).map((g) => g.id), ["any"]);
  assert.deepEqual(groupMaterials([]), []);
  assert.deepEqual(groupMaterials(null), []);
});

test("a cell says what draws its material", () => {
  assert.equal(cellNote(material("w", "image", { width: 2928, height: 4080 })), "2928 × 4080");
  assert.equal(cellNote(material("wu", "blended", { from: ["w", "u"] })), "blended from White and Blue");
  assert.equal(cellNote(material("gold", "any")), "uses the any image");
  assert.equal(cellNote(material("gold", "missing")), "no image");
  assert.equal(cellNote(material("old", "extra")), "not read by this slot");
});

test("a part's summary counts its images and the holes", () => {
  const part = { materials: [material("any", "missing"), material("w", "image"), material("u", "image"), material("old", "extra"), material("wu", "blended"), material("gold", "missing")] };
  assert.equal(partSummary(part), "2 of 5 materials have an image, 2 would leave a hole");
  assert.equal(partSummary({ materials: [material("any", "image")] }), "1 image");
  assert.equal(partSummary({ materials: [material("any", "missing")] }), "no image");
  assert.equal(partSummary({ materials: [material("w", "image"), material("u", "any")] }), "1 of 2 materials have an image");
  assert.equal(partSummary(null), "no image");
});

test("a layer finds its part among the app's, and nothing else has one", () => {
  const assets = { parts: [{ name: "frame" }, { name: "bg" }] };
  assert.equal(partOfItem({ kind: "layer", part: "bg" }, assets).name, "bg");
  assert.equal(partOfItem({ kind: "layer", part: "none" }, assets), null);
  assert.equal(partOfItem({ kind: "text", part: "bg" }, assets), null);
  assert.equal(partOfItem({ kind: "layer", part: "bg" }, null), null);
  assert.equal(partOfItem(null, assets), null);
});

test("a text box is named with letters, digits and underscores and kept apart from the ones taken", () => {
  assert.equal(boxKey("flavor", []), "flavor");
  assert.equal(boxKey("flavor", ["flavor"]), "flavor_2");
  assert.equal(boxKey("flavor", ["flavor", "flavor_2"]), "flavor_3");
  assert.equal(boxKey("my box!", []), "my_box");
  assert.equal(boxKey("9lives", []), "lives");
  assert.equal(boxKey("", []), "box");
  assert.equal(boxKey("!!!", ["box"]), "box_2");
  assert.equal(boxKey("Title-ish", []), "Title_ish", "a hyphen is not allowed in a text box's name");
});

test("a layer's id is lowercase with hyphens allowed and kept apart from the ones taken", () => {
  assert.equal(idKey("Frame", []), "frame");
  assert.equal(idKey("frame", ["frame"]), "frame-2");
  assert.equal(idKey("frame", ["frame", "frame-2"]), "frame-3");
  assert.equal(idKey("My Glow!", []), "my-glow");
  assert.equal(idKey("snake_case", []), "snake_case");
  assert.equal(idKey("", ["layer"]), "layer-2");
});

test("a part's name is lowercase letters, digits, underscores and hyphens", () => {
  assert.equal(partNameFor("Gold Pinlines.png", []), "gold_pinlines");
  assert.equal(partNameFor("2 frame", []), "2_frame");
  assert.equal(partNameFor("_frame", []), "frame");
  assert.equal(partNameFor("frame", ["frame"]), "frame_2");
  assert.equal(partNameFor("frame", ["frame", "frame_2"]), "frame_3");
  assert.equal(partNameFor("", []), "layer");
  assert.equal(partNameFor("???.png", ["layer"]), "layer_2");
  assert.equal(partNameFor("a-b", []), "a-b");
});

const view = { layout: "normal", delivered: { width: 1000, height: 1400 } };

test("a new text box is named for what it draws and placed in the middle", () => {
  const op = newTextBox(view, "flavor", ["title", "flavor"]);
  assert.equal(op.op, "add");
  assert.equal(op.kind, "text");
  assert.equal(op.key, "flavor_2");
  assert.deepEqual(op.rect, { x: 150, y: 644, width: 700, height: 112 });
  const value = op.value;
  assert.equal(value.draws, "flavor");
  assert.ok(value.text.fontSize >= 8 && value.text.fontSize < 112);
  assert.equal(value.rect, undefined, "the place is sent beside the value, in the finished image's pixels");
});

test("a new art window goes at the end of the list after the layer given", () => {
  const op = newArtWindow(view, "frame", 2);
  assert.equal(op.kind, "art");
  assert.equal(op.key, "2");
  assert.deepEqual(op.value, { after: "frame" });
  assert.deepEqual(op.rect, { x: 100, y: 210, width: 800, height: 560 });
  assert.deepEqual(newArtWindow(view, "", 0).value, {});
});

test("a new layer is named for its part, and takes an id only when the name is used", () => {
  const stack = [{ id: "bg", part: "bg" }, { part: "frame" }];
  const a = newLayer("normal", "glow", stack);
  assert.deepEqual([a.key, a.to, a.value], ["glow", 2, { part: "glow" }]);
  const b = newLayer("normal", "frame", stack, 1);
  assert.deepEqual([b.key, b.to, b.value], ["frame-2", 1, { id: "frame-2", part: "frame" }]);
});

test("a copy is moved a little and named for what it copies", () => {
  const text = duplicateOp(view, { kind: "text", key: "title" }, ["title"]);
  assert.deepEqual(text, { op: "duplicate", layout: "normal", kind: "text", key: "title", dx: 20, dy: 20, as: "title_copy" });
  assert.equal(duplicateOp(view, { kind: "layer", key: "frame" }, ["frame", "frame-copy"]).as, "frame-copy-2");
  assert.equal(duplicateOp(view, { kind: "text", key: "title" }, ["title", "title_copy"]).as, "title_copy_2");
  const art = duplicateOp(view, { kind: "art", key: "0" }, []);
  assert.equal(art.as, undefined);
  assert.equal(duplicateOp({ ...view, delivered: { width: 100, height: 140 } }, { kind: "art", key: "0" }, []).dx, 8);
});

test("deleting names the element and the layout", () => {
  assert.deepEqual(deleteOp(view, { kind: "text", key: "pt" }), { op: "delete", layout: "normal", kind: "text", key: "pt" });
});

test("a role is set to a font or an engine font, or back to its default", () => {
  const body = { role: "body", standard: true };
  assert.deepEqual(roleOps(body, "font:plantin"), [{ op: "set", kind: "template", path: ["roles", "body"], value: { font: "plantin" } }]);
  assert.deepEqual(roleOps(body, "builtin:mimic-m15-title"), [{ op: "set", kind: "template", path: ["roles", "body"], value: { builtin: "mimic-m15-title" } }]);
  assert.deepEqual(roleOps(body, ""), [{ op: "unset", kind: "template", path: ["roles", "body"] }]);
  const saga = { role: "saga-name", standard: false };
  assert.equal(roleOps(saga, "font:x", "Saga names")[0].value.label, "Saga names");
  assert.equal(roleOps(saga, "font:x", "saga-name")[0].value.label, undefined, "a label that is just the name is not kept");
  assert.equal(roleOps(body, "font:x", "Rules text")[0].value.label, undefined, "a standard role has no label to keep");
});

test("a role's choice reads back as the form the select takes", () => {
  assert.equal(roleChoice({ font: "plantin" }), "font:plantin");
  assert.equal(roleChoice({ builtin: "mimic-m15-info" }), "builtin:mimic-m15-info");
  assert.equal(roleChoice({}), "");
});

test("removing a font frees its roles, takes its entry out and drops its file", () => {
  const assets = { roles: [{ role: "title", font: "x" }, { role: "body", font: "y" }, { role: "info" }, { role: "saga", font: "x" }] };
  assert.deepEqual(removeFontOps({ id: "x", file: "font/x.ttf" }, assets), [
    { op: "unset", kind: "template", path: ["roles", "title"] },
    { op: "unset", kind: "template", path: ["roles", "saga"] },
    { op: "unset", kind: "template", path: ["fonts", "x"] },
    { op: "drop", file: "font/x.ttf" },
  ]);
});

test("removing an image drops its file and a variant the part names itself", () => {
  const part = { name: "frame" };
  const m = { key: "w", file: "parts/frame/w.png" };
  assert.deepEqual(removeImageOps(part, m, {}), [{ op: "drop", file: "parts/frame/w.png" }]);
  assert.deepEqual(removeImageOps(part, m, { w: "parts/frame/w.png" }), [
    { op: "unset", kind: "part", key: "frame", path: ["variants", "w"] },
    { op: "drop", file: "parts/frame/w.png" },
  ]);
  assert.deepEqual(removeImageOps(part, m, undefined), [{ op: "drop", file: "parts/frame/w.png" }]);
});

import test from "node:test";
import assert from "node:assert/strict";

import { intersect, avoidance } from "../js/editorflow.js";

const r = (x, y, width, height) => ({ x, y, width, height });
const layer = (key, rect, drawn = true) => ({ kind: "layer", key, drawn, delivered: drawn ? rect : r(0, 0, 0, 0) });
const oracle = { kind: "text", key: "oracle", drawn: true };

test("two rectangles share the part where they overlap, and nothing when they only touch", () => {
  assert.deepEqual(intersect(r(0, 0, 10, 10), r(5, 5, 10, 10)), r(5, 5, 5, 5));
  assert.deepEqual(intersect(r(0, 0, 10, 10), r(2, 3, 4, 2)), r(2, 3, 4, 2));
  assert.equal(intersect(r(0, 0, 10, 10), r(10, 0, 5, 5)), null);
  assert.equal(intersect(r(0, 0, 10, 10), r(20, 20, 5, 5)), null);
  assert.equal(intersect(r(0, 0, 10, 10), r(1, 1, 0, 5)), null);
});

test("a box that names no layer avoids nothing", () => {
  assert.equal(avoidance(oracle, r(0, 0, 100, 100), {}, []), null);
  assert.equal(avoidance(oracle, r(0, 0, 100, 100), undefined, []), null);
  assert.equal(avoidance(oracle, r(0, 0, 100, 100), { avoidLayer: "" }, []), null);
  // Only a text box flows
  assert.equal(avoidance({ kind: "art", key: "0" }, r(0, 0, 10, 10), { avoidLayer: "pt" }, [layer("pt", r(0, 0, 5, 5))]), null);
});

test("the region is the part of the layer that lies in the box", () => {
  const items = [layer("frame", r(0, 0, 1000, 1400)), layer("pt", r(700, 1200, 200, 100))];
  const got = avoidance(oracle, r(100, 900, 700, 350), { avoidLayer: "pt" }, items);
  assert.deepEqual(got, { state: "active", layer: "pt", rect: r(700, 1200, 100, 50), note: "Steps around pt" });
});

test("the box as it is dragged is the one the region is cut to", () => {
  const items = [layer("pt", r(700, 1200, 200, 100))];
  assert.equal(avoidance(oracle, r(100, 900, 700, 350), { avoidLayer: "pt" }, items).state, "active");
  assert.equal(avoidance(oracle, r(100, 100, 300, 300), { avoidLayer: "pt" }, items).state, "clear");
});

test("a layer the card does not draw leaves the text the whole box, and a layer that is not there is said", () => {
  const idle = avoidance(oracle, r(0, 0, 100, 100), { avoidLayer: "pt" }, [layer("pt", null, false)]);
  assert.equal(idle.state, "idle");
  assert.equal(idle.rect, null);
  assert.match(idle.note, /does not draw, so the text has the whole box/);
  const gone = avoidance(oracle, r(0, 0, 100, 100), { avoidLayer: "pt" }, [layer("frame", r(0, 0, 10, 10))]);
  assert.equal(gone.state, "missing");
  assert.match(gone.note, /pt, which is not in the stack/);
  // A text box or art window of the same key is not a layer
  assert.equal(avoidance(oracle, r(0, 0, 10, 10), { avoidLayer: "oracle" }, [oracle]).state, "missing");
});

test("a layer that is drawn but does not reach the box says so", () => {
  const got = avoidance(oracle, r(0, 0, 100, 100), { avoidLayer: "pt" }, [layer("pt", r(500, 500, 50, 50))]);
  assert.equal(got.state, "clear");
  assert.match(got.note, /does not reach this box/);
});

import { readout, fitNote, placementNote, overrideNote, selectionNotes } from "../js/editorflow.js";

test("the readout puts the place in one column and the size in the other", () => {
  const cells = readout({ kind: "text" }, r(10, 20, 300, 40));
  assert.deepEqual(cells, [
    { label: "X", value: "10" },
    { label: "Width", value: "300" },
    { label: "Y", value: "20" },
    { label: "Height", value: "40" },
  ]);
  const empty = readout({ kind: "layer" }, r(0, 0, 0, 0));
  assert.deepEqual(empty.map((c) => c.value), ["", "", "", ""]);
  assert.deepEqual(empty.map((c) => c.label), ["X", "Width", "Y", "Height"]);
});

test("a box set on its baseline reads as the line its text sits on", () => {
  const item = { kind: "text", baseline: r(100, 220, 600, 0) };
  assert.deepEqual(readout(item, r(100, 180, 420, 52)), [
    { label: "X", value: "100" },
    { label: "Width", value: "600" },
    { label: "Baseline", value: "220" },
    { label: "", value: "" },
  ]);
  // A turned layout makes the line a vertical one, which runs down from its Y
  const turned = { kind: "text", baseline: r(220, 100, 0, 600) };
  assert.deepEqual(readout(turned, r(180, 100, 52, 420)), [
    { label: "Baseline", value: "220" },
    { label: "Length", value: "600" },
    { label: "Y", value: "100" },
    { label: "", value: "" },
  ]);
});

test("the fit is said when the text shrank or does not fit, and not when it did", () => {
  assert.equal(fitNote(null), null);
  assert.equal(fitNote({ size: 36, wanted: 36, floor: 20, overflows: false }), null);
  assert.deepEqual(fitNote({ size: 30.04, wanted: 36, floor: 20, overflows: false }), { level: "info", text: "The text is shrunk to 30 px to fit, from the 36 px the box asks for" });
  assert.deepEqual(fitNote({ size: 20, wanted: 36, floor: 20, overflows: true }), { level: "warn", text: "The text does not fit even at its smallest size, 20 px, so it runs past the box" });
  // Text that fits at the size asked for, within rounding, says nothing
  assert.equal(fitNote({ size: 35.99, wanted: 36, floor: 20, overflows: false }), null);
});

test("a box the card moved or narrowed says which, naming the box from its flow", () => {
  assert.equal(placementNote({ moved: false, narrowed: false }, {}), null);
  assert.equal(placementNote({ narrowed: true }, { clearOf: "mana" }).text, "Narrowed to stay clear of mana for this card");
  assert.equal(placementNote({ moved: true }, { duck: { layer: "pt", row: "artist" } }).text, "Moved to the row of artist for this card");
  assert.equal(placementNote({ moved: true, narrowed: true }, { clearOf: "mana", duck: { row: "artist" } }).text, "Moved to the row of artist and narrowed to stay clear of mana for this card");
  // The flow may not name them, as when the layout was changed since
  assert.equal(placementNote({ moved: true, narrowed: true }, null).text, "Moved to another row and narrowed to stay clear of another box for this card");
});

test("an override in force is named by its condition", () => {
  assert.equal(overrideNote({}), null);
  assert.match(overrideNote({ override: { when: "legendary", index: 1 } }).text, /An override for "legendary" is in force for this card/);
  assert.match(overrideNote({ override: { when: "", index: 0 } }).text, /^An override is in force/);
});

test("the notes under the selection come in the order they matter", () => {
  const item = { kind: "text", drawn: true, narrowed: true, override: { when: "land", index: 0 }, fit: { size: 20, wanted: 30, floor: 18, overflows: false } };
  const notes = selectionNotes(item, { locked: true, hidden: true, flow: { clearOf: "mana" }, avoid: { note: "Steps around pt" } });
  assert.deepEqual(notes.map((n) => n.text.split(" ").slice(0, 2).join(" ")), ["Hidden in", "Locked, so", "Steps around", "Narrowed to", "An override", "The text"]);
  assert.deepEqual(selectionNotes({ kind: "layer", drawn: false, why: "its condition is not met" }, { forced: true }).map((n) => n.text), ["Not drawn: its condition is not met", "Drawn whatever its condition says, for this preview only"]);
  assert.deepEqual(selectionNotes(null), []);
  assert.deepEqual(selectionNotes({ kind: "text", drawn: true }), []);
});

import { placedLine, captionTags } from "../js/editorflow.js";

test("a box on its baseline that the card placed is marked by the line it ends up on", () => {
  const item = { baseline: r(100, 220, 600, 0), placed: r(100, 220, 380, 80) };
  // Unturned, the baseline is the top of the box, and the box's height below it is left out
  assert.deepEqual(placedLine(item), { x: 100, y: 220, width: 380, height: 0 });
  assert.deepEqual(placedLine(item, 0, false), { x: 100, y: 220, width: 380, height: 0 });
  // A turned layout carries the top round to the right, the bottom and the left
  assert.deepEqual(placedLine({ baseline: r(0, 0, 0, 5), placed: r(220, 100, 80, 380) }, 90), { x: 300, y: 100, width: 0, height: 380 });
  assert.deepEqual(placedLine(item, 180), { x: 100, y: 300, width: 380, height: 0 });
  assert.deepEqual(placedLine({ baseline: r(0, 0, 0, 5), placed: r(220, 100, 80, 380) }, 270), { x: 220, y: 100, width: 0, height: 380 });
  // A box laid out in the finished image is not turned, so its baseline is its top
  assert.deepEqual(placedLine(item, 90, true), { x: 100, y: 220, width: 380, height: 0 });
  // Anything else has no line: a box with an area, or one the card left alone
  assert.equal(placedLine({ placed: r(1, 2, 3, 4) }), null);
  assert.equal(placedLine({ baseline: r(1, 2, 3, 0) }), null);
  assert.equal(placedLine(null), null);
});

test("the tags under the selection name each mark on the card and none that is not there", () => {
  assert.deepEqual(captionTags(null), []);
  assert.deepEqual(captionTags({ kind: "text" }), []);
  assert.deepEqual(captionTags({ baseline: r(1, 2, 3, 0), narrowed: true }), [
    { text: "baseline", tone: "line" },
    { text: "Narrowed", tone: "placed" },
  ]);
  assert.equal(captionTags({ moved: true }).at(0).text, "Moved");
  assert.equal(captionTags({ moved: true, narrowed: true }).at(0).text, "Moved and narrowed");
  // What it interacts with is named by what it does, and the other way round for the other box
  const links = [
    { via: "avoid", label: "pt", reverse: false },
    { via: "clear", label: "mana", reverse: false },
    { via: "duck", label: "crown", reverse: false },
    { via: "avoid", label: "oracle", reverse: true },
    { via: "clear", label: "title", reverse: true },
    { via: "duck", label: "artist", reverse: true },
  ];
  assert.deepEqual(captionTags({}, links).map((t) => t.text), ["Avoids pt", "Clears mana", "Moves for crown", "Avoided by oracle", "Cleared by title", "artist moves for it"]);
  assert.ok(captionTags({}, links).every((t) => t.tone === "avoid"));
});

import { extentOf, referrers, flowZones } from "../js/editorflow.js";

const txt = (key, delivered, extra = {}) => ({ kind: "text", key, drawn: true, delivered, ...extra });
const lay = (key, delivered, drawn = true) => ({ kind: "layer", key, drawn, delivered });

test("a box's own room is its rectangle, or for a baseline box the width of its line as tall as its text", () => {
  assert.deepEqual(extentOf(txt("oracle", r(100, 900, 800, 300))), r(100, 900, 800, 300));
  assert.deepEqual(extentOf(txt("title", r(100, 100, 420, 52), { baseline: r(100, 160, 700, 0) })), r(100, 100, 700, 52));
  // A line that runs down, in a turned layout, is as wide as its text is
  assert.deepEqual(extentOf(txt("title", r(100, 100, 52, 420), { baseline: r(160, 100, 0, 700) })), r(100, 100, 52, 700));
});

test("the boxes that name an item in their flow are found, by what they do", () => {
  const doc = {
    textBoxes: {
      oracle: { flow: { avoidLayer: "pt" } },
      title: { flow: { clearOf: "mana", duck: { layer: "crown", row: "pt" } } },
      type: { flow: { duck: { layer: "pt", row: "artist" } } },
      mana: {},
      loop: { flow: { clearOf: "loop" } },
    },
  };
  assert.deepEqual(referrers(doc, lay("pt")), [{ key: "oracle", via: "avoid" }, { key: "type", via: "duck" }]);
  assert.deepEqual(referrers(doc, lay("crown")), [{ key: "title", via: "duck" }]);
  assert.deepEqual(referrers(doc, txt("mana")), [{ key: "title", via: "clear" }]);
  // A box does not interact with itself, and a layer is not cleared and a box is not avoided
  assert.deepEqual(referrers(doc, txt("loop")), []);
  assert.deepEqual(referrers(doc, lay("mana")), []);
  assert.deepEqual(referrers(doc, { kind: "art", key: "0" }), []);
  assert.deepEqual(referrers(null, lay("pt")), []);
});

test("a baseline box has room on the side of its text that the card leaves it, and the room ends where the card ends it", () => {
  const title = txt("title", r(100, 100, 300, 52), { baseline: r(100, 150, 700, 0) });
  const mana = txt("mana", r(600, 100, 200, 52), { baseline: r(600, 150, 200, 0) });
  const items = [title, mana];
  // Not narrowed: the room is from the text to the end of the box's line
  let z = flowZones({ item: title, rect: title.delivered, line: title.baseline, reach: title.baseline, flow: { clearOf: "mana" }, items });
  assert.deepEqual(z.ghosts, [r(400, 100, 400, 52)]);
  // The box the title keeps clear of is outlined, and the room the title has as written meets it where they overlap
  assert.deepEqual(z.links, [{ via: "clear", label: "mana", rect: mana.delivered, reverse: false }]);
  assert.deepEqual(z.hazards, [r(600, 100, 200, 52)]);
  // Narrowed by the card: the room stops at the narrowed line, short of the box, and what was lost is the hazard
  z = flowZones({ item: title, rect: title.delivered, line: title.baseline, reach: r(100, 150, 450, 0), flow: { clearOf: "mana" }, items });
  assert.deepEqual(z.ghosts, [r(400, 100, 150, 52)]);
  assert.deepEqual(z.hazards, [r(600, 100, 200, 52)]);
  // Text that is aligned right or centered has room on both sides
  const centered = txt("c", r(300, 100, 100, 52), { baseline: r(100, 150, 500, 0) });
  assert.deepEqual(flowZones({ item: centered, rect: centered.delivered, line: centered.baseline, reach: centered.baseline, items: [centered] }).ghosts, [r(100, 100, 200, 52), r(400, 100, 200, 52)]);
  // No room beside text that fills its line
  const full = txt("f", r(100, 100, 500, 52), { baseline: r(100, 150, 500, 0) });
  assert.deepEqual(flowZones({ item: full, rect: full.delivered, line: full.baseline, reach: full.baseline, items: [full] }).ghosts, []);
});

test("a line that runs down has its room above and below its text", () => {
  const t = txt("t", r(100, 300, 52, 100), { baseline: r(150, 100, 0, 700) });
  const z = flowZones({ item: t, rect: t.delivered, line: t.baseline, reach: t.baseline, items: [t] });
  assert.deepEqual(z.ghosts, [r(100, 100, 52, 200), r(100, 400, 52, 400)]);
});

test("a box with an area has the whole box as its room, or what the card left it, and loses where a layer overlaps", () => {
  const pt = lay("pt", r(700, 1100, 200, 150));
  const oracle = txt("oracle", r(100, 900, 800, 300));
  let z = flowZones({ item: oracle, rect: oracle.delivered, flow: { avoidLayer: "pt" }, items: [oracle, pt] });
  assert.deepEqual(z.ghosts, [r(100, 900, 800, 300)]);
  assert.deepEqual(z.links.map((l) => [l.via, l.label]), [["avoid", "pt"]]);
  assert.deepEqual(z.hazards, [r(700, 1100, 200, 100)]);
  // A box the card narrowed has its narrowed rectangle for room
  z = flowZones({ item: oracle, rect: oracle.delivered, reach: r(100, 900, 600, 300), flow: {}, items: [oracle] });
  assert.deepEqual(z.ghosts, [r(100, 900, 600, 300)]);
  // A layer the card does not draw is not in the way, and one that does not reach the box has no overlap
  assert.deepEqual(flowZones({ item: oracle, rect: oracle.delivered, flow: { avoidLayer: "pt" }, items: [oracle, lay("pt", null, false)] }).links, []);
  const far = flowZones({ item: oracle, rect: oracle.delivered, flow: { avoidLayer: "pt" }, items: [oracle, lay("pt", r(10, 10, 20, 20))] });
  assert.equal(far.links.length, 1);
  assert.deepEqual(far.hazards, []);
});

test("a box that moves for a layer outlines the layer and loses no room to it", () => {
  const title = txt("title", r(100, 100, 300, 52), { baseline: r(100, 150, 700, 0) });
  const z = flowZones({ item: title, rect: title.delivered, line: title.baseline, reach: title.baseline, flow: { duck: { layer: "crown", row: "pt" } }, items: [title, lay("crown", r(300, 80, 200, 100))] });
  assert.deepEqual(z.links.map((l) => [l.via, l.label]), [["duck", "crown"]]);
  assert.deepEqual(z.hazards, []);
});

test("a layer selected shows the boxes that steer around it, and where their room meets it", () => {
  const pt = lay("pt", r(700, 1100, 200, 150));
  const oracle = txt("oracle", r(100, 900, 800, 300));
  const type = txt("type", r(100, 800, 300, 40), { baseline: r(100, 830, 800, 0) });
  const z = flowZones({ item: pt, rect: pt.delivered, refs: [{ key: "oracle", via: "avoid" }, { key: "type", via: "duck" }, { key: "gone", via: "avoid" }], items: [pt, oracle, type] });
  // No room is drawn for a layer
  assert.deepEqual(z.ghosts, []);
  assert.deepEqual(z.links.map((l) => [l.via, l.label, l.reverse]), [["avoid", "oracle", true], ["duck", "type", true]]);
  assert.deepEqual(z.links[0].rect, oracle.delivered);
  assert.deepEqual(z.links[1].rect, r(100, 800, 800, 40));
  // The oracle's room meets the layer, and the type, which moves for it, loses none
  assert.deepEqual(z.hazards, [r(700, 1100, 200, 100)]);
});

test("a box selected also shows the boxes that stay clear of it, and the room they would lose to it", () => {
  const mana = txt("mana", r(600, 100, 200, 52), { baseline: r(600, 150, 200, 0) });
  const title = txt("title", r(100, 100, 300, 52), { baseline: r(100, 150, 700, 0) });
  const z = flowZones({ item: mana, rect: mana.delivered, line: mana.baseline, reach: mana.baseline, refs: [{ key: "title", via: "clear" }], items: [mana, title] });
  assert.deepEqual(z.links.map((l) => [l.via, l.label, l.reverse]), [["clear", "title", true]]);
  assert.deepEqual(z.links[0].rect, r(100, 100, 700, 52));
  assert.deepEqual(z.hazards, [r(600, 100, 200, 52)]);
  assert.deepEqual(z.ghosts, []);
});

test("a selected box moved while dragged has its room and hazards follow", () => {
  const pt = lay("pt", r(700, 1100, 200, 150));
  const oracle = txt("oracle", r(100, 900, 800, 300));
  const moved = flowZones({ item: oracle, rect: r(100, 1000, 800, 300), flow: { avoidLayer: "pt" }, items: [oracle, pt] });
  assert.deepEqual(moved.hazards, [r(700, 1100, 200, 150)]);
  assert.deepEqual(moved.ghosts, [r(100, 1000, 800, 300)]);
});

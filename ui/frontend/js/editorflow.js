// Where a text box's text keeps out of a layer, as pure functions of what the app
// described and what the layout file says. The engine avoids the visible pixels of the
// layer a box names as its avoidLayer, and only for a card whose render draws that layer,
// which is what the app reports for each layer, so the region needs no more than that

const area = (r) => (r.width > 0 && r.height > 0 ? r.width * r.height : 0);

// intersect is the rectangle two rectangles share, or null when they share none
export function intersect(a, b) {
  const x = Math.max(a.x, b.x);
  const y = Math.max(a.y, b.y);
  const right = Math.min(a.x + a.width, b.x + b.width);
  const bottom = Math.min(a.y + a.height, b.y + b.height);
  const r = { x, y, width: right - x, height: bottom - y };
  return area(r) > 0 ? r : null;
}

// avoidance says what a text box's avoidLayer does for the card being drawn, as
// { state, layer, rect, note }, or null for a box that names no layer. rect is the part
// of the layer that lies in the box, in the finished image's pixels, and is set only
// while state is "active". box is the item, rect its place as it is now, flow the
// box's flow from the layout file, and items every item the layout draws
export function avoidance(box, rect, flow, items) {
  const id = flow && typeof flow.avoidLayer === "string" ? flow.avoidLayer : "";
  if (!id || !box || box.kind !== "text") return null;
  const layer = (items || []).find((it) => it.kind === "layer" && it.key === id);
  if (!layer) return { state: "missing", layer: id, rect: null, note: `Steps around ${id}, which is not in the stack` };
  if (!layer.drawn) return { state: "idle", layer: id, rect: null, note: `Steps around ${id}, which this card does not draw, so the text has the whole box` };
  const shared = rect && intersect(layer.delivered, rect);
  if (!shared) return { state: "clear", layer: id, rect: null, note: `Steps around ${id}, which does not reach this box` };
  return { state: "active", layer: id, rect: shared, note: `Steps around ${id}` };
}

// --- what is said about the selected text box ---

const fixed = (n) => (Number.isInteger(n) ? String(n) : String(Math.round(n * 10) / 10));

// anchorName is what the line a box is set on is called, from the anchor it is measured to,
// where an item that names none is on its baseline
export const anchorName = (item) => ({ "cap-top": "cap top", "cap-center": "cap center" })[item && item.anchor] || "baseline";
const capital = (s) => s[0].toUpperCase() + s.slice(1);

// readout is the four numbers under the selection's name, as the cells of two columns read
// across: the place in the first and the size in the second. A box set on its baseline is
// the line its text sits on, and its own height is not used, so the line's place and length
// are what is said, with the one across the line named for the anchor, the baseline or the capitals. A cell with nothing
// to say is empty
export function readout(item, rect) {
  const blank = !rect || (rect.width === 0 && rect.height === 0);
  const cell = (label, value) => ({ label, value: blank ? "" : String(value) });
  const line = item && item.baseline;
  if (line && !blank) {
    // A turned layout makes the line a vertical one, which runs down and sits across at X
    if (line.height === 0) return [cell("X", line.x), cell("Width", line.width), cell(capital(anchorName(item)), line.y), { label: "", value: "" }];
    return [cell(capital(anchorName(item)), line.x), cell("Length", line.height), cell("Y", line.y), { label: "", value: "" }];
  }
  return [cell("X", rect ? rect.x : 0), cell("Width", rect ? rect.width : 0), cell("Y", rect ? rect.y : 0), cell("Height", rect ? rect.height : 0)];
}

// fitNote says how a text box's text fitted when it is worth saying: that it shrank, or that
// it does not fit even at the least size. Text that fitted at the size asked for says nothing
export function fitNote(fit) {
  if (!fit) return null;
  if (fit.overflows) return { level: "warn", text: `The text does not fit even at its smallest size, ${fixed(fit.floor)} px, so it runs past the box` };
  if (fit.size < fit.wanted - 0.05) return { level: "info", text: `The text is shrunk to ${fixed(fit.size)} px to fit, from the ${fixed(fit.wanted)} px the box asks for` };
  return null;
}

// placementNote says what the card did to a text box to place it, from its flow in the layout
// file, since the app says that a box moved or narrowed and not which other box it was for
export function placementNote(item, flow) {
  if (!item || (!item.moved && !item.narrowed)) return null;
  const f = flow || {};
  const parts = [];
  if (item.moved) parts.push(f.duck && f.duck.row ? `moved to the row of ${f.duck.row}` : "moved to another row");
  if (item.narrowed) parts.push(f.clearOf ? `narrowed to stay clear of ${f.clearOf}` : "narrowed to stay clear of another box");
  const text = parts.join(" and ");
  return { level: "info", text: text[0].toUpperCase() + text.slice(1) + " for this card" };
}

// overrideNote says that an override of the box is in force, which is why the numbers shown
// for the box are not the ones it is written with
export function overrideNote(item) {
  const o = item && item.override;
  if (!o) return null;
  return { level: "info", text: o.when ? `An override for "${o.when}" is in force for this card, so the box is not as its own fields say` : "An override is in force for this card, so the box is not as its own fields say" };
}

// selectionNotes is everything worth saying under the selection's name besides what it is,
// in the order it matters: that it is not drawn, forced, hidden or locked, what a text box's
// flow does for this card, which override is in force and how its text fitted. locked, hidden
// and forced say how the page has the item marked
export function selectionNotes(item, { locked = false, hidden = false, forced = false, flow = null, avoid = null } = {}) {
  if (!item) return [];
  const notes = [];
  if (!item.drawn) notes.push({ level: "info", text: `Not drawn: ${item.why}` });
  if (forced) notes.push({ level: "info", text: "Drawn whatever its condition says, for this preview only" });
  if (hidden) notes.push({ level: "info", text: "Hidden in this preview only" });
  if (locked) notes.push({ level: "info", text: "Locked, so the card cannot pick or move it" });
  if (avoid) notes.push({ level: "info", text: avoid.note });
  for (const n of [placementNote(item, flow), overrideNote(item), fitNote(item.fit)]) if (n) notes.push(n);
  return notes;
}

// --- how a selected text box is marked on the card ---

// The edge of a box's rectangle that its baseline is on, in the finished image: the top of the
// box as the layout writes it, which a layout turned clockwise carries round to the right, the
// bottom and the left. A box laid out in the finished image itself is not turned
const BASELINE_EDGE = { 0: "top", 90: "right", 180: "bottom", 270: "left" };

// placedLine is the baseline of a box that the card moved or narrowed, where it ends up, as a
// rectangle with no thickness across the line, or null for a box that is not set on its baseline
// or that the card left alone. placed is where the whole box ends up. Its height is not used by a
// box on its baseline, and hangs below the line where the text is not, so only the line is drawn
export function placedLine(item, rotate = 0, output = false) {
  if (!item || !item.baseline || !item.placed) return null;
  const p = item.placed;
  switch (output ? "top" : BASELINE_EDGE[((rotate % 360) + 360) % 360] || "top") {
    case "bottom":
      return { x: p.x, y: p.y + p.height, width: p.width, height: 0 };
    case "right":
      return { x: p.x + p.width, y: p.y, width: 0, height: p.height };
    case "left":
      return { x: p.x, y: p.y, width: 0, height: p.height };
  }
  return { x: p.x, y: p.y, width: p.width, height: 0 };
}

// --- the space a text box has, and what is in its way ---

const nonEmpty = (r) => !!r && r.width > 0 && r.height > 0;

// band is the room a box on its baseline has across its width: the span of the line it
// runs along, as thick as its text is tall. rect is where the text is, line the baseline, and
// along the line whose span is wanted, which is the box's own for the whole width and the one the
// card left it for what remains
function band(rect, line, along) {
  if (line.height === 0) return { x: along.x, y: rect.y, width: along.width, height: rect.height };
  return { x: rect.x, y: along.y, width: rect.width, height: along.height };
}

// extentOf is the room a text box has to itself on the card as the layout gives it: the whole
// of its rectangle, or for a box on its baseline, the width of its line as tall as its text
export function extentOf(item) {
  return item.baseline ? band(item.delivered, item.baseline, item.baseline) : item.delivered;
}

// ghostsOf is the room a box on its baseline has left on each side of its text, from the
// text to the end of the line the card left it, which is a narrower one when the card narrowed
// the box. A line that stops at the text on a side leaves none there
function ghostsOf(rect, reach) {
  const out = [];
  if (reach.height === 0) {
    const lo = reach.x;
    const hi = reach.x + reach.width;
    if (rect.x - lo > 0) out.push({ x: lo, y: rect.y, width: rect.x - lo, height: rect.height });
    if (hi - (rect.x + rect.width) > 0) out.push({ x: rect.x + rect.width, y: rect.y, width: hi - (rect.x + rect.width), height: rect.height });
  } else {
    const lo = reach.y;
    const hi = reach.y + reach.height;
    if (rect.y - lo > 0) out.push({ x: rect.x, y: lo, width: rect.width, height: rect.y - lo });
    if (hi - (rect.y + rect.height) > 0) out.push({ x: rect.x, y: rect.y + rect.height, width: rect.width, height: hi - (rect.y + rect.height) });
  }
  return out;
}

// referrers are the text boxes of a layout that name an item in their flow, as { key, via }
// where via is avoid, clear or duck: the boxes that keep their text out of a layer, stay clear of a
// box, or move for a layer. doc is the layout as its file has it
export function referrers(doc, item) {
  const out = [];
  if (!doc || !item) return out;
  for (const [key, box] of Object.entries(doc.textBoxes || {})) {
    const f = (box && box.flow) || {};
    if (item.kind === "layer" && f.avoidLayer === item.key) out.push({ key, via: "avoid" });
    if (item.kind === "layer" && f.duck && f.duck.layer === item.key) out.push({ key, via: "duck" });
    if (item.kind === "text" && f.clearOf === item.key) out.push({ key, via: "clear" });
  }
  return out.filter((r) => r.key !== item.key);
}

// flowZones is what to draw on the card for how a selected item's space relates to what is
// in its way, from the numbers the app reported and the layout's flow, in the finished image:
//
//   ghosts   the room a text box has left, beside its text for a box on its baseline and the
//            whole box for another, ending where the card ends it
//   links    the things it interacts with, each { via, label, rect, reverse }: the layer it keeps
//            its text out of, the box it stays clear of and the layer it moves for, and in the
//            other direction the boxes that do the same about it
//   hazards  where the room it has as the layout gives it meets something it keeps out of, so
//            the space that cannot be used
//
// item is selected, rect is where its text is, and line its baseline, both as they are now, so a
// drag moves them. reach is the line or rectangle the card leaves it, which is the line or the
// rect itself when there is nothing to say. flow is the box's own flow, refs the boxes that name
// it, and items everything the layout draws
export function flowZones({ item, rect, line = null, reach = null, flow = null, refs = [], items = [] }) {
  const find = (kind, key) => (items || []).find((it) => it.kind === kind && it.key === key && it.drawn && nonEmpty(it.delivered));
  const links = [];
  const f = flow || {};
  if (item && item.kind === "text") {
    const add = (via, kind, key) => {
      const it = key && find(kind, key);
      if (it) links.push({ via, label: key, rect: it.delivered, reverse: false });
    };
    add("avoid", "layer", f.avoidLayer);
    add("clear", "text", f.clearOf);
    add("duck", "layer", f.duck && f.duck.layer);
  }
  for (const r of refs || []) {
    const it = find("text", r.key);
    if (it) links.push({ via: r.via, label: r.key, rect: extentOf(it), reverse: true });
  }
  const own = line ? band(rect, line, line) : rect;
  const hazards = [];
  for (const l of links) {
    // A box that moves for a layer is not kept out of it, so there is no space to lose
    if (l.via === "duck") continue;
    // The room the box has as the layout gives it, against the thing, or for another box
    // that keeps clear of this item, that box's room against this item
    const meets = intersect(l.reverse ? l.rect : own, l.reverse ? rect : l.rect);
    if (meets) hazards.push(meets);
  }
  let ghosts = [];
  if (item && item.kind === "text") ghosts = line ? ghostsOf(rect, reach || line) : [reach || rect];
  return { ghosts, links, hazards };
}

// captionTags are the short names set beneath the selected box that say what its marks on the
// card are, so no word is written over the text. Each is { text, tone }: baseline for the line, what the
// card did to the box for where it ended up, and the things it interacts with, named by what they do
export function captionTags(item, links = []) {
  if (!item) return [];
  const tags = [];
  if (item.baseline) tags.push({ text: anchorName(item), tone: "line" });
  if (item.moved || item.narrowed) tags.push({ text: item.moved && item.narrowed ? "Moved and narrowed" : item.moved ? "Moved" : "Narrowed", tone: "placed" });
  const said = {
    avoid: (l) => (l.reverse ? `Avoided by ${l.label}` : `Avoids ${l.label}`),
    clear: (l) => (l.reverse ? `Cleared by ${l.label}` : `Clears ${l.label}`),
    duck: (l) => (l.reverse ? `${l.label} moves for it` : `Moves for ${l.label}`),
  };
  for (const l of links) tags.push({ text: said[l.via](l), tone: "avoid" });
  return tags;
}

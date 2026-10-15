import test from "node:test";
import assert from "node:assert/strict";

import { emptyVisibility, requestLists, hiddenRefs, eyeState, toggleEye, isEmpty } from "../js/editorvisibility.js";

const layer = (key, drawn = true) => ({ kind: "layer", key, drawn });
const text = (key, drawn = true) => ({ kind: "text", key, drawn });

test("nothing is hidden or forced to begin with", () => {
  const vis = emptyVisibility();
  assert.equal(isEmpty(vis), true);
  assert.deepEqual(requestLists(vis, "normal"), { hide: [], show: [] });
  assert.equal(eyeState(vis, "normal", layer("frame")).kind, "shown");
});

test("pressing the eye of an item that shows hides it, and pressing it again shows it", () => {
  const first = toggleEye(emptyVisibility(), "normal", text("title"));
  assert.deepEqual(first, { vis: { hidden: ["normal/text:title"], forced: [] }, described: false });
  assert.equal(eyeState(first.vis, "normal", text("title")).kind, "hidden");
  assert.equal(eyeState(first.vis, "normal", text("title")).on, false);
  assert.deepEqual(requestLists(first.vis, "normal"), { hide: ["text:title"], show: [] });
  const back = toggleEye(first.vis, "normal", text("title"));
  assert.deepEqual(back, { vis: emptyVisibility(), described: false });
});

test("a layer the card does not draw is forced on by its eye, which asks for the layout again", () => {
  const crown = layer("crown", false);
  assert.equal(eyeState(emptyVisibility(), "normal", crown).kind, "idle");
  const forced = toggleEye(emptyVisibility(), "normal", crown);
  assert.deepEqual(forced, { vis: { hidden: [], forced: ["normal/layer:crown"] }, described: true });
  // It is drawn now, and the eye still says it is forced, since the card would not draw it
  assert.deepEqual(eyeState(forced.vis, "normal", layer("crown", true)), { kind: "forced", on: true, disabled: false, tip: "Show/Hide", label: "Stop drawing crown when its condition does not hold" });
  assert.deepEqual(requestLists(forced.vis, "normal"), { hide: [], show: ["layer:crown"] });
  const off = toggleEye(forced.vis, "normal", layer("crown", true));
  assert.deepEqual(off, { vis: emptyVisibility(), described: true });
});

test("what is not a layer and is not drawn has no eye to press", () => {
  const pt = text("pt", false);
  const state = eyeState(emptyVisibility(), "normal", pt);
  assert.deepEqual([state.kind, state.on, state.disabled], ["blocked", false, true]);
  const vis = emptyVisibility();
  assert.deepEqual(toggleEye(vis, "normal", pt), { vis, described: false });
});

test("a hidden item and a forced layer are kept for the layout they are in", () => {
  let vis = toggleEye(emptyVisibility(), "front", layer("frame")).vis;
  vis = toggleEye(vis, "back", layer("frame")).vis;
  vis = toggleEye(vis, "front", layer("badge", false)).vis;
  assert.deepEqual(requestLists(vis, "front"), { hide: ["layer:frame"], show: ["layer:badge"] });
  assert.deepEqual(requestLists(vis, "back"), { hide: ["layer:frame"], show: [] });
  assert.deepEqual(requestLists(vis, "other"), { hide: [], show: [] });
  assert.deepEqual([...hiddenRefs(vis, "front")], ["layer:frame"]);
  assert.equal(eyeState(vis, "other", layer("frame")).kind, "shown");
  // A layout named as the start of another's is not mixed with it
  assert.deepEqual(requestLists(toggleEye(emptyVisibility(), "normal-2", layer("x")).vis, "normal"), { hide: [], show: [] });
});

test("only a layer can be forced, so a text box never goes in the request's show list", () => {
  const vis = { hidden: [], forced: ["normal/text:title", "normal/layer:crown"] };
  assert.deepEqual(requestLists(vis, "normal").show, ["layer:crown"]);
});

test("a state is not changed by what is read from it", () => {
  const vis = emptyVisibility();
  toggleEye(vis, "normal", layer("frame"));
  assert.deepEqual(vis, emptyVisibility());
});

import { register } from "node:module";
import test, { beforeEach } from "node:test";
import assert from "node:assert/strict";

register("./loader.mjs", import.meta.url);
const { app } = await import("../js/state.js");
const { describeShape, faceSupport, unsupportedBy, partlyUnsupportedBy, faceUnsupportedBy } = await import("../js/supports.js");

const single = { role: "single", kind: "standard" };
const walker = { role: "single", kind: "planeswalker" };
const front = { role: "transform_front", kind: "standard" };
const back = { role: "transform_back", kind: "standard" };

const withFaces = (faces) => (app.template.value = { name: "normal", faces });

beforeEach(() => {
  app.template.value = null;
});

test("shapes read the way a person would say them", () => {
  assert.equal(describeShape(single), "standard cards");
  assert.equal(describeShape(walker), "planeswalker cards");
  assert.equal(describeShape({ role: "split", kind: "standard" }), "split cards");
  assert.equal(describeShape({ role: "adventure", kind: "standard" }), "adventure cards");
  assert.equal(describeShape(front), "transform front faces");
  assert.equal(describeShape({ role: "mdfc_back", kind: "standard" }), "MDFC back faces");
  assert.equal(describeShape({ role: "saga_front", kind: "saga" }), "saga saga front faces");
  assert.equal(describeShape({ role: "unknown", kind: "standard" }), "cards with this layout");
  assert.equal(describeShape({ role: "single", kind: "double_faced_token" }), "double faced token cards");
});

test("a card without shapes, or a template without faces, is left to the engine", () => {
  assert.deepEqual(faceSupport({ shapes: [] }), { total: 1, renderable: 1, missing: [] });
  assert.deepEqual(faceSupport(null), { total: 1, renderable: 1, missing: [] });
  assert.deepEqual(faceSupport({ shapes: [single, walker] }), { total: 2, renderable: 2, missing: [] });
  assert.equal(unsupportedBy({ shapes: [walker] }), "");
});

test("faces the template does not list are missing", () => {
  withFaces({ "single/standard": true });
  const f = faceSupport({ shapes: [single, walker] });
  assert.equal(f.total, 2);
  assert.equal(f.renderable, 1);
  assert.deepEqual(f.missing, [walker]);
});

test("a card with no renderable face says which shape blocks it", () => {
  withFaces({ "single/standard": true });
  assert.equal(unsupportedBy({ shapes: [walker] }), "No installed template renders planeswalker cards");
  assert.equal(unsupportedBy({ shapes: [walker, single] }), "");
});

test("a card with some renderable faces says which will be skipped", () => {
  withFaces({ "transform_front/standard": true });
  const card = { shapes: [front, back] };
  assert.equal(partlyUnsupportedBy(card), "Only 1 of 2 faces will render, since no installed template renders transform back faces");
  assert.equal(unsupportedBy(card), "");
  withFaces({ "transform_front/standard": true, "transform_back/standard": true });
  assert.equal(partlyUnsupportedBy(card), "");
  withFaces({});
  assert.equal(partlyUnsupportedBy(card), "", "none renderable is unsupported, not partly");
});

test("one face reports its own reason", () => {
  withFaces({ "transform_front/standard": true });
  const card = { shapes: [front, back] };
  assert.equal(faceUnsupportedBy(card, 0), "");
  assert.equal(faceUnsupportedBy(card, 1), "No installed template renders transform back faces");
  assert.equal(faceUnsupportedBy(card, 2), "", "a face with no shape is left to the engine");
  app.template.value = null;
  assert.equal(faceUnsupportedBy(card, 1), "");
});

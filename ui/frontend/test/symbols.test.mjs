import { register } from "node:module";
import test from "node:test";
import assert from "node:assert/strict";

register("./loader.mjs", import.meta.url);
const { codes } = await import("../js/components/symbols.js");

test("braced codes are listed in order", () => {
  assert.deepEqual(codes("{2}{W}{W}"), ["2", "W", "W"]);
  assert.deepEqual(codes("{T}: Add {G}."), ["T", "G"]);
  assert.deepEqual(codes("{W/U}{2/B}{G/P}"), ["W/U", "2/B", "G/P"]);
});

test("text without codes, or nothing, lists none", () => {
  assert.deepEqual(codes("no symbols"), []);
  assert.deepEqual(codes(""), []);
  assert.deepEqual(codes(undefined), []);
  assert.deepEqual(codes(null), []);
});

test("unbalanced or nested braces are not codes", () => {
  assert.deepEqual(codes("{W"), []);
  assert.deepEqual(codes("W}"), []);
  assert.deepEqual(codes("{}"), []);
  assert.deepEqual(codes("{{W}}"), ["W"]);
});

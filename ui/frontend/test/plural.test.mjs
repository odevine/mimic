import test from "node:test";
import assert from "node:assert/strict";

const { plural } = await import("../js/plural.js");

test("plural picks the noun that agrees with the count", () => {
  assert.equal(plural(0, "row"), "0 rows");
  assert.equal(plural(1, "row"), "1 row");
  assert.equal(plural(2, "row"), "2 rows");
});

test("plural takes an irregular plural", () => {
  assert.equal(plural(1, "checked row", "checked rows"), "1 checked row");
  assert.equal(plural(3, "copy", "copies"), "3 copies");
});

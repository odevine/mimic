import { register } from "node:module";
import test from "node:test";
import assert from "node:assert/strict";

// The editor reaches the app through api.js, which imports the Wails runtime
register("./loader.mjs", import.meta.url);
const { valuesOf, isDirty, normColors, isSplit, ALL_FIELDS, FIELDS } = await import("../js/components/cardEditor.js");
const { FIELD_LABELS } = await import("../js/components/fieldLabels.js");

const elf = { Name: "Llanowar Elves", TypeLine: "Creature — Elf Druid", ManaCost: "{G}", Power: "1", Toughness: "1", Colors: ["G"], OracleText: "{T}: Add {G}." };

test("a card reads into the form's strings", () => {
  const v = valuesOf(elf);
  assert.equal(v.name, "Llanowar Elves");
  assert.equal(v.power, "1");
  assert.equal(v.colors, "G");
  assert.equal(v.flavor, "");
  assert.equal(v.half1Name, "", "a card that is not split has empty halves");
});

test("a split card reads its halves from its faces", () => {
  const split = { Layout: "split", Name: "Fire // Ice", Faces: [{ Name: "Fire", Colors: ["R"] }, { Name: "Ice", Colors: ["U"] }] };
  assert.equal(isSplit(split), true);
  const v = valuesOf(split);
  assert.equal(v.half1Name, "Fire");
  assert.equal(v.half2Colors, "U");
});

test("colors compare as sets, other fields as text", () => {
  assert.equal(normColors("ugw"), "WUG");
  assert.equal(isDirty("colors", { colors: "GW" }, { colors: "WG" }), false);
  assert.equal(isDirty("colors", { colors: "W" }, { colors: "WG" }), true);
  assert.equal(isDirty("power", { power: "2" }, { power: "1" }), true);
  assert.equal(isDirty("power", { power: "1" }, { power: "1" }), false);
});

test("every editable field has a name the rules and bulk edit can show", () => {
  for (const { f } of ALL_FIELDS) assert.ok(FIELD_LABELS[f], `${f} has no label`);
  assert.ok(FIELDS.length > 10);
});

import test from "node:test";
import assert from "node:assert/strict";

const { poolFont, roleChoices, pickedValue, mergeOverrides, pickedState, ownFont, folderNote } = await import("../js/fontmodel.js");

const pool = [
  { name: "Gotham Medium", family: "Gotham", style: "Medium", file: "G.otf", removable: true },
  { name: "Plantin MT Pro Italic", family: "Plantin MT Pro", style: "Italic", file: "P.ttf", removable: true },
];
const title = { role: "title", label: "Card name", own: "Beleren Proxy Bold", source: "template" };

test("a font is found by name whatever its capitals and spacing", () => {
  assert.equal(poolFont(pool, "  gotham   MEDIUM ").file, "G.otf");
  assert.equal(poolFont(pool, "Gotham Bold"), null);
  assert.equal(poolFont(null, "Gotham Medium"), null);
});

test("a picker offers the role's own font first and then every font in the folder", () => {
  const choices = roleChoices(title, pool);
  assert.deepEqual(
    choices.map((c) => c.value),
    ["", "Gotham Medium", "Plantin MT Pro Italic"],
  );
  assert.equal(choices[0].label, "Beleren Proxy Bold (template default)");
});

test("an assigned font the folder lacks stays in the picker, marked", () => {
  const role = { ...title, override: "Lost Font Regular", source: "template-missing-override" };
  const choices = roleChoices(role, pool);
  assert.deepEqual(choices.at(-1), { value: "Lost Font Regular", label: "Lost Font Regular (not in the folder)", missing: true });
  assert.equal(pickedValue(role, pool), "Lost Font Regular");
});

test("a picked value overrides the saved assignment in the picker", () => {
  const role = { ...title, override: "Gotham Medium" };
  assert.equal(pickedValue(role, pool), "Gotham Medium");
  assert.equal(pickedValue(role, pool, ""), "");
  assert.equal(pickedValue(role, pool, "plantin mt pro italic"), "Plantin MT Pro Italic");
  assert.equal(roleChoices(role, pool, "").some((c) => c.missing), false);
});

test("the folder's own spelling is selected for a saved name in other capitals", () => {
  assert.equal(pickedValue({ ...title, override: "gotham medium" }, pool), "Gotham Medium");
});

test("merging changes only the roles that have a picker", () => {
  const saved = { title: "Gotham Medium", body: "Plantin MT Pro Italic", "saga-chapter": "Gotham Medium" };
  assert.deepEqual(mergeOverrides(saved, { title: "", body: "Gotham Medium", info: "Gotham Medium" }), {
    body: "Gotham Medium",
    "saga-chapter": "Gotham Medium",
    info: "Gotham Medium",
  });
  assert.deepEqual(mergeOverrides(saved, null), saved);
  assert.deepEqual(mergeOverrides(undefined, { title: "Gotham Medium" }), { title: "Gotham Medium" });
});

test("merging leaves the saved assignments alone", () => {
  const saved = { title: "Gotham Medium" };
  mergeOverrides(saved, { title: "" });
  assert.deepEqual(saved, { title: "Gotham Medium" });
});

test("a role says what it draws with for the font its picker shows", () => {
  assert.deepEqual(pickedState(title, pool, ""), { text: "Built in", tone: "" });
  assert.deepEqual(pickedState({ ...title, bundled: true }, pool, ""), { text: "Shipped with the template", tone: "" });
  assert.deepEqual(pickedState(title, pool, "gotham medium"), { text: "Your font, G.otf", tone: "ok" });
  assert.equal(pickedState(title, pool, "Lost").tone, "warn");
  assert.match(pickedState(title, pool, "Lost").text, /^Lost is not in the folder/);
  assert.equal(pickedState({ ...title, source: "fallback" }, pool, "").tone, "warn");
});

test("a role's own font reads as its name, or its family and style", () => {
  assert.equal(ownFont(title), "Beleren Proxy Bold");
  assert.equal(ownFont({ family: "Gotham", style: "Medium" }), "Gotham Medium");
  assert.equal(ownFont({}), "Unknown font");
});

test("the folder note says where fonts come from", () => {
  assert.match(folderNote("managed"), /kept in the app's fonts folder/);
  assert.match(folderNote("checkout", "../local-fonts"), /\.\.\/local-fonts from the checkout/);
  assert.match(folderNote("flag", "/f"), /-fonts flag/);
  assert.match(folderNote("none"), /no config folder/);
});

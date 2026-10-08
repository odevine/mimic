// What each editable card field is called wherever one is picked from a list:
// the override rules, and the bulk edit of list rows. The keys are the names the
// app carries fields by

export const FIELD_LABELS = {
  name: "Name",
  manaCost: "Mana cost",
  colors: "Colors",
  typeLine: "Type line",
  oracle: "Rules text",
  flavor: "Flavor text",
  power: "Power",
  toughness: "Toughness",
  loyalty: "Loyalty",
  artist: "Artist",
  setCode: "Set code",
  collector: "Collector number",
  rarity: "Rarity",
  released: "Release date",
  language: "Language",
};
for (const n of [1, 2]) {
  const half = n === 1 ? "First half" : "Second half";
  Object.assign(FIELD_LABELS, {
    [`half${n}Name`]: `${half} name`,
    [`half${n}ManaCost`]: `${half} cost`,
    [`half${n}Colors`]: `${half} colors`,
    [`half${n}TypeLine`]: `${half} type`,
    [`half${n}Oracle`]: `${half} rules`,
    [`half${n}Flavor`]: `${half} flavor`,
  });
}

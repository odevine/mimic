// Card objects as the app sends them, for replies a page test sets
export function card(name, extra = {}) {
  return {
    Name: name,
    ManaCost: "{1}",
    TypeLine: "Artifact",
    OracleText: "",
    FlavorText: "",
    Power: "",
    Toughness: "",
    Loyalty: "",
    Colors: [],
    ColorIdentity: [],
    Rarity: "common",
    CollectorNumber: "1",
    SetCode: "c21",
    Artist: "",
    Language: "en",
    ReleasedAt: "2021-04-23",
    ArtworkURL: "",
    Layout: "normal",
    shapes: [{ role: "single", kind: "standard" }],
    ...extra,
  };
}

// A search result is a line of text and the card it stands for
export const result = (name, extra) => ({ text: name, card: card(name, extra) });

// A render job's events, ending in success or in an error
export const progress = (seq, step, frac) => ({ seq, step, frac });
export const done = (seq, extra = {}) => ({ seq, frac: 1, done: true, ...extra });

// A resolved list row, as the resolver reports it
export const matched = (index, name, extra) => ({ index, status: "matched", card: card(name, extra) });

// What the Fonts setting reports with a template installed: the standard roles,
// each on its own built-in font, and nothing in the folder
const standardRole = (role, label, family, style) => ({ role, label, source: "template", family, style, own: `${family} ${style}` });
export const installedFonts = {
  kind: "managed",
  path: "/fonts",
  writable: true,
  roles: [
    standardRole("title", "Card name, type line and power/toughness", "Beleren Proxy", "Bold"),
    standardRole("body", "Rules text", "Plantin MT Pro", "Regular"),
    standardRole("body-italic", "Flavor text", "Plantin MT Pro", "Italic"),
    standardRole("mana", "Mana and other card symbols", "Mana", "Regular"),
    standardRole("small-caps", "Artist credit", "Beleren Small Caps", "Bold"),
    standardRole("info", "Collector number and set code", "Gotham", "Medium"),
  ],
  pool: [],
  rejected: [],
  duplicates: [],
};

// An installed template, as the app reports the active one when a bundle is in use
export const installedTemplate = {
  name: "mimic.m15",
  version: "0.1.0",
  label: "Modern frame (M15) · 0.1.0",
  source: "cached",
  shapes: [{ role: "single", kind: "standard" }],
  faces: { "single/standard": "mimic.m15" },
};

// A template the editor holds, and what the editor is told when it opens one
export const draftSummary = (id, name, extra = {}) => ({ id, templateId: `user.${id}`, name, updated: "2026-10-12T09:00:00Z", ...extra });
export const draftView = (draft, extra = {}) => ({
  draft,
  meta: {},
  layouts: [{ name: "normal", width: 2176, height: 2960, dpi: 800, serves: [{ role: "single", kind: "standard" }] }],
  diagnostics: [],
  valid: true,
  ...extra,
});

// What a draft's layout draws for the test card, in draw order. Places are in the
// finished image's pixels, and a thing that is drawn has the same place in the
// layout's own canvas unless a test says the card is turned
export const at = (x, y, width, height) => ({ x, y, width, height });
export const drawn = (kind, key, rect, extra = {}) => ({ kind, key, drawn: true, authored: rect, delivered: rect, ...extra });
export const standardItems = [
  drawn("layer", "background", at(0, 0, 1000, 1400), { part: "background" }),
  drawn("layer", "frame", at(50, 50, 900, 1300), { part: "frame" }),
  drawn("art", "0", at(80, 200, 840, 600), { after: "frame" }),
  drawn("text", "title", at(100, 80, 600, 80)),
  // A text box set on its baseline is described by where its text is
  drawn("text", "type", at(100, 790, 360, 40), { ink: true }),
  drawn("text", "oracle", at(100, 900, 800, 300)),
  { kind: "text", key: "pt", drawn: false, why: "the card has no pt to draw", authored: at(780, 1250, 120, 80), delivered: at(780, 1250, 120, 80) },
];
export const describeView = (items = standardItems, extra = {}) => ({
  layout: "normal",
  authored: { width: 1000, height: 1400 },
  delivered: { width: 1000, height: 1400 },
  items,
  editable: true,
  rev: 0,
  ...extra,
});

// The files behind standardItems, as a draft keeps them. A test changes the objects and
// the bridge writes them out as the app would send them
export const standardLayout = () => ({
  serves: [{ role: "single", kind: "standard" }],
  canvas: { width: 1000, height: 1400, dpi: 800 },
  stack: [
    { id: "background", part: "background" },
    { id: "frame", part: "frame", condition: "nonland" },
  ],
  textBoxes: {
    title: { rect: { x: 100, y: 80, width: 600, height: 80 }, text: { fontSize: 60, font: "title", color: "#000000", align: "left", vAlign: "top" } },
    type: { draws: "type", rect: { x: 100, y: 820, width: 800, height: 60 }, text: { fontSize: 40, font: "title", align: "left", vAlign: "baseline" } },
    oracle: { rect: { x: 100, y: 900, width: 800, height: 300 }, text: { fontSize: 36, lineSpacing: 1, paddingX: 20, vAlign: "center" } },
    pt: { rect: { x: 780, y: 1250, width: 120, height: 80 }, text: { fontSize: 50, align: "center" } },
  },
  art: [{ x: 80, y: 200, width: 840, height: 600, after: "frame" }],
  symbols: { set: { x: 800, y: 600, width: 100, height: 60, align: "right" } },
});
export const standardTemplateDoc = () => ({
  contract: "1.1",
  id: "user.mine",
  version: "0.1.0",
  name: "Mine",
  parts: { background: {}, frame: { slot: "background", origin: { x: 10, y: 20 } } },
  credits: [{ name: "Original Artist", url: "https://example.com/art", note: "art" }, { name: "Mine too" }],
});

// The cards the presets name, as the app looks them up, by preset id
const art = (name) => `https://cards.scryfall.io/art_crop/front/${name}.jpg`;
const single = [{ role: "single", kind: "standard" }];
export const presetCards = {
  monocolored: card("Mirran Spy", { ManaCost: "{1}{U}", TypeLine: "Creature — Human Rogue", OracleText: "Flying\nWhenever you cast an artifact spell, you may untap target creature.", Power: "1", Toughness: "1", Colors: ["U"], ColorIdentity: ["U"], SetCode: "mbs", CollectorNumber: "26", Rarity: "common", ArtworkURL: art("mirran-spy"), shapes: single }),
  "two-colors": card("Consuming Aberration", { ManaCost: "{3}{U}{B}", TypeLine: "Creature — Horror", OracleText: "Consuming Aberration's power and toughness are each equal to the number of cards in your opponents' graveyards.", Power: "*", Toughness: "*", Colors: ["U", "B"], ColorIdentity: ["U", "B"], SetCode: "gtc", CollectorNumber: "152", Rarity: "rare", ArtworkURL: art("consuming-aberration"), shapes: single }),
  "three-colors": card("Eerie Ultimatum", { ManaCost: "{W}{W}{B}{B}{G}{G}", TypeLine: "Sorcery", OracleText: "Return any number of permanent cards with different names from your graveyard to the battlefield.", Colors: ["W", "B", "G"], ColorIdentity: ["W", "B", "G"], SetCode: "iko", CollectorNumber: "184", Rarity: "rare", ArtworkURL: art("eerie-ultimatum"), shapes: single }),
  legendary: card("Norin the Wary", { ManaCost: "{R}", TypeLine: "Legendary Creature — Human Warrior", OracleText: "When a player casts a spell or a creature attacks, exile Norin the Wary.", Power: "1", Toughness: "1", Colors: ["R"], ColorIdentity: ["R"], SetCode: "tsp", CollectorNumber: "171", Rarity: "rare", ArtworkURL: art("norin-the-wary"), shapes: single }),
  artifact: card("Sensei's Divining Top", { ManaCost: "{1}", TypeLine: "Artifact", OracleText: "{1}: Look at the top three cards of your library, then put them back in any order.", SetCode: "2x2", CollectorNumber: "314", Rarity: "rare", ArtworkURL: art("sensei-s-divining-top"), shapes: single }),
  land: card("Flooded Strand", { ManaCost: "", TypeLine: "Land", OracleText: "{T}, Pay 1 life, Sacrifice Flooded Strand: Search your library for a Plains or Island card.", SetCode: "ktk", CollectorNumber: "233", Rarity: "rare", ArtworkURL: art("flooded-strand"), shapes: single }),
};

// What a draft holds in images and fonts, as the app reports it. A part has a cell for
// each material its slot reads: an image of its own, a run blended from its colors, the
// any image's cover, or nothing, which leaves a hole
const LETTERS = "wubrg";
export const slotKeys = (slot) => {
  if (!slot) return ["any"];
  const keys = ["any", ...LETTERS];
  for (let i = 0; i < LETTERS.length; i++) for (let j = i + 1; j < LETTERS.length; j++) keys.push(LETTERS[i] + LETTERS[j]);
  return [...keys, "gold", "artifact", "colorless", "land", "vehicle"];
};
export const partView = (name, slot, images = [], extra = {}) => {
  const has = (k) => images.includes(k);
  const materials = slotKeys(slot).map((key) => {
    if (has(key)) return { key, state: "image", file: `parts/${name}/${key}.png`, width: 1000, height: 1400, bytes: 120000 };
    if (key.length === 2 && [...key].every((c) => LETTERS.includes(c) && has(c))) return { key, state: "blended", from: [...key] };
    if (has("any") && key !== "any") return { key, state: "any" };
    return { key, state: "missing" };
  });
  return { name, ...(slot ? { slot } : {}), dir: `parts/${name}`, materials, ...extra };
};
const ROLES = [
  ["title", "Card name, type line and power/toughness", "mimic-m15-title"],
  ["body", "Rules text", "mimic-m15-body"],
  ["body-italic", "Flavor text", "mimic-m15-body-italic"],
  ["mana", "Mana and other card symbols", "mimic-m15-mana"],
  ["small-caps", "Artist credit", "mimic-m15-small-caps"],
  ["info", "Collector number and set code", "mimic-m15-info"],
];
export const assetsView = (parts, extra = {}) => ({
  parts,
  fonts: [],
  roles: ROLES.map(([role, label, def]) => ({ role, label, standard: true, default: def })),
  builtins: ROLES.map(([, , id]) => ({ id })),
  slots: ["", "background", "pinlines", "twins", "ptBox", "crown", "indicator", "transform_icon", "fuse", "fuse_pinlines"].map((name) => ({ name, materials: slotKeys(name) })),
  ...extra,
});
export const standardAssets = () => assetsView([partView("background", "", ["any"]), partView("frame", "background", [...LETTERS])]);
// A one pixel image to stand for a thumbnail
export const PIXEL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==";

// source is a draft's files the bridge sends, kept as objects so a test can change them
export class Source {
  constructor(layout = standardLayout(), template = standardTemplateDoc()) {
    this.layout = layout;
    this.template = template;
    this.extra = {};
  }
  files() {
    const out = [
      { name: "template.json", content: JSON.stringify(this.template, null, 2) + "\n" },
      { name: "layouts/normal.json", content: JSON.stringify(this.layout, null, 2) + "\n" },
    ];
    return { files: out.map((f) => ({ ...f, content: this.extra[f.name] ?? f.content })) };
  }
}

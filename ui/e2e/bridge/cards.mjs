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

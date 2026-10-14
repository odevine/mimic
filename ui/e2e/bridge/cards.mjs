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
  drawn("text", "type", at(100, 820, 800, 60)),
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

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

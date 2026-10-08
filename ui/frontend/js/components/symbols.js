import { api } from "../api.js";
import { h } from "../dom.js";

// Mana-aware inputs. Braced codes in a field render as the engine's own pips
// beneath it, and a code the engine does not draw is flagged in place. The
// palette inserts codes for anyone who does not know the syntax

const CODE = /\{([^{}]+)\}/g;

// codes lists the braced codes in text, in order
export function codes(text) {
  return [...(text || "").matchAll(CODE)].map((m) => m[1]);
}

// known caches whether the engine draws each code, answered by whether its
// pip image loads, so a flagged code never flickers between states
const known = new Map();

function probe(code) {
  if (!known.has(code)) {
    known.set(
      code,
      new Promise((resolve) => {
        const img = new Image();
        img.onload = () => resolve(true);
        img.onerror = () => resolve(false);
        img.src = api.symbolURL(code);
      }),
    );
  }
  return known.get(code);
}

function pip(code, ok) {
  if (ok) return h("img", { src: api.symbolURL(code), alt: `{${code}}`, title: `{${code}}` });
  return h("span", { class: "unknown", title: "The engine does not draw this symbol, so it prints as typed" }, `{${code}}`);
}

// renderPips fills el with the pips for text. distinct collapses repeats, for
// rules text where the same symbol recurs. A later call supersedes an earlier
// one still resolving
export async function renderPips(el, text, { distinct = false, label = "" } = {}) {
  let list = codes(text);
  if (distinct) list = [...new Set(list)];
  const token = (el._pipToken = {});
  const results = await Promise.all(list.map(probe));
  if (el._pipToken !== token) return;

  el.replaceChildren();
  if (!list.length) return;
  if (label) el.append(h("span", { class: "pips-label" }, label));
  list.forEach((c, i) => el.append(pip(c, results[i])));
  const unknown = results.filter((ok) => !ok).length;
  el.dataset.unknown = String(unknown);
}

const PALETTE = [
  ["Colors", ["W", "U", "B", "R", "G", "C"]],
  ["Generic", ["0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "X"]],
  ["Hybrid", ["W/U", "U/B", "B/R", "R/G", "G/W", "W/B", "U/R", "B/G", "R/W", "G/U"]],
  ["Twobrid and Phyrexian", ["2/W", "2/U", "2/B", "2/R", "2/G", "W/P", "U/P", "B/P", "R/P", "G/P"]],
  ["Other", ["T", "Q", "S"]],
];

// symbolPalette builds the palette content. onPick receives the braced code
export function symbolPalette(onPick) {
  const grid = h("div", { class: "symbol-palette", role: "group", "aria-label": "Mana symbols" });
  for (const [heading, list] of PALETTE) {
    grid.append(h("h4", {}, heading));
    for (const code of list) {
      grid.append(
        h(
          "button",
          { type: "button", title: `{${code}}`, onclick: () => onPick(`{${code}}`) },
          h("img", { src: api.symbolURL(code), alt: `{${code}}` }),
        ),
      );
    }
  }
  grid.append(h("p", { class: "hint" }, "Codes are typed in braces, like {2}{U}{U}."));
  return grid;
}

// insertAtCursor puts text into an input or textarea at the caret and fires
// input so listeners see the change
export function insertAtCursor(el, text) {
  const start = el.selectionStart ?? el.value.length;
  const end = el.selectionEnd ?? el.value.length;
  el.setRangeText(text, start, end, "end");
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.focus();
}

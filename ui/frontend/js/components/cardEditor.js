import { h, icon } from "../dom.js";
import { effect, signal } from "../state.js";
import { openPopover } from "./popover.js";
import { renderPips, symbolPalette, insertAtCursor } from "./symbols.js";

// The card field editor, shared by Single and the list inspector. It edits a
// map of field values held in a signal and knows nothing about what the values
// are for: the page that mounts it decides when to render and what a revert
// returns to

// FIELDS pairs each editable field with its card.Data key. kind picks the
// control: text, area for multi-line, mana for a cost with pips, rules for
// rules text with pips, and colors for the WUBRG toggles
export const FIELDS = [
  { f: "name", key: "Name", label: "Name", kind: "text" },
  { f: "manaCost", key: "ManaCost", label: "Cost", kind: "mana", placeholder: "{2}{U}{U}" },
  { f: "colors", key: "Colors", label: "Colors", kind: "colors" },
  { f: "typeLine", key: "TypeLine", label: "Type", kind: "text" },
  { f: "oracle", key: "OracleText", label: "Rules", kind: "rules", rows: 6 },
  { f: "flavor", key: "FlavorText", label: "Flavor", kind: "area", rows: 3 },
  { f: "power", key: "Power", label: "P / T", kind: "text", group: "pt" },
  { f: "toughness", key: "Toughness", label: "", kind: "text", group: "pt" },
  { f: "loyalty", key: "Loyalty", label: "Loyalty", kind: "text", group: "pt" },
  { f: "artist", key: "Artist", label: "Artist", kind: "text" },
  { f: "setCode", key: "SetCode", label: "Set", kind: "text", group: "details" },
  { f: "collector", key: "CollectorNumber", label: "Collector #", kind: "text", group: "details" },
  { f: "rarity", key: "Rarity", label: "Rarity", kind: "text", group: "details" },
  { f: "released", key: "ReleasedAt", label: "Released", kind: "text", group: "details", placeholder: "YYYY-MM-DD" },
  { f: "language", key: "Language", label: "Language", kind: "text", group: "details" },
];

// HALF_FIELDS are the fields of each half of a split card, read from its faces
// and edited under the same names the app's Edits carries. A split card
// draws only its halves, so these stand in for the top-level face fields
export const HALF_FIELDS = [1, 2].flatMap((n) => [
  { f: `half${n}Name`, key: "Name", half: n, label: "Name", kind: "text" },
  { f: `half${n}ManaCost`, key: "ManaCost", half: n, label: "Cost", kind: "mana", placeholder: "{2}{U}{U}" },
  { f: `half${n}Colors`, key: "Colors", half: n, label: "Colors", kind: "colors" },
  { f: `half${n}TypeLine`, key: "TypeLine", half: n, label: "Type", kind: "text" },
  { f: `half${n}Oracle`, key: "OracleText", half: n, label: "Rules", kind: "rules", rows: 5 },
  { f: `half${n}Flavor`, key: "FlavorText", half: n, label: "Flavor", kind: "area", rows: 2 },
]);
export const ALL_FIELDS = [...FIELDS, ...HALF_FIELDS];

// FACE_FIELDS are the top-level fields a split card does not draw, which its
// halves replace
export const FACE_FIELDS = new Set(["name", "manaCost", "colors", "typeLine", "oracle", "flavor", "power", "toughness", "loyalty"]);

export const isSplit = (card) => !!card && card.Layout === "split" && (card.Faces || []).length >= 2;

const COLORS = [
  ["W", "White", "--mana-w"],
  ["U", "Blue", "--mana-u"],
  ["B", "Black", "--mana-b"],
  ["R", "Red", "--mana-r"],
  ["G", "Green", "--mana-g"],
];
const COLOR_ORDER = "WUBRGC";

export const normColors = (s) =>
  [...new Set((s || "").toUpperCase())].filter((c) => COLOR_ORDER.includes(c)).sort((a, b) => COLOR_ORDER.indexOf(a) - COLOR_ORDER.indexOf(b)).join("");

// valuesOf reads a card into the form's string values. A half's fields read from
// its face, and are empty for a card that is not split
export function valuesOf(card) {
  const out = {};
  for (const { f, key, half } of ALL_FIELDS) {
    const src = half ? (isSplit(card) ? card.Faces[half - 1] : {}) : card;
    out[f] = key === "Colors" ? normColors((src.Colors || []).join("")) : src[key] || "";
  }
  return out;
}

export const isColorField = (f) => f === "colors" || /^half\d+Colors$/.test(f);
export const isDirty = (f, edits, base) => (isColorField(f) ? normColors(edits[f]) !== normColors(base[f]) : edits[f] !== base[f]);

// createCardEditor makes an editor over signals the caller owns. base holds the
// card, which says whether it is split, and edits the field values as strings.
// reference holds the values an edit is measured against and a revert returns
// to, and ruled, when given, the fields a rule set. onEnter runs when the user
// presses Enter in a one-line field or submits the form. mount fills a form
// element with the fields, and dirty says whether any field differs
export function createCardEditor(ctx) {
  ctx.setEdit = (f, value) => {
    ctx.edits.value = { ...ctx.edits.peek(), [f]: value };
  };
  const dirty = signal(false);

  function colorToggles(f) {
    const wrap = h("div", { class: "color-toggles", role: "group", "aria-label": "Colors" });
    const chips = {};
    for (const [c, name, tok] of COLORS) {
      chips[c] = h(
        "button",
        {
          type: "button",
          class: "chip color-chip",
          style: `--swatch: var(${tok})`,
          "aria-pressed": "false",
          "aria-label": name,
          onclick: () => {
            let cur = normColors(ctx.edits.peek()[f]).replace("C", "");
            cur = cur.includes(c) ? cur.replace(c, "") : cur + c;
            ctx.setEdit(f, normColors(cur));
          },
        },
        h("span", { class: "swatch" }),
        c,
      );
      wrap.append(chips[c]);
    }
    chips.C = h(
      "button",
      {
        type: "button",
        class: "chip color-chip",
        style: "--swatch: var(--mana-c)",
        "aria-pressed": "false",
        "aria-label": "Colorless",
        onclick: () => ctx.setEdit(f, normColors(ctx.edits.peek()[f]) === "C" ? "" : "C"),
      },
      h("span", { class: "swatch" }),
      "Colorless",
    );
    wrap.append(chips.C);
    effect(() => {
      const e = ctx.edits.value;
      if (!e) return;
      const v = normColors(e[f]);
      for (const [c, el] of Object.entries(chips)) el.setAttribute("aria-pressed", String(v.includes(c)));
    });
    return [wrap, h("p", { class: "note" }, "Colors pick the frame. Lands take theirs from the mana they produce.")];
  }

  function paletteButton(input) {
    return h(
      "button",
      {
        type: "button",
        class: "btn subtle icon-only",
        "aria-label": "Insert a mana symbol",
        "data-tip": "Insert a mana symbol",
        onclick: (e) => openPopover(e.currentTarget, symbolPalette((code) => insertAtCursor(input, code)), { align: "end" }),
      },
      icon("symbols"),
    );
  }

  function buildField(spec) {
    const id = `f-${spec.f}`;
    const revert = h(
      "button",
      {
        type: "button",
        class: "revert",
        "aria-label": `Revert ${spec.label || spec.f}`,
        "data-tip": "Revert to the fetched card",
        onclick: () => ctx.setEdit(spec.f, ctx.reference.peek()[spec.f]),
      },
      icon("revert"),
    );
    const label = h(spec.kind === "colors" ? "span" : "label", { class: spec.kind === "colors" ? "label" : null, for: spec.kind === "colors" ? null : id }, h("span", { class: "dirty-dot" }), revert, spec.label);
    const control = h("div", { class: "control" });
    const el = h("div", { class: "field", dataset: { field: spec.f } }, label, control);

    if (spec.kind === "colors") {
      control.append(...colorToggles(spec.f));
      return el;
    }

    const multi = spec.kind === "area" || spec.kind === "rules";
    const input = h(multi ? "textarea" : "input", {
      id,
      rows: spec.rows,
      placeholder: spec.placeholder || "",
      spellcheck: multi ? "true" : "false",
      class: spec.kind === "mana" ? "mono" : null,
      oninput: (e) => ctx.setEdit(spec.f, e.target.value),
    });
    if (!multi) {
      // Enter in a single-line field renders, multi-line fields keep it for newlines
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter" && !e.isComposing) {
          e.preventDefault();
          ctx.onEnter();
        }
      });
    }

    if (spec.kind === "mana" || spec.kind === "rules") {
      control.append(h("div", { class: "control-row" }, input, paletteButton(input)));
      const pips = h("div", { class: "pips", "aria-live": "polite" });
      control.append(pips);
      effect(() => {
        const e = ctx.edits.value;
        if (e) renderPips(pips, e[spec.f], { distinct: spec.kind === "rules", label: spec.kind === "rules" ? "Symbols" : "" });
      });
    } else {
      control.append(input);
    }

    // Keep the input in step with the store without fighting the caret while typing
    effect(() => {
      const e = ctx.edits.value;
      if (e && input.value !== e[spec.f]) input.value = e[spec.f];
    });
    return el;
  }

  function mount(form) {
    const main = FIELDS.filter((s) => !s.group);
    const pt = FIELDS.filter((s) => s.group === "pt");
    const details = FIELDS.filter((s) => s.group === "details");

    const grid = h("div", { class: "field-grid" });
    for (const s of main) {
      grid.append(buildField(s));
      if (s.f === "flavor") grid.append(h("div", { class: "field-pair" }, ...pt.map(buildField)));
    }
    const more = h(
      "details",
      { class: "disclosure" },
      h("summary", {}, icon("chevron-down"), "Printing details"),
      h("div", { class: "field-grid" }, ...details.map(buildField)),
    );
    // A split card draws its two halves, so their fields replace the face fields
    // of the whole card, which would edit nothing it prints
    const halves = [1, 2].map((n) => {
      const title = h("h3", { class: "half-title" });
      effect(() => {
        const e = ctx.edits.value;
        const name = e && e[`half${n}Name`];
        title.textContent = `${n === 1 ? "First" : "Second"} half${name ? ` · ${name}` : ""}`;
      });
      return h(
        "section",
        { class: "half-fields", hidden: true, "aria-label": `${n === 1 ? "First" : "Second"} half` },
        title,
        h("div", { class: "field-grid" }, ...HALF_FIELDS.filter((s) => s.half === n).map(buildField)),
      );
    });
    const faceEls = [...grid.children].filter((el) => el.classList.contains("field-pair") || FACE_FIELDS.has(el.dataset.field));
    effect(() => {
      const split = isSplit(ctx.base.value);
      for (const el of faceEls) el.hidden = split;
      for (const el of halves) el.hidden = !split;
    });
    form.replaceChildren(...halves, grid, more);
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      ctx.onEnter();
    });

    // Dirty and rule marks, one effect for the whole form since every field reads
    // edits. A field is dirty when it differs from the reference, which is the
    // fetched card with any rule's changes laid on it, and ruled when it holds what
    // a rule set and nothing since
    effect(() => {
      const e = ctx.edits.value;
      const ref = ctx.reference.value;
      const ruled = ctx.ruled ? ctx.ruled.value : {};
      if (!e || !ctx.base.value) return;
      let any = false;
      for (const el of form.querySelectorAll(".field[data-field]")) {
        const f = el.dataset.field;
        const isDirtyField = isDirty(f, e, ref);
        el.classList.toggle("dirty", isDirtyField);
        el.classList.toggle("ruled", !isDirtyField && f in ruled);
        any ||= isDirtyField;
      }
      dirty.value = any;
    });
  }

  return { mount, dirty };
}

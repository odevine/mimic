import { api } from "../api.js";
import { $, h, icon } from "../dom.js";
import { app, signal, effect, batch } from "../state.js";
import { openPopover, closePopover } from "../components/popover.js";
import { renderPips, symbolPalette, insertAtCursor } from "../components/symbols.js";
import { setZoom } from "../components/preview.js";
import { faceUnsupportedBy } from "../supports.js";

// Flow 1: search a card, adjust anything about it, render, download. The
// browser holds the fetched card as the base and the form values as edits, so
// every field knows whether it differs from what Scryfall returned

// FIELDS pairs each editable field with its card.Data key. kind picks the
// control: text, area for multi-line, mana for a cost with pips, rules for
// rules text with pips, and colors for the WUBRG toggles
const FIELDS = [
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
const HALF_FIELDS = [1, 2].flatMap((n) => [
  { f: `half${n}Name`, key: "Name", half: n, label: "Name", kind: "text" },
  { f: `half${n}ManaCost`, key: "ManaCost", half: n, label: "Cost", kind: "mana", placeholder: "{2}{U}{U}" },
  { f: `half${n}Colors`, key: "Colors", half: n, label: "Colors", kind: "colors" },
  { f: `half${n}TypeLine`, key: "TypeLine", half: n, label: "Type", kind: "text" },
  { f: `half${n}Oracle`, key: "OracleText", half: n, label: "Rules", kind: "rules", rows: 5 },
  { f: `half${n}Flavor`, key: "FlavorText", half: n, label: "Flavor", kind: "area", rows: 2 },
]);
const ALL_FIELDS = [...FIELDS, ...HALF_FIELDS];

// FACE_FIELDS are the top-level fields a split card does not draw, which its
// halves replace
const FACE_FIELDS = new Set(["name", "manaCost", "colors", "typeLine", "oracle", "flavor", "power", "toughness", "loyalty"]);

const isSplit = (card) => !!card && card.Layout === "split" && (card.Faces || []).length >= 2;

const COLORS = [
  ["W", "White", "--mana-w"],
  ["U", "Blue", "--mana-u"],
  ["B", "Black", "--mana-b"],
  ["R", "Red", "--mana-r"],
  ["G", "Green", "--mana-g"],
];
const COLOR_ORDER = "WUBRGC";

const normColors = (s) =>
  [...new Set((s || "").toUpperCase())].filter((c) => COLOR_ORDER.includes(c)).sort((a, b) => COLOR_ORDER.indexOf(a) - COLOR_ORDER.indexOf(b)).join("");

// valuesOf reads a card into the form's string values. A half's fields read from
// its face, and are empty for a card that is not split
function valuesOf(card) {
  const out = {};
  for (const { f, key, half } of ALL_FIELDS) {
    const src = half ? (isSplit(card) ? card.Faces[half - 1] : {}) : card;
    out[f] = key === "Colors" ? normColors((src.Colors || []).join("")) : src[key] || "";
  }
  return out;
}

const store = {
  results: signal([]), // [{ name, cards: [card...] }]
  recents: signal([]),
  noMatch: signal(null), // the last query when it matched nothing
  selected: signal(-1),
  base: signal(null),
  edits: signal(null),
  rendered: signal(null), // the edits JSON the preview on screen was rendered from
  printings: signal(null), // null while loading, [] when none
  activity: signal({ state: "idle", title: "", step: "", frac: 0 }),
  face: signal(0), // which of the card's images the preview shows, 0 for the front
};

let searchAbort = null;
let printingsAbort = null;
let renderJob = null;
let renderJobId = ""; // the app job behind renderJob
let renderSeq = 0; // counts renders started, so a slow start can tell it was superseded
let liveTimer = 0;
let preview = null; // { jobId, dpi, face } of the preview on screen

const faceCount = (card) => (card && card.shapes ? card.shapes.length : 1);

// faceTitle is the name the shown face prints. The form edits the front, so a
// back face shows its own name as fetched
function faceTitle() {
  const face = store.face.peek();
  const base = store.base.peek();
  if (face > 0 && base && base.Faces && base.Faces[face]) return base.Faces[face].Name || "card";
  return store.edits.peek().name || "card";
}
let saving = false;

const baseValues = () => (store.base.peek() ? valuesOf(store.base.peek()) : {});
const isColorField = (f) => f === "colors" || /^half\d+Colors$/.test(f);
const isDirty = (f, edits, base) => (isColorField(f) ? normColors(edits[f]) !== normColors(base[f]) : edits[f] !== base[f]);

function setEdit(f, value) {
  store.edits.value = { ...store.edits.peek(), [f]: value };
}

// --- search and results ---

function group(cards, expand) {
  if (expand) return cards.map((c) => ({ name: c.Name, cards: [c] }));
  const byName = new Map();
  for (const c of cards) {
    if (!byName.has(c.Name)) byName.set(c.Name, { name: c.Name, cards: [] });
    byName.get(c.Name).cards.push(c);
  }
  return [...byName.values()];
}

let lastCards = [];

async function runSearch(query, { focusResults = false } = {}) {
  query = query.trim();
  if (!query) return;
  if (searchAbort) searchAbort.abort();
  const ac = (searchAbort = new AbortController());
  app.status.value = `Searching for ${JSON.stringify(query)}…`;
  $("results-count").textContent = "Searching…";
  try {
    const results = await api.search(query, ac.signal);
    lastCards = results.map((r) => r.card);
    batch(() => {
      store.results.value = group(lastCards, app.settings.peek().expandPrintings);
      store.selected.value = -1;
      store.noMatch.value = lastCards.length ? null : query;
    });
    const n = store.results.peek().length;
    app.status.value = n === 0 ? `No cards match ${JSON.stringify(query)}` : `${n} ${n === 1 ? "result" : "results"}`;
    loadRecents();
    // A recent picked from the column is replaced by the results, so focus follows
    const first = $("results").querySelector(".result");
    if (focusResults && first) first.focus();
  } catch (err) {
    if (err.name !== "AbortError") {
      app.status.value = `Search failed: ${err.message}`;
      $("results-count").textContent = "Search failed";
    }
  } finally {
    if (searchAbort === ac) searchAbort = null;
  }
}

async function loadRecents() {
  try {
    store.recents.value = (await api.recents()) || [];
  } catch {
    // suggestions are a convenience
  }
}

// searchRecent fills the box with a remembered query and runs it
function searchRecent(q) {
  closePopover();
  $("search-input").value = q;
  runSearch(q, { focusResults: $("results").contains(document.activeElement) });
}

// openRecents lists remembered queries on request, for when results fill the
// column and the inline list is not showing
function openRecents() {
  const menu = h(
    "div",
    { class: "menu", role: "menu" },
    h("div", { class: "menu-heading section-heading" }, "Recent searches"),
    ...store.recents.peek().map((q) =>
      h("button", { type: "button", class: "menu-item", role: "menuitem", onclick: () => searchRecent(q) }, icon("clock"), h("span", {}, q)),
    ),
  );
  openPopover($("recents-btn"), menu, { cls: "recents-menu", align: "end" });
}

function recentRow(q) {
  return h(
    "li",
    { class: "result recent", role: "option", tabIndex: -1, dataset: { query: q }, onclick: () => searchRecent(q) },
    h("span", { class: "r-name" }, icon("clock"), h("span", {}, q)),
  );
}

// Recent searches live in the results column while it has nothing else to
// show, rather than in a popup over it, so they never cover real results
function renderResults() {
  const groups = store.results.value;
  const list = $("results");

  if (groups.length === 0) {
    // Read only on this branch, so refreshing recents never rebuilds real results
    const recents = store.recents.value;
    const noMatch = store.noMatch.value;
    const rows = [];
    if (noMatch) rows.push(h("li", { class: "results-note", role: "presentation" }, `No cards match ${JSON.stringify(noMatch)}`));
    if (recents.length) {
      rows.push(h("li", { class: "results-heading section-heading", role: "presentation" }, "Recent searches"));
      rows.push(...recents.map(recentRow));
    }
    list.replaceChildren(...rows);
    const first = list.querySelector(".result");
    if (first) first.tabIndex = 0;
    $("results-count").textContent = "Type a query and press Enter";
    return;
  }

  list.replaceChildren(
    ...groups.map((g, i) => {
      const c = g.cards[0];
      return h(
        "li",
        {
          class: "result",
          role: "option",
          "aria-selected": "false",
          tabIndex: i === 0 ? 0 : -1,
          dataset: { index: String(i) },
          onclick: () => selectResult(i),
        },
        h("span", { class: "r-name" }, g.name),
        h(
          "span",
          { class: "r-meta" },
          c.SetCode ? h("span", { class: "set" }, c.SetCode.toUpperCase()) : null,
          h("span", { class: "type" }, c.TypeLine || ""),
          g.cards.length > 1 ? h("span", { class: "prints" }, `${g.cards.length} printings`) : null,
        ),
      );
    }),
  );
  const n = groups.length;
  $("results-count").textContent = n ? `${n} ${n === 1 ? "result" : "results"}` : "Type a query and press Enter";
}

function selectResult(i) {
  const g = store.results.peek()[i];
  if (!g) return;
  const card = g.cards[0];
  batch(() => {
    store.selected.value = i;
    store.base.value = card;
    store.edits.value = valuesOf(card);
    store.face.value = 0;
  });
  loadPrintings(card.Name, g.cards.length > 1 ? g.cards : null);
  renderIfSupported();
}

// Arrow keys move through results, Enter opens the focused one
function onResultsKey(e) {
  const items = [...$("results").querySelectorAll(".result")];
  const i = items.indexOf(document.activeElement);
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    const next = items[Math.max(0, Math.min(items.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)))];
    if (next) {
      items.forEach((el) => (el.tabIndex = -1));
      next.tabIndex = 0;
      next.focus();
    }
  } else if (e.key === "Enter" && i >= 0) {
    e.preventDefault();
    const q = items[i].dataset.query;
    if (q !== undefined) searchRecent(q);
    else selectResult(i);
  }
}

// --- printings ---

const printingKey = (c) => `${c.SetCode}/${c.CollectorNumber}`;

function printingText(c) {
  const parts = [c.SetCode ? c.SetCode.toUpperCase() : "?"];
  if (c.CollectorNumber) parts.push(c.CollectorNumber);
  if (c.ReleasedAt) parts.push(c.ReleasedAt.slice(0, 4));
  return parts.join(" · ");
}

// loadPrintings lists every printing of the card. Results that already hold
// several printings, from a unique:prints search, are used as they are
async function loadPrintings(name, known) {
  if (printingsAbort) printingsAbort.abort();
  if (known) {
    store.printings.value = known;
    return;
  }
  store.printings.value = null;
  const ac = (printingsAbort = new AbortController());
  try {
    const list = await api.printings(name, ac.signal);
    if (store.base.peek() && store.base.peek().Name === name) store.printings.value = list.length ? list : [store.base.peek()];
  } catch (err) {
    if (err.name !== "AbortError") store.printings.value = [store.base.peek()];
  }
}

function openPrintings() {
  const list = store.printings.peek();
  const current = store.base.peek();
  if (!current) return;
  let content;
  if (list === null) {
    content = h("div", { class: "menu" }, h("div", { class: "menu-item", disabled: true }, "Loading printings…"));
  } else {
    content = h(
      "div",
      { class: "menu", role: "menu" },
      ...list.map((c) =>
        h(
          "button",
          {
            type: "button",
            class: "menu-item",
            role: "menuitemradio",
            "aria-checked": String(printingKey(c) === printingKey(current)),
            onclick: () => {
              closePopover();
              choosePrinting(c);
            },
          },
          h("span", { class: "set" }, (c.SetCode || "").toUpperCase()),
          h("span", {}, `#${c.CollectorNumber || "?"}`),
          h("span", { class: "dim" }, c.Rarity || ""),
          h("span", { class: "meta" }, (c.ReleasedAt || "").slice(0, 4)),
        ),
      ),
    );
  }
  openPopover($("printing-trigger"), content, { cls: "printing-menu" });
}

// choosePrinting swaps the base card. Fields the user has not edited follow the
// new printing, and edited ones keep their edit
function choosePrinting(card) {
  const oldBase = baseValues();
  const next = valuesOf(card);
  const edits = store.edits.peek();
  const merged = {};
  for (const { f } of ALL_FIELDS) merged[f] = isDirty(f, edits, oldBase) ? edits[f] : next[f];
  batch(() => {
    store.base.value = card;
    store.edits.value = merged;
    if (store.face.peek() >= faceCount(card)) store.face.value = 0;
  });
  renderIfSupported();
}

// --- editor ---

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
          let cur = normColors(store.edits.peek()[f]).replace("C", "");
          cur = cur.includes(c) ? cur.replace(c, "") : cur + c;
          setEdit(f, normColors(cur));
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
      onclick: () => setEdit(f, normColors(store.edits.peek()[f]) === "C" ? "" : "C"),
    },
    h("span", { class: "swatch" }),
    "Colorless",
  );
  wrap.append(chips.C);
  effect(() => {
    const e = store.edits.value;
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
      onclick: () => setEdit(spec.f, baseValues()[spec.f]),
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
    oninput: (e) => setEdit(spec.f, e.target.value),
  });
  if (!multi) {
    // Enter in a single-line field renders, multi-line fields keep it for newlines
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.isComposing) {
        e.preventDefault();
        render();
      }
    });
  }

  if (spec.kind === "mana" || spec.kind === "rules") {
    control.append(h("div", { class: "control-row" }, input, paletteButton(input)));
    const pips = h("div", { class: "pips", "aria-live": "polite" });
    control.append(pips);
    effect(() => {
      const e = store.edits.value;
      if (e) renderPips(pips, e[spec.f], { distinct: spec.kind === "rules", label: spec.kind === "rules" ? "Symbols" : "" });
    });
  } else {
    control.append(input);
  }

  // Keep the input in step with the store without fighting the caret while typing
  effect(() => {
    const e = store.edits.value;
    if (e && input.value !== e[spec.f]) input.value = e[spec.f];
  });
  return el;
}

function buildEditor() {
  const form = $("editor-form");
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
      const e = store.edits.value;
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
    const split = isSplit(store.base.value);
    for (const el of faceEls) el.hidden = split;
    for (const el of halves) el.hidden = !split;
  });
  form.replaceChildren(...halves, grid, more);
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    render();
  });

  // Dirty marks, one effect for the whole form since every field reads edits
  effect(() => {
    const e = store.edits.value;
    const b = store.base.value;
    if (!e || !b) return;
    const base = valuesOf(b);
    let any = false;
    for (const el of form.querySelectorAll(".field[data-field]")) {
      const dirty = isDirty(el.dataset.field, e, base);
      el.classList.toggle("dirty", dirty);
      any ||= dirty;
    }
    $("revert-all").hidden = !any;
  });
}

// --- activity ---

// The activity strip reports render and save at the size of a primary control,
// since in Single they are the only long operations. state is idle, working,
// done, warn or error, and the strip adds stale on top of done when the edits
// have moved on since the preview was drawn
let ticker = 0;

function begin(title, detail) {
  clearInterval(ticker);
  store.activity.value = { state: "working", title, step: "Starting…", frac: 0, started: performance.now(), detail };
  ticker = setInterval(() => {
    const a = store.activity.peek();
    if (a.state === "working") store.activity.value = { ...a };
  }, 100);
  app.status.value = title;
}

function progress(step, frac) {
  const a = store.activity.peek();
  if (a.state !== "working") return;
  store.activity.value = { ...a, step: step ? `${step}…` : a.step, frac };
}

function finish(state, title, step, detail) {
  clearInterval(ticker);
  const a = store.activity.peek();
  const secs = a.started ? (performance.now() - a.started) / 1000 : 0;
  store.activity.value = { state, title, step, frac: 1, secs, detail: detail ?? a.detail };
  app.status.value = step ? `${title}. ${step}` : title;
}

// compactSize is a resolution short enough to sit beside the strip title
const compactSize = (r) => (r ? `${r.width}×${r.height} · ${r.dpi} dpi` : "");

const seconds = (s) => (s < 10 ? `${s.toFixed(1)}s` : `${Math.round(s)}s`);

// --- render and save ---

function snapshot() {
  return JSON.stringify(store.edits.peek());
}

function outputDPI() {
  const r = app.resolution.peek();
  return r && r.output ? r.output.dpi : 0;
}

// Edits redraw the preview once typing pauses for this long
const LIVE_DELAY_MS = 350;

// abandonRender stops following the render in flight and tells the app to
// stop working on it
function abandonRender() {
  renderSeq++;
  if (renderJob) renderJob.close();
  if (renderJobId) api.cancelRender(renderJobId);
  renderJob = null;
  renderJobId = "";
}

async function render() {
  clearTimeout(liveTimer);
  const base = store.base.peek();
  if (!base || saving) return;
  abandonRender();
  const seq = renderSeq;
  preview = null;
  const name = faceTitle();
  const face = store.face.peek();
  const snap = snapshot();
  const res = app.resolution.peek();
  begin(`Rendering ${name}`, compactSize(res && res.preview));
  try {
    const { jobId, dpi } = await api.render(base, store.edits.peek(), "preview", face);
    if (seq !== renderSeq) {
      api.cancelRender(jobId);
      return;
    }
    renderJobId = jobId;
    const job = (renderJob = api.renderEvents(jobId, (step, frac) => {
      if (renderJob === job) progress(step, frac);
    }));
    const done = await job;
    if (renderJob !== job) return;
    renderJob = null;
    renderJobId = "";
    const img = $("preview-img");
    img.src = api.renderImageURL(jobId);
    img.hidden = false;
    $("stage-placeholder").hidden = true;
    preview = { jobId, dpi, face };
    store.rendered.value = snap;
    if (done.artMissing) finish("warn", `Rendered ${name}`, "The art could not be fetched, so the frame is empty");
    else finish("done", `Rendered ${name}`, "");
  } catch (err) {
    if (seq !== renderSeq) return;
    renderJob = null;
    renderJobId = "";
    finish("error", "Render failed", err.message);
  }
}

// renderIfSupported draws a newly picked card or face, or for one no installed
// template renders, clears the preview and leaves the strip idle so it shows
// the warning. Render stays available, since an edited type line can make the
// card one a template renders
function renderIfSupported() {
  if (!faceUnsupportedBy(store.base.peek(), store.face.peek())) {
    render();
    return;
  }
  clearTimeout(liveTimer);
  abandonRender();
  preview = null;
  clearInterval(ticker);
  batch(() => {
    store.rendered.value = null;
    store.activity.value = { state: "idle", title: "", step: "", frac: 0 };
  });
  $("preview-img").hidden = true;
  $("stage-placeholder").hidden = false;
}

// save writes the card at the output resolution. The preview was rendered smaller,
// so this renders again at full size unless the two agree and the preview on
// screen is current
async function save() {
  const base = store.base.peek();
  if (!base || saving) return;
  const name = faceTitle();
  const face = store.face.peek();
  const res = app.resolution.peek();
  const size = compactSize(res && res.output);
  if (preview && preview.dpi === outputDPI() && preview.face === face && store.rendered.peek() === snapshot()) {
    await writeFile(preview.jobId, name, size);
    return;
  }
  abandonRender();
  saving = true;
  $("save-btn").disabled = true;
  $("render-btn").disabled = true;
  begin(`Saving ${name}.png`, size);
  try {
    const { jobId } = await api.render(base, store.edits.peek(), "output", face);
    await api.renderEvents(jobId, progress);
    await writeFile(jobId, name, size);
  } catch (err) {
    finish("error", "Save failed", err.message);
  } finally {
    saving = false;
    $("save-btn").disabled = !store.base.peek();
    $("render-btn").disabled = !store.base.peek();
  }
}

// writeFile asks where to put a finished render and writes it there, ending the
// activity with the file's path, or quietly when the user cancels the dialog
async function writeFile(jobId, name, size) {
  try {
    const path = await api.saveRender(jobId, app.settings.peek().outputDir || "");
    if (path) finish("done", `Saved ${name}.png`, path, size);
    else finish("idle", "", "");
  } catch (err) {
    finish("error", "Save failed", err.message);
  }
}

// --- wiring ---

export const single = {
  render,
  save,
  focusSearch() {
    const input = $("search-input");
    input.focus();
    input.select();
  },
};

export function initSingle() {
  buildEditor();

  $("search-form").addEventListener("submit", (e) => {
    e.preventDefault();
    runSearch($("search-input").value);
  });
  $("search-input").addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") {
      const first = $("results").querySelector(".result");
      if (first) {
        e.preventDefault();
        first.focus();
      }
    }
  });
  $("results").addEventListener("keydown", onResultsKey);
  $("empty-search").addEventListener("click", single.focusSearch);
  $("render-btn").addEventListener("click", render);
  $("save-btn").addEventListener("click", save);
  $("printing-trigger").addEventListener("click", openPrintings);
  $("recents-btn").addEventListener("click", openRecents);
  $("revert-all").addEventListener("click", () => {
    store.edits.value = baseValues();
  });

  effect(renderResults);
  effect(() => {
    $("recents-btn").hidden = store.recents.value.length === 0;
  });

  // Selection updates rows in place, so keyboard focus survives opening one
  effect(() => {
    const sel = store.selected.value;
    $("results").querySelectorAll(".result").forEach((el, i) => {
      el.setAttribute("aria-selected", String(i === sel));
      if (sel >= 0) el.tabIndex = i === sel ? 0 : -1;
    });
  });

  // Regroup the last results when the printing setting changes, and only then,
  // since other settings writes pass through the same signal
  let lastExpand = app.settings.peek().expandPrintings;
  effect(() => {
    const expand = app.settings.value.expandPrintings;
    if (expand === lastExpand) return;
    lastExpand = expand;
    if (!lastCards.length) return;
    store.results.value = group(lastCards, expand);
    store.selected.value = -1;
  });

  effect(() => {
    const has = !!store.base.value;
    $("single-empty").hidden = has;
    $("editor").hidden = !has;
    $("render-btn").disabled = !has;
    $("save-btn").disabled = !has || saving;
    if (!has) {
      $("preview-img").hidden = true;
      $("stage-placeholder").hidden = false;
      setZoom(0);
    }
  });

  effect(() => {
    const e = store.edits.value;
    $("editor-title").textContent = e ? e.name || "Untitled card" : "";
  });

  // A double-faced card previews one face at a time. The fields always edit
  // the front, which the note says while the back is showing
  effect(() => {
    const faces = faceCount(store.base.value);
    const face = store.face.value;
    $("face-toggle").hidden = faces < 2;
    $("face-toggle-label").textContent = face > 0 ? "Front face" : "Back face";
    $("face-note").hidden = face === 0;
  });
  $("face-toggle").addEventListener("click", () => {
    const faces = faceCount(store.base.peek());
    if (faces < 2) return;
    store.face.value = (store.face.peek() + 1) % faces;
    renderIfSupported();
  });

  effect(() => {
    const b = store.base.value;
    const list = store.printings.value;
    if (!b) return;
    $("printing-label").textContent = printingText(b);
    $("printing-count").textContent = list && list.length > 1 ? `${list.length} printings` : "";
  });

  // The preview is stale when the edits have moved on since it was drawn
  const stale = () => {
    const e = store.edits.value;
    const r = store.rendered.value;
    return !!e && r !== null && JSON.stringify(e) !== r;
  };

  effect(() => {
    $("stale-dot").hidden = !stale();
  });

  // Live preview: an edit that leaves the preview behind schedules a redraw.
  // A face no installed template renders is skipped, and Render stays manual
  effect(() => {
    const e = store.edits.value;
    clearTimeout(liveTimer);
    if (!e || app.settings.value.livePreview === false || JSON.stringify(e) === store.rendered.peek()) return;
    liveTimer = setTimeout(() => {
      const base = store.base.peek();
      if (base && !faceUnsupportedBy(base, store.face.peek())) render();
    }, LIVE_DELAY_MS);
  });

  const GLYPH = { idle: "card", working: "render", done: "check", stale: "render", warn: "warn", error: "warn" };
  effect(() => {
    const a = store.activity.value;
    const has = !!store.base.value;
    app.template.value;
    const why = has ? faceUnsupportedBy(store.base.value, store.face.value) : "";
    let { state, title, step } = a;
    let meta = "";

    if (!has && state !== "working") {
      state = "idle";
      title = "No card selected";
      step = "Search for a card and pick a result to render it here";
    } else if (state === "idle" && why) {
      state = "warn";
      title = why;
      step = "Install a template that does from the template menu, or edit the type line and press Render";
    } else if (state === "working") {
      meta = `${Math.round(a.frac * 100)}% · ${seconds((performance.now() - a.started) / 1000)}`;
    } else if (state === "done" && stale()) {
      state = "stale";
      title = "Preview is behind your edits";
      step = app.settings.peek().livePreview === false ? "Press Render or ⌘↵ to draw them" : "Redrawing when you pause, or press ⌘↵ to draw now";
    } else if (a.secs) {
      meta = seconds(a.secs);
    }
    if (a.detail && state !== "idle" && state !== "working") meta = meta ? `${a.detail} · ${meta}` : a.detail;

    const strip = $("activity");
    strip.dataset.state = state;
    $("activity-title").textContent = title;
    $("activity-step").textContent = step || "";
    $("activity-meta").textContent = meta;
    $("activity-glyph").setAttribute("href", `icons.svg#i-${GLYPH[state]}`);
    const fill = $("activity-fill");
    fill.style.width = `${(state === "idle" ? 0 : state === "working" ? a.frac : 1) * 100}%`;
    $("mode-single").classList.toggle("busy", state === "working");
  });

  document.addEventListener("mimic:template-changed", () => {
    if (store.base.peek()) renderIfSupported();
  });
  document.addEventListener("mimic:resolution-changed", () => {
    if (store.base.peek()) render();
  });

  loadRecents();
}

import { api } from "../api.js";
import { $, h, icon } from "../dom.js";
import { app, signal, effect, batch } from "../state.js";
import { openPopover, closePopover } from "../components/popover.js";
import { renderPips, symbolPalette, insertAtCursor } from "../components/symbols.js";
import { setZoom } from "../components/preview.js";
import { faceUnsupportedBy } from "../supports.js";
import { createCardEditor, ALL_FIELDS, valuesOf, isDirty } from "../components/cardEditor.js";

// Flow 1: search a card, adjust anything about it, render, download. The
// browser holds the fetched card as the base and the form values as edits, so
// every field knows whether it differs from what Scryfall returned

const store = {
  results: signal([]), // [{ name, cards: [card...] }]
  recents: signal([]),
  noMatch: signal(null), // the last query when it matched nothing
  selected: signal(-1),
  base: signal(null),
  edits: signal(null),
  // reference is what edits are measured against: the fetched card with the
  // global rules' changes on it, and ruled is those changes alone
  reference: signal({}),
  ruled: signal({}),
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

// ruledFields asks which fields the global rules change on a card. A failure
// reads as none, since a card is still worth showing when the rules cannot be read
async function ruledFields(card, fields) {
  try {
    return (await api.applyRules(card, fields)).fields || {};
  } catch {
    return {};
  }
}

// adoptSeq counts the cards taken up, so a slow answer about one that has been
// replaced since is dropped
let adoptSeq = 0;

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

async function selectResult(i) {
  const g = store.results.peek()[i];
  if (!g) return;
  leaveRow();
  const card = g.cards[0];
  store.selected.value = i;
  const seq = ++adoptSeq;
  const ruled = await ruledFields(card, {});
  if (seq !== adoptSeq) return;
  const reference = { ...valuesOf(card), ...ruled };
  batch(() => {
    store.base.value = card;
    store.ruled.value = ruled;
    store.reference.value = reference;
    store.edits.value = reference;
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

// retarget points the editor at a card with the rules' changes on it. Fields the
// user has not edited follow the new reference, and edited ones keep their edit
function retarget(card, ruled) {
  const old = store.reference.peek();
  const reference = { ...valuesOf(card), ...ruled };
  const edits = store.edits.peek();
  const merged = {};
  for (const { f } of ALL_FIELDS) merged[f] = isDirty(f, edits, old) ? edits[f] : reference[f];
  batch(() => {
    store.base.value = card;
    store.ruled.value = ruled;
    store.reference.value = reference;
    store.edits.value = merged;
    if (store.face.peek() >= faceCount(card)) store.face.value = 0;
  });
}

// choosePrinting swaps the base card
async function choosePrinting(card) {
  const seq = ++adoptSeq;
  const ruled = await ruledFields(card, {});
  if (seq !== adoptSeq) return;
  retarget(card, ruled);
  renderIfSupported();
}

// --- editing a list row ---

// fromRow is the list row being edited here, with what to do with its edits. The
// card and the row's own fields are on screen as in any other card, and the bar
// above the editor sends the edits back
let fromRow = null;

function leaveRow() {
  fromRow = null;
  $("row-return").hidden = true;
}

// openRow loads a list row's card with the fields the row sets, so the preview
// shows what the render of that row will draw
async function openRow({ card, fields, label, onApply }) {
  const seq = ++adoptSeq;
  const ruled = await ruledFields(card, fields);
  if (seq !== adoptSeq) return;
  const reference = { ...valuesOf(card), ...ruled };
  fromRow = { onApply };
  batch(() => {
    store.selected.value = -1;
    store.base.value = card;
    store.ruled.value = ruled;
    store.reference.value = reference;
    store.edits.value = { ...reference, ...(fields || {}) };
    store.face.value = 0;
  });
  $("row-return-name").textContent = label || card.Name;
  $("row-return").hidden = false;
  loadPrintings(card.Name, null);
  renderIfSupported();
}

// applyToRow hands the fields that differ from the card, with the rules' changes
// counted as part of it, back to the row
function applyToRow() {
  if (!fromRow) return;
  const edits = store.edits.peek();
  const reference = store.reference.peek();
  const fields = {};
  for (const { f } of ALL_FIELDS) if (isDirty(f, edits, reference)) fields[f] = edits[f];
  const { onApply } = fromRow;
  leaveRow();
  location.hash = "list";
  onApply(fields);
}

// The rules changed, so the card on screen takes the new ones
async function reapplyRules() {
  const card = store.base.peek();
  if (!card) return;
  const seq = ++adoptSeq;
  const ruled = await ruledFields(card, {});
  if (seq !== adoptSeq) return;
  retarget(card, ruled);
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
  openRow,
  focusSearch() {
    const input = $("search-input");
    input.focus();
    input.select();
  },
};

let editor = null;

export function initSingle() {
  editor = createCardEditor({ base: store.base, edits: store.edits, reference: store.reference, ruled: store.ruled, onEnter: render });
  editor.mount($("editor-form"));
  effect(() => {
    $("revert-all").hidden = !editor.dirty.value;
  });
  document.addEventListener("mimic:rules-changed", reapplyRules);
  $("row-return-apply").addEventListener("click", applyToRow);
  $("row-return-cancel").addEventListener("click", () => {
    leaveRow();
    location.hash = "list";
  });

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
    store.edits.value = store.reference.peek();
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

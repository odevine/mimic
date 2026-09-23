import { api } from "../api.js";
import { $, h, icon, modKey } from "../dom.js";
import { app, signal, effect, batch } from "../state.js";
import { keyedRows } from "../components/keyed.js";
import { syncChips } from "../components/chips.js";
import { openFolderPicker } from "../components/folderPicker.js";
import { toast } from "../components/toast.js";

// Flow 2: paste a list, check what it matched, render the lot to a folder. The
// server parses and resolves, the browser holds the rows. Each row keeps its
// state in its own signal, so a resolved row redraws only itself

const FORMAT_LABEL = {
  names: "Plain names",
  quantity: "Quantity prefix",
  arena: "Arena export",
  csv: "CSV",
};

const STATUS = {
  pending: { label: "resolving", cls: "idle" },
  matched: { label: "matched", cls: "ok" },
  ambiguous: { label: "ambiguous", cls: "warn" },
  notFound: { label: "not found", cls: "err" },
  error: { label: "lookup failed", cls: "err" },
  custom: { label: "custom", cls: "info" },
};

// A row needs attention until it has one card the user can stand behind
const needsAttention = (s) => s.status === "ambiguous" || s.status === "notFound" || s.status === "error";
const renderable = (s) => !s.excluded && (s.status === "matched" || s.status === "custom") && s.card;

const store = {
  rows: signal([]), // [{ id, line, text, qty, group, fields, key, state: signal }]
  version: signal(0), // bumped on any row change, for counts and filters
  format: signal(""), // what the server read the list as
  resolving: signal(null), // { done, total } while a resolve streams
  filter: signal("all"), // all, attention, custom, or group:<name>
  query: signal(""),
  selected: signal(null),
  source: signal("Pasted list"),
};

let table = null;
let resolveJob = null;
// excludedKeys remembers what the user unticked by line text and occurrence, so
// an exclusion survives resolving the list again
const excludedKeys = new Set();

const bump = () => (store.version.value = store.version.peek() + 1);

function setRow(row, patch) {
  row.state.value = { ...row.state.peek(), ...patch };
  bump();
}

// --- resolving ---

async function resolve() {
  const text = $("list-input").value;
  if (!text.trim()) {
    $("list-input").focus();
    return;
  }
  if (resolveJob) resolveJob.close();
  let res;
  try {
    res = await api.resolve(text, $("list-format").value);
  } catch (err) {
    toast(err.message, "err", 5000);
    return;
  }

  const seen = new Map();
  const rows = res.rows.map((r, i) => {
    const base = (r.text || "").toLowerCase();
    const n = (seen.get(base) || 0) + 1;
    seen.set(base, n);
    const key = `${base}\u0000${n}`;
    return {
      id: String(i),
      line: r.line,
      text: r.text,
      qty: r.qty,
      name: r.name || "",
      group: r.group || "",
      fields: r.fields || null,
      key,
      state: signal({ status: "pending", card: null, candidates: null, note: "", excluded: excludedKeys.has(key) }),
    };
  });

  batch(() => {
    store.format.value = res.format;
    store.rows.value = rows;
    store.filter.value = "all";
    store.selected.value = null;
    store.resolving.value = { done: 0, total: rows.length };
  });
  table.reset(rows);
  bump();
  if (!rows.length) {
    store.resolving.value = null;
    return;
  }

  const byIndex = rows.slice();
  const job = api.resolveEvents(res.jobId, (step, frac, e) => {
    if (!e.row) return;
    applyResolved(byIndex[e.row.index], e.row);
    store.resolving.value = { done: store.resolving.peek().done + 1, total: rows.length };
  });
  resolveJob = job;
  try {
    await job;
  } catch (err) {
    if (resolveJob !== job) return;
    toast(`Resolving stopped: ${err.message}`, "err", 5000);
  }
  if (resolveJob !== job) return;
  resolveJob = null;
  store.resolving.value = null;
  // A messy list opens on the rows that need a look, a clean one on the deck
  store.filter.value = counts().attention > 0 ? "attention" : "all";
}

// applyResolved lands one result on its row. A query line becomes one row per
// card it matched, inserted where the line was
function applyResolved(row, r) {
  if (!row) return;
  if (r.cards && r.cards.length) {
    const expanded = r.cards.map((card, k) => {
      const key = `${row.key}\u0000${k}`;
      return {
        id: `${row.id}.${k}`,
        line: row.line,
        text: card.Name,
        qty: 1,
        group: row.group,
        fields: null,
        key,
        fromQuery: row.text,
        state: signal({ status: "matched", card, candidates: null, note: k === 0 ? r.note || "" : "", excluded: excludedKeys.has(key) }),
      };
    });
    const rows = store.rows.peek();
    const at = rows.indexOf(row);
    store.rows.value = [...rows.slice(0, at), ...expanded, ...rows.slice(at + 1)];
    table.replace(row.id, expanded);
    bump();
    return;
  }
  setRow(row, { status: r.status, card: r.card || null, candidates: r.candidates || null, note: r.note || "" });
}

// retryRow resolves one row again on its own, for a lookup that failed
async function retryRow(row, name) {
  // A new resolve would supersede the one still streaming, so wait for it
  if (store.resolving.peek()) {
    toast("Wait for the list to finish resolving, then try again");
    return;
  }
  setRow(row, { status: "pending", note: "" });
  try {
    const res = await api.resolve(name, "names");
    await api.resolveEvents(res.jobId, (step, frac, e) => {
      if (e.row) applyResolved(row, e.row);
    });
  } catch (err) {
    setRow(row, { status: "error", note: err.message });
  }
}

// --- counts and filters ---

function counts() {
  const c = { total: 0, matched: 0, ambiguous: 0, notFound: 0, error: 0, custom: 0, attention: 0, render: 0, cards: 0, groups: new Map() };
  for (const row of store.rows.peek()) {
    const s = row.state.peek();
    c.total++;
    if (c[s.status] !== undefined) c[s.status]++;
    if (needsAttention(s)) c.attention++;
    if (renderable(s)) {
      c.render++;
      c.cards += row.qty;
    }
    if (row.group) c.groups.set(row.group, (c.groups.get(row.group) || 0) + 1);
  }
  return c;
}

function matchesFilter(row) {
  const s = row.state.peek();
  const f = store.filter.peek();
  if (f === "attention" && !needsAttention(s)) return false;
  if (f === "custom" && s.status !== "custom") return false;
  if (f.startsWith("group:") && row.group !== f.slice(6)) return false;
  const q = store.query.peek().trim().toLowerCase();
  if (q) {
    const hay = `${row.text} ${s.card ? `${s.card.Name} ${s.card.SetCode} ${s.card.TypeLine}` : ""}`.toLowerCase();
    if (!hay.includes(q)) return false;
  }
  return true;
}

// --- rows ---

function thumb(card) {
  if (!card || !card.ArtworkURL) return h("span", { class: "thumb" });
  return h("img", { class: "thumb", src: card.ArtworkURL, alt: "", loading: "lazy", decoding: "async" });
}

function printingOf(card) {
  if (!card || !card.SetCode) return "";
  return `${card.SetCode.toUpperCase()}${card.CollectorNumber ? ` ${card.CollectorNumber}` : ""}`;
}

function buildRow(row) {
  const include = h("input", {
    type: "checkbox",
    "aria-label": `Render ${row.text}`,
    onClick: (e) => e.stopPropagation(),
    onChange: (e) => {
      if (e.target.checked) excludedKeys.delete(row.key);
      else excludedKeys.add(row.key);
      setRow(row, { excluded: !e.target.checked });
    },
  });
  const qty = h("td", { class: "num" });
  const name = h("span", { class: "row-name" });
  const sub = h("span", { class: "row-sub" });
  const cardCell = h("td", { class: "card-cell" });
  const set = h("td", { class: "mono" });
  const group = h("td", { class: "dim col-group" });
  const status = h("td", {});
  const tr = h(
    "tr",
    {
      class: "row-main",
      tabindex: "-1",
      onClick: () => select(row),
    },
    h("td", {}, include),
    qty,
    cardCell,
    set,
    group,
    status,
  );
  const detail = h("tr", { class: "expanded row-detail" });
  const el = h("tbody", { class: "row-group", dataset: { id: row.id } }, tr, detail);

  const disposeState = effect(() => {
    const s = row.state.value;
    include.checked = !s.excluded;
    tr.classList.toggle("excluded", !!s.excluded);
    qty.textContent = row.qty > 1 ? String(row.qty) : "1";
    const card = s.card;
    name.textContent = card && s.status !== "notFound" ? card.Name : row.fromQuery ? row.text : nameOf(row);
    const typed = nameOf(row);
    sub.textContent = card && card.Name.toLowerCase() !== typed.toLowerCase() && !row.fromQuery ? `from “${typed}”` : card ? card.TypeLine : "";
    if (row.fromQuery && s.note) sub.textContent = s.note;
    // A CSV row's own columns overlay the matched card, so say which
    const changed = Object.keys(row.fields || {}).filter((k) => k !== "name");
    if (changed.length && s.status === "matched") sub.textContent = `${changed.join(", ")} set by the list`;
    cardCell.replaceChildren(thumb(s.status === "notFound" ? null : card), h("span", { class: "row-text" }, name, sub));
    set.textContent = s.status === "matched" || s.status === "custom" ? printingOf(card) : "";
    group.textContent = row.group;
    const st = STATUS[s.status] || STATUS.pending;
    const pill = h("span", { class: `pill ${st.cls}` }, st.label);
    status.replaceChildren(pill);
    if (s.note && !row.fromQuery) status.append(h("span", { class: "note-dot", "data-tip": s.note, "aria-label": s.note }, icon("warn")));
    buildDetail(row, s, detail);
  });
  const disposeSel = effect(() => {
    tr.classList.toggle("selected", store.selected.value === row.id);
  });
  return {
    el,
    dispose() {
      disposeState();
      disposeSel();
    },
  };
}

// nameOf is the name a row asked for, before any lookup
function nameOf(row) {
  return (row.fields && row.fields.name) || row.name || row.text;
}

// buildDetail fills the row's second line: the choices for an ambiguous row, a
// search for a row with no match, and a retry for a failed lookup. Only a
// selected row that needs attention shows it
function buildDetail(row, s, detail) {
  const open = store.selected.peek() === row.id || (needsAttention(s) && store.filter.peek() === "attention");
  const show = open && (s.candidates?.length || s.status === "notFound" || s.status === "error");
  detail.hidden = !show;
  if (!show) {
    detail.replaceChildren();
    return;
  }
  const cell = h("td", { colspan: "6" });
  detail.replaceChildren(cell);
  if (s.status === "error") {
    cell.append(
      h("span", { class: "dim" }, s.note || "The lookup failed."),
      h("button", { class: "btn subtle", type: "button", onClick: () => retryRow(row, nameOf(row)) }, icon("render"), "Try again"),
    );
    return;
  }
  if (s.candidates?.length) {
    const chosen = s.status === "matched" ? s.card : null;
    cell.append(
      h("span", { class: "dim" }, s.status === "matched" ? "Chosen from" : "Did you mean"),
      h(
        "span",
        { class: "choices", role: "radiogroup", "aria-label": `Choices for ${row.text}` },
        s.candidates.map((c) =>
          h(
            "button",
            {
              class: "chip choice",
              type: "button",
              role: "radio",
              "aria-checked": String(chosen === c),
              onClick: (e) => {
                e.stopPropagation();
                setRow(row, { status: "matched", card: c, note: "" });
              },
            },
            c.Name,
            c.SetCode ? h("span", { class: "count mono" }, c.SetCode.toUpperCase()) : null,
          ),
        ),
      ),
    );
    return;
  }
  if (row.text.startsWith("?")) {
    cell.append(h("span", { class: "dim" }, s.note || "The query matched no cards."), h("span", { class: "faint" }, "Edit the line and resolve again."));
    return;
  }
  // No match at all: search Scryfall by hand and pick from what comes back
  const results = h("span", { class: "choices" });
  const input = h("input", {
    class: "inline-search",
    placeholder: "Search Scryfall for this card…",
    value: nameOf(row),
    onClick: (e) => e.stopPropagation(),
    onKeydown: async (e) => {
      if (e.key !== "Enter") return;
      e.preventDefault();
      results.replaceChildren(h("span", { class: "faint" }, "Searching…"));
      try {
        const found = (await api.search(input.value)).slice(0, 8);
        results.replaceChildren(
          ...(found.length
            ? found.map((f) =>
                h(
                  "button",
                  {
                    class: "chip choice",
                    type: "button",
                    onClick: (ev) => {
                      ev.stopPropagation();
                      setRow(row, { status: "matched", card: f.card, candidates: found.map((x) => x.card), note: "" });
                    },
                  },
                  f.card.Name,
                ),
              )
            : [h("span", { class: "faint" }, "Nothing matched")]),
        );
      } catch (err) {
        results.replaceChildren(h("span", { class: "faint" }, err.message));
      }
    },
  });
  cell.append(input, results);
}

function select(row) {
  const prev = store.selected.peek();
  store.selected.value = prev === row.id ? null : row.id;
  // Redraw the two rows whose detail line depends on selection
  for (const r of store.rows.peek()) {
    if (r.id === prev || r.id === row.id) r.state.value = { ...r.state.peek() };
  }
  table.el(row.id)?.querySelector(".row-main")?.focus({ preventScroll: true });
}

function moveSelection(delta) {
  const visible = store.rows.peek().filter((r) => !table.el(r.id)?.hidden);
  if (!visible.length) return;
  const i = visible.findIndex((r) => r.id === store.selected.peek());
  const next = visible[Math.max(0, Math.min(visible.length - 1, i < 0 ? 0 : i + delta))];
  if (next.id !== store.selected.peek()) select(next);
  table.el(next.id)?.scrollIntoView({ block: "nearest" });
}

// --- output and render ---

function outputDir() {
  return app.settings.value.outputDir || "";
}

// rememberOutputDir stores the folder and moves it to the front of the recents
export function rememberOutputDir(dir) {
  const s = app.settings.peek();
  const recent = [dir, ...(s.recentOutputDirs || []).filter((d) => d !== dir)].slice(0, 6);
  app.settings.value = { ...s, outputDir: dir, recentOutputDirs: recent };
  api.saveSettings(app.settings.peek()).catch(() => {});
}

async function render() {
  const c = counts();
  if (store.resolving.peek() || !c.render) return;
  const dir = outputDir();
  if (!dir) {
    toast("Choose an output folder first", "err");
    pickFolder();
    return;
  }
  const rows = [];
  for (const row of store.rows.peek()) {
    const s = row.state.peek();
    if (!renderable(s)) continue;
    rows.push({ name: s.card.Name, qty: row.qty, group: row.group, base: s.card, fields: row.fields || undefined });
  }
  $("list-render").disabled = true;
  try {
    const run = await api.run(rows, dir, store.source.peek());
    rememberOutputDir(dir);
    document.dispatchEvent(new CustomEvent("mimic:run-started", { detail: run }));
  } catch (err) {
    toast(err.message, "err", 6000);
  } finally {
    $("list-render").disabled = false;
  }
}

function pickFolder() {
  const s = app.settings.peek();
  openFolderPicker($("list-output"), {
    start: s.outputDir || "",
    recent: s.recentOutputDirs || [],
    onChoose: (dir) => rememberOutputDir(dir),
  });
}

// --- source ---

function loadFile(file) {
  if (!file) return;
  if (file.size > 2 << 20) {
    toast("That file is too large to be a list", "err");
    return;
  }
  const reader = new FileReader();
  reader.onload = () => {
    $("list-input").value = String(reader.result);
    store.source.value = file.name.replace(/\.[^.]+$/, "");
    resolve();
  };
  reader.readAsText(file);
}

function initSource() {
  const input = $("list-input");
  input.addEventListener("keydown", (e) => {
    if (modKey(e) && e.key === "Enter") {
      e.preventDefault();
      e.stopPropagation();
      resolve();
    }
  });
  input.addEventListener("input", () => {
    store.source.value = "Pasted list";
  });
  input.addEventListener("dragover", (e) => {
    e.preventDefault();
    input.classList.add("drop");
  });
  input.addEventListener("dragleave", () => input.classList.remove("drop"));
  input.addEventListener("drop", (e) => {
    e.preventDefault();
    input.classList.remove("drop");
    loadFile(e.dataTransfer.files[0]);
  });
  $("list-open").addEventListener("click", () => $("list-file").click());
  $("list-file").addEventListener("change", (e) => {
    loadFile(e.target.files[0]);
    e.target.value = "";
  });
  $("list-resolve").addEventListener("click", resolve);
  $("list-format").addEventListener("change", () => {
    if (store.rows.peek().length) resolve();
  });

  effect(() => {
    const f = store.format.value;
    const forced = $("list-format").value;
    $("list-detected").textContent = f ? `${forced ? "Read as" : "Detected"} ${FORMAT_LABEL[f] || f}.` : "Paste a list, drop a file, or open one from disk.";
  });
}

// --- review chrome ---

function initReview() {
  table = keyedRows($("list-table-el"), buildRow);

  $("list-filter").addEventListener("input", (e) => (store.query.value = e.target.value));
  $("list-all").addEventListener("change", (e) => {
    for (const row of store.rows.peek()) {
      if (table.el(row.id)?.hidden) continue;
      if (e.target.checked) excludedKeys.delete(row.key);
      else excludedKeys.add(row.key);
      row.state.value = { ...row.state.peek(), excluded: !e.target.checked };
    }
    bump();
  });
  $("list-output").addEventListener("click", pickFolder);
  $("list-render").addEventListener("click", render);

  $("list-review").addEventListener("keydown", (e) => {
    if (e.target.closest("input, textarea, button")) return;
    if (e.key === "ArrowDown" || e.key === "j") {
      e.preventDefault();
      moveSelection(1);
    } else if (e.key === "ArrowUp" || e.key === "k") {
      e.preventDefault();
      moveSelection(-1);
    } else if (e.key === " " && store.selected.peek()) {
      e.preventDefault();
      const row = store.rows.peek().find((r) => r.id === store.selected.peek());
      if (row) table.el(row.id).querySelector("input[type=checkbox]").click();
    }
  });

  // Summary line, filter chips, and which rows show
  effect(() => {
    store.version.value;
    const filter = store.filter.value;
    store.query.value;
    const c = counts();
    const res = store.resolving.value;
    const empty = c.total === 0 && !res;
    $("list-empty").hidden = !empty;
    $("list-review-body").hidden = empty;
    if (empty) return;

    const parts = [`${c.total} rows`];
    if (c.matched) parts.push(`${c.matched} matched`);
    if (c.ambiguous) parts.push(`${c.ambiguous} ambiguous`);
    if (c.notFound) parts.push(`${c.notFound} not found`);
    if (c.error) parts.push(`${c.error} failed`);
    if (c.custom) parts.push(`${c.custom} custom`);
    $("list-summary").textContent = res ? `Resolved ${res.done} of ${res.total}` : parts.join(" · ");
    $("list-progress").hidden = !res;
    if (res) $("list-progress").firstElementChild.style.width = `${(res.done / Math.max(res.total, 1)) * 100}%`;

    const chips = [
      { label: "All", value: "all", count: c.total },
      { label: "Needs attention", value: "attention", count: c.attention },
    ];
    if (c.custom) chips.push({ label: "Custom", value: "custom", count: c.custom });
    for (const [g, n] of c.groups) chips.push({ label: g, value: `group:${g}`, count: n });
    syncChips($("list-chips"), chips, filter, (v) => (store.filter.value = v));
    // Section is empty for a list with no headers, so it only shows when used
    $("list-table-el").classList.toggle("no-groups", c.groups.size === 0);

    const shown = table.show(store.rows.peek(), matchesFilter);
    $("list-none").hidden = shown > 0;
    const allIncluded = store.rows.peek().every((r) => !r.state.peek().excluded);
    $("list-all").checked = allIncluded;

    const skipped = c.total - c.render - store.rows.peek().filter((r) => r.state.peek().excluded).length;
    $("list-render-label").textContent = c.render ? `Render ${c.render} ${c.render === 1 ? "card" : "cards"}` : "Render";
    $("list-render").disabled = !!res || !c.render;
    $("list-skip-note").textContent = !res && skipped > 0 ? `${skipped} ${skipped === 1 ? "row needs" : "rows need"} attention and will be skipped` : "";
  });

  // The selected row's detail line reads the filter, so a filter change
  // redraws the rows that need attention
  let lastFilter = store.filter.peek();
  effect(() => {
    const f = store.filter.value;
    if (f === lastFilter) return;
    lastFilter = f;
    for (const row of store.rows.peek()) {
      if (needsAttention(row.state.peek())) row.state.value = { ...row.state.peek() };
    }
  });

  effect(() => {
    const dir = outputDir();
    $("list-output-label").textContent = dir || "Choose a folder…";
    $("list-output-label").classList.toggle("faint", !dir);
    $("list-output").dataset.tip = dir ? "Change the output folder" : "Choose where the PNGs are written";
  });
}

export const list = {
  render,
  resolve,
  focusInput: () => $("list-input").focus(),
};

export function initList() {
  initSource();
  initReview();
}

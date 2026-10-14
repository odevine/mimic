import { api } from "../api.js";
import { $, h, icon } from "../dom.js";
import { app, signal, effect, batch } from "../state.js";
import { toast } from "../components/toast.js";
import {
  DPI_DEFAULT,
  canvasSize,
  cardLine,
  checkName,
  draftLabel,
  forkName,
  forkedFrom,
  layoutLine,
  parseDpi,
  problems,
  startingCard,
} from "../studio.js";

// The editor: build a template and see a test card drawn with it. A draft is a
// template on disk that the app renders as it is, so everything shown here is read
// from the app and nothing is kept in the page beyond the card being tested

const store = {
  drafts: signal(null), // null until read, then the list
  current: signal(""), // the id of the draft open
  view: signal(null), // what openDraft told about it
  presets: signal([]),
  card: signal(null), // the test card, as the app sent it
  face: signal(0),
  preset: signal(""), // the id of the preset the card came from, or ""
  results: signal([]), // search results for a test card
  searched: signal(""), // the query behind the results
  activity: signal({ state: "idle", title: "Nothing drawn yet", step: "Open a template to draw a test card with it", frac: 0 }),
};

let renderJob = null;
let renderJobId = "";
let renderSeq = 0; // counts renders started, so a slow start can tell it was superseded
let openSeq = 0; // counts drafts opened, so a slow open of a draft left behind is dropped
let searchAbort = null;
let ticker = 0;

// --- activity ---

// The strip reports a render the way Single's does: working with a bar, then done,
// or the reason a draft could not be drawn
function begin(title) {
  clearInterval(ticker);
  store.activity.value = { state: "working", title, step: "Starting…", frac: 0, started: performance.now() };
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

function finish(state, title, step) {
  clearInterval(ticker);
  const a = store.activity.peek();
  const secs = a.started ? (performance.now() - a.started) / 1000 : 0;
  store.activity.value = { state, title, step, frac: 1, secs };
  app.status.value = step ? `${title}. ${step}` : title;
}

const seconds = (s) => (s < 10 ? `${s.toFixed(1)}s` : `${Math.round(s)}s`);

// --- render ---

function abandonRender() {
  renderSeq++;
  if (renderJob) renderJob.close();
  if (renderJobId) api.cancelRender(renderJobId);
  renderJob = null;
  renderJobId = "";
}

function clearPreview() {
  $("ed-img").hidden = true;
  $("ed-placeholder").hidden = false;
}

// render draws the test card through the draft as it is on disk
async function render() {
  const id = store.current.peek();
  const card = store.card.peek();
  const view = store.view.peek();
  if (!id || !card || !view || !view.valid) return;
  abandonRender();
  const seq = renderSeq;
  begin(`Drawing ${card.Name}`);
  try {
    const { jobId } = await api.renderDraft(id, card, store.face.peek());
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
    const img = $("ed-img");
    img.src = api.renderImageURL(jobId);
    img.hidden = false;
    $("ed-placeholder").hidden = true;
    if (done.artMissing) finish("warn", `Drew ${card.Name}`, "The art could not be fetched, so the art window is empty");
    else finish("done", `Drew ${card.Name}`, "");
  } catch (err) {
    if (seq !== renderSeq) return;
    renderJob = null;
    renderJobId = "";
    clearPreview();
    finish("error", "Could not draw the card", err.message);
  }
}

// --- drafts ---

async function loadDrafts() {
  try {
    store.drafts.value = (await api.drafts()) || [];
  } catch (err) {
    store.drafts.value = [];
    toast(`Could not read your templates: ${err.message}`, "err", 6000);
  }
}

async function loadPresets() {
  if (store.presets.peek().length) return;
  try {
    store.presets.value = (await api.testCards()) || [];
  } catch (err) {
    toast(`Could not load the preset cards: ${err.message}`, "err", 6000);
  }
}

// open shows a draft: it is read afresh, its test card is restored, and the card is
// drawn. A draft that does not load shows what is wrong instead
async function open(id) {
  const seq = ++openSeq;
  abandonRender();
  let view;
  try {
    view = await api.openDraft(id);
  } catch (err) {
    if (seq !== openSeq) return;
    toast(err.message, "err", 6000);
    await loadDrafts();
    return;
  }
  if (seq !== openSeq) return;
  await loadPresets();
  const start = startingCard(view.meta, store.presets.peek());
  batch(() => {
    store.current.value = id;
    store.view.value = view;
    store.card.value = start ? start.card : null;
    store.face.value = start ? start.face : 0;
    store.preset.value = start ? start.preset : "";
  });
  clearPreview();
  if (!view.valid) {
    finish("error", "This template does not load", "The problems are listed on the right");
    return;
  }
  render();
}

// chooseCard sets the card the draft is tested with, remembers it, and draws it
function chooseCard(card, preset) {
  const id = store.current.peek();
  if (!id) return;
  batch(() => {
    store.card.value = card;
    store.face.value = 0;
    store.preset.value = preset || "";
  });
  api.setTestCard(id, card, 0).catch(() => {
    // The card is a convenience, so a draft that cannot remember it still draws it
  });
  render();
}

// --- search ---

async function runSearch(query) {
  query = query.trim();
  if (!query) return;
  if (searchAbort) searchAbort.abort();
  const ac = (searchAbort = new AbortController());
  $("ed-results-count").textContent = "Searching…";
  try {
    const found = await api.search(query, ac.signal);
    batch(() => {
      store.results.value = found.map((r) => r.card);
      store.searched.value = query;
    });
  } catch (err) {
    if (err.name !== "AbortError") $("ed-results-count").textContent = `Search failed: ${err.message}`;
  } finally {
    if (searchAbort === ac) searchAbort = null;
  }
}

function drawResults() {
  const cards = store.results.value;
  const list = $("ed-results");
  list.replaceChildren(
    ...cards.map((c, i) =>
      h(
        "li",
        { class: "result", role: "option", "aria-selected": "false", tabIndex: i === 0 ? 0 : -1, onclick: () => chooseCard(c, "") },
        h("span", { class: "r-name" }, c.Name),
        h("span", { class: "r-meta" }, c.SetCode ? h("span", { class: "set" }, c.SetCode.toUpperCase()) : null, h("span", { class: "type" }, c.TypeLine || "")),
      ),
    ),
  );
  const q = store.searched.value;
  $("ed-results-count").textContent = q ? (cards.length ? `${cards.length} ${cards.length === 1 ? "result" : "results"}` : `No cards match ${JSON.stringify(q)}`) : "Pick a preset above, or search for a card";
}

function onResultsKey(e) {
  const rows = [...$("ed-results").querySelectorAll(".result")];
  const at = rows.indexOf(document.activeElement);
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    const next = rows[Math.min(rows.length - 1, Math.max(0, at + (e.key === "ArrowDown" ? 1 : -1)))];
    for (const r of rows) r.tabIndex = r === next ? 0 : -1;
    next?.focus();
  } else if ((e.key === "Enter" || e.key === " ") && at >= 0) {
    e.preventDefault();
    rows[at].click();
  }
}

// --- the dialog ---

let dialogAction = null;

// ask opens the one dialog that makes, copies and renames a template. confirm gets
// the name, and the resolution when the dialog has that field, and resolves when
// done. It stays open and shows the reason when confirm throws
function ask({ title, lede, confirm, name, dpi, onDone }) {
  $("draft-dialog-title").textContent = title;
  $("draft-dialog-lede").textContent = lede;
  $("draft-name").value = name;
  const withDpi = dpi !== undefined;
  $("draft-dpi-row").hidden = !withDpi;
  $("draft-dpi").value = withDpi ? String(dpi) : "";
  $("draft-confirm").textContent = confirm;
  $("draft-error").hidden = true;
  showSize();
  dialogAction = { withDpi, onDone };
  const dlg = $("draft-dialog");
  dlg.showModal();
  $("draft-name").focus();
  $("draft-name").select();
}

function showSize() {
  const { dpi, error } = parseDpi($("draft-dpi").value);
  const size = canvasSize(dpi);
  $("draft-dpi-size").textContent = error || `A canvas of ${size.width} × ${size.height} pixels`;
}

function showError(message) {
  const el = $("draft-error");
  el.textContent = message;
  el.hidden = false;
}

async function confirmDialog() {
  if (!dialogAction) return;
  const { name, error } = checkName($("draft-name").value);
  if (error) return showError(error);
  let dpi = 0;
  if (dialogAction.withDpi) {
    const parsed = parseDpi($("draft-dpi").value);
    if (parsed.error) return showError(parsed.error);
    dpi = parsed.dpi;
  }
  const btn = $("draft-confirm");
  btn.disabled = true;
  try {
    await dialogAction.onDone(name, dpi);
    $("draft-dialog").close();
  } catch (err) {
    showError(err.message);
  } finally {
    btn.disabled = false;
  }
}

// made takes up a draft the app has just made, and shows it
async function made(summary) {
  await loadDrafts();
  await open(summary.id);
  location.hash = "editor";
}

function newTemplate() {
  ask({
    title: "New template",
    lede: "A new template starts as a plain card with the standard text boxes, drawn so you can see it at once.",
    confirm: "Create",
    name: "",
    dpi: DPI_DEFAULT,
    onDone: async (name, dpi) => made(await api.createDraft(name, dpi)),
  });
}

// copyTemplate asks for a name and makes an editable copy of an installed template
function copyTemplate(template, version, title) {
  ask({
    title: "Edit a copy",
    lede: `This makes a copy of ${title || template}${version && version !== "local" ? ` ${version}` : ""} that you can change. The original stays as it is, and the copy keeps its credits and license.`,
    confirm: "Make a copy",
    name: forkName(title || template),
    onDone: async (name) => made(await api.forkDraft(template, version, name)),
  });
}

function rename() {
  const view = store.view.peek();
  if (!view) return;
  ask({
    title: "Rename the template",
    lede: "This changes the name shown in the app. The template's id stays the same.",
    confirm: "Rename",
    name: view.draft.name,
    onDone: async (name) => {
      await api.renameDraft(view.draft.id, name);
      await loadDrafts();
      store.view.value = { ...store.view.peek(), draft: { ...store.view.peek().draft, name } };
    },
  });
}

async function remove() {
  const view = store.view.peek();
  if (!view) return;
  const ok = await api.confirm("Delete this template?", `${view.draft.name} and its images will be deleted from this computer. This cannot be undone.`, "Delete");
  if (!ok) return;
  try {
    await api.deleteDraft(view.draft.id);
  } catch (err) {
    toast(err.message, "err", 6000);
    return;
  }
  abandonRender();
  batch(() => {
    store.current.value = "";
    store.view.value = null;
    store.card.value = null;
  });
  await loadDrafts();
  const next = (store.drafts.peek() || [])[0];
  if (next) await open(next.id);
}

// --- drawing ---

function drawPicker() {
  const list = store.drafts.value;
  const sel = $("ed-draft-select");
  if (!list) return;
  sel.replaceChildren(...list.map((d) => new Option(draftLabel(d), d.id)));
  sel.value = store.current.value;
}

function drawPresets() {
  const on = store.preset.value;
  $("ed-presets").replaceChildren(
    ...store.presets.value.map((p) =>
      h("button", { type: "button", class: "chip", "aria-pressed": String(p.id === on), onclick: () => chooseCard(p.card, p.id) }, p.label),
    ),
  );
}

function drawSide() {
  const view = store.view.value;
  if (!view) return;
  $("ed-about-name").textContent = view.draft.name;
  $("ed-about-id").textContent = view.draft.templateId;
  const fork = forkedFrom(view.draft);
  $("ed-about-fork").hidden = !fork;
  $("ed-about-fork").textContent = fork;
  $("ed-layouts").replaceChildren(...view.layouts.map((l) => h("li", {}, layoutLine(l))));
  const { errors, warnings } = problems(view.diagnostics);
  $("ed-problems").replaceChildren(
    ...errors.map((t) => h("li", { class: "error" }, t)),
    ...warnings.map((t) => h("li", { class: "warning" }, t)),
  );
  const n = errors.length + warnings.length;
  $("ed-problems-none").hidden = n > 0;
  $("ed-problems-count").textContent = n ? String(n) : "";
}

const GLYPH = { idle: "card", working: "render", done: "check", warn: "warn", error: "warn" };

function drawActivity() {
  const a = store.activity.value;
  const strip = $("ed-activity");
  let meta = "";
  if (a.state === "working") meta = `${Math.round(a.frac * 100)}% · ${seconds((performance.now() - a.started) / 1000)}`;
  else if (a.secs) meta = seconds(a.secs);
  strip.dataset.state = a.state;
  $("ed-activity-title").textContent = a.title;
  $("ed-activity-step").textContent = a.step || "";
  $("ed-activity-meta").textContent = meta;
  $("ed-activity-glyph").setAttribute("href", `icons.svg#i-${GLYPH[a.state]}`);
  $("ed-activity-fill").style.width = `${(a.state === "idle" ? 0 : a.state === "working" ? a.frac : 1) * 100}%`;
}

function drawChosen() {
  const card = store.card.value;
  $("ed-card-line").textContent = card ? cardLine(card) : "No card chosen";
}

// enter reads what the mode shows each time it is opened, so a draft made or
// changed elsewhere is never stale, and opens the newest draft when none is open
async function enter() {
  await Promise.all([loadDrafts(), loadPresets()]);
  const list = store.drafts.peek() || [];
  const current = store.current.peek();
  if (current && list.some((d) => d.id === current)) return;
  if (list.length) await open(list[0].id);
}

export const editor = { newTemplate, copyTemplate };

export function initEditor() {
  $("ed-empty-new").addEventListener("click", newTemplate);
  $("ed-new").addEventListener("click", newTemplate);
  $("ed-empty-copy").addEventListener("click", () => {
    location.hash = "templates";
  });
  $("ed-rename").addEventListener("click", rename);
  $("ed-delete").addEventListener("click", remove);
  $("ed-draft-select").addEventListener("change", (e) => open(e.target.value));

  $("ed-search-form").addEventListener("submit", (e) => {
    e.preventDefault();
    runSearch($("ed-search-input").value);
  });
  $("ed-search-input").addEventListener("keydown", (e) => {
    if (e.key !== "ArrowDown") return;
    const first = $("ed-results").querySelector(".result");
    if (first) {
      e.preventDefault();
      first.focus();
    }
  });
  $("ed-results").addEventListener("keydown", onResultsKey);

  $("draft-confirm").addEventListener("click", confirmDialog);
  $("draft-dpi").addEventListener("input", showSize);
  for (const id of ["draft-name", "draft-dpi"]) {
    $(id).addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        confirmDialog();
      }
    });
  }
  $("tm-new-template").addEventListener("click", newTemplate);

  effect(() => {
    const has = (store.drafts.value || []).length > 0;
    $("ed-empty").hidden = has;
    $("ed-workspace").hidden = !has;
  });
  effect(drawPicker);
  effect(drawPresets);
  effect(drawResults);
  effect(drawSide);
  effect(drawActivity);
  effect(drawChosen);
  effect(() => {
    if (app.mode.value === "editor") enter();
  });
  document.addEventListener("mimic:resolution-changed", () => {
    if (app.mode.peek() === "editor" && store.card.peek()) render();
  });
}

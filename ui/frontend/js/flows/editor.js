import { api } from "../api.js";
import { $, h, icon, isTyping } from "../dom.js";
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
} from "../editor.js";
import { asPercent, hitTest, moveRect, nudgeFor, refOf, resizeRect, snapMove, snapTargets, visibleHandles } from "../editorgeometry.js";
import { createHistory } from "../editorhistory.js";
import { afterOps, isMarked, refsFor, withMark } from "../editormarks.js";
import { deleteLabel, deleteOp, duplicateOp } from "../editorassets.js";
import { kindIcon, moveTarget, reorderTarget, rowMenu } from "../editorlayers.js";
import { closePopover, openPopover } from "../components/popover.js";
import { initSections } from "../components/sections.js";
import { LEFT_DEFAULTS, LEFT_SECTIONS } from "../sectionlayout.js";
import { createCardEditor, valuesOf } from "../components/cardEditor.js";
import { createViewport } from "../components/viewport.js";
import { initAssets } from "./editorassets.js";
import { creditAdd, creditEdit, creditRemove, displayValue, editLabel, editOps, fieldGroups, keepRatio, layerAt, parseField, readPath, shadowOps, sizeOps } from "../editorlevers.js";

// The editor: build a template and see a test card drawn with it. A draft is a
// template on disk that the app renders as it is, so everything shown here is read
// from the app and nothing is kept in the page beyond the card being tested

const store = {
  drafts: signal(null), // null until read, then the list
  current: signal(""), // the id of the draft open
  view: signal(null), // what openDraft told about it
  presets: signal([]),
  card: signal(null), // the test card, as the app sent it
  // the card form's values, which the card is edited with, and the ones it began with
  edits: signal(null),
  reference: signal({}),
  face: signal(0),
  preset: signal(""), // the id of the preset the card came from, or ""
  results: signal([]), // search results for a test card
  searched: signal(""), // the query behind the results
  activity: signal({ state: "idle", title: "Nothing drawn yet", step: "Open a template to draw a test card with it", frac: 0 }),
  // what the layout draws for the test card, from the app: { layout, delivered, items,
  // editable, reason }, or null when there is none
  layout: signal(null),
  selected: signal(""), // the ref of the item selected, or ""
  hover: signal(""),
  // the selected item as it is being dragged, { rect, guides }, or null when it is not
  live: signal(null),
  canUndo: signal(false),
  canRedo: signal(false),
  // the template's files as the app has them: { files, docs }, where docs holds each
  // one parsed, or null for one that is not JSON, and the names the fields take
  source: signal(null),
  vocab: signal(null),
  // the draft's images and fonts, from the app, or null until read
  assets: signal(null),
  // which items are locked and which ones a drag snaps to, as "layout/kind:key"
  marks: signal({ locked: [], snap: [] }),
};

const history = createHistory();
let cardEditor = null;
// the view of the card: zoom and pan, and what dragging on it does. A drag pans when the
// pan tool is chosen or Space is held over the view, and selects and moves otherwise
let viewport = null;
const tool = { pan: signal(false), space: signal(false), over: false };
const panning = () => tool.pan.peek() || tool.space.peek();
let renderTimer = 0;
let describeSeq = 0;

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
  $("ed-canvas").hidden = true;
  $("ed-placeholder").hidden = false;
  batch(() => {
    store.layout.value = null;
    store.live.value = null;
  });
}

// render draws the test card through the draft as it is on disk
async function render() {
  const id = store.current.peek();
  const card = store.card.peek();
  const view = store.view.peek();
  if (!id || !card || !view || !view.valid) return;
  abandonRender();
  const seq = renderSeq;
  // The card is named as the form has it, so a changed name is the one the strip says
  const name = (store.edits.peek() && store.edits.peek().name) || card.Name;
  begin(`Drawing ${name}`);
  try {
    const { jobId } = await api.renderDraft(id, card, store.edits.peek(), store.face.peek());
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
    $("ed-canvas").hidden = false;
    $("ed-placeholder").hidden = true;
    loadLayout();
    if (done.artMissing) finish("warn", `Drew ${name}`, "The art could not be fetched, so the art window is empty");
    else finish("done", `Drew ${name}`, "");
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

async function loadVocab() {
  if (store.vocab.peek()) return;
  try {
    store.vocab.value = await api.draftVocabulary();
  } catch (err) {
    toast(`Could not load the names a field can take: ${err.message}`, "err", 6000);
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
  history.clear();
  syncHistory();
  batch(() => {
    store.selected.value = "";
    store.current.value = id;
    store.view.value = view;
    store.card.value = start ? start.card : null;
    store.face.value = start ? start.face : 0;
    store.preset.value = start ? start.preset : "";
    setForm(start ? start.card : null);
    store.marks.value = { locked: view.meta.locked || [], snap: view.meta.snapTo || [] };
  });
  savedMarks = JSON.stringify([id, store.marks.peek()]);
  clearPreview();
  if (viewport) viewport.reset();
  store.source.value = null;
  store.assets.value = null;
  loadSource();
  loadAssets();
  if (!view.valid) {
    finish("error", "This template does not load", "The problems are listed on the right");
    return;
  }
  // A template tested with nothing yet starts with the first preset, which is looked up
  if (start && !start.card) {
    const first = store.presets.peek().find((p) => p.id === start.preset);
    if (first) choosePreset(first);
    return;
  }
  render();
}

// setForm fills the card form with a card's own values, as the card it is measured
// against, so nothing in it is changed yet
function setForm(card) {
  const values = card ? valuesOf(card) : {};
  store.reference.value = values;
  store.edits.value = card ? values : null;
  appliedEdits = JSON.stringify(store.edits.peek());
}

let appliedEdits = "";
let cardTimer = 0;

// keepCardEdits keeps the card with the changes made in the form and draws it, a moment
// after the changes stop. The form's own values start as the card's, and a change from
// the card is the person's
function keepCardEdits() {
  const key = JSON.stringify(store.edits.value);
  if (!store.card.peek() || key === appliedEdits) return;
  clearTimeout(cardTimer);
  cardTimer = setTimeout(() => {
    appliedEdits = key;
    const id = store.current.peek();
    if (!id || !store.card.peek()) return;
    api.setTestCard(id, store.card.peek(), store.edits.peek(), store.face.peek()).catch(() => {});
    render();
  }, 350);
}

let presetSeq = 0;

// choosePreset looks up the card a preset names and makes it the test card. A lookup that
// fails is said, and a newer choice made while it waits is the one that stands
async function choosePreset(p) {
  const id = store.current.peek();
  if (!id) return;
  const seq = ++presetSeq;
  abandonRender();
  begin(`Looking up ${p.name}`);
  try {
    const found = await api.resolveTestCard(p.id);
    if (seq !== presetSeq || id !== store.current.peek()) return;
    chooseCard(found, p.id);
  } catch (err) {
    if (seq !== presetSeq) return;
    finish("error", `Could not look up ${p.name}`, err.message);
    toast(err.message, "err", 6000);
  }
}

// chooseCard sets the card the draft is tested with, remembers it, and draws it
function chooseCard(card, preset) {
  const id = store.current.peek();
  if (!id) return;
  presetSeq++;
  batch(() => {
    store.card.value = card;
    store.face.value = 0;
    store.preset.value = preset || "";
    setForm(card);
  });
  api.setTestCard(id, card, store.edits.peek(), 0).catch(() => {
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
    store.edits.value = null;
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
  // A preset is the card as it was made, so one whose fields have been changed is no
  // longer the one in use
  const on = cardEditor && cardEditor.dirty.value ? "" : store.preset.value;
  $("ed-presets").replaceChildren(
    ...store.presets.value.map((p) =>
      h("button", { type: "button", class: "chip", "aria-pressed": String(p.id === on), onclick: () => choosePreset(p) }, p.label),
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
  // The count is seen with the section collapsed, so it says whether any are errors
  $("ed-problems-count").dataset.level = errors.length ? "error" : warnings.length ? "warning" : "";
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
  const edits = store.edits.value;
  // The line names the card as the form has it, so a changed name or set shows at once
  const shown = card && edits ? { ...card, Name: edits.name ?? card.Name, SetCode: edits.setCode ?? card.SetCode } : card;
  $("ed-card-line").textContent = shown ? cardLine(shown) : "No card chosen";
}

// --- the layout on the preview ---

const KIND_LABEL = { layer: "Layer", text: "Text box", art: "Art window", symbol: "Symbol" };

// nameOf is what an item is called in the lists
function nameOf(item) {
  if (item.kind === "art") return "Art window";
  if (item.kind === "symbol" && item.key === "set") return "Set symbol";
  return item.key;
}

const itemOf = (view, ref) => (view && ref ? view.items.find((it) => refOf(it) === ref) : null);
const selectedItem = () => itemOf(store.layout.peek(), store.selected.peek());

// loadLayout asks the app what the layout draws for the test card, which is where the
// selection comes from. It is asked again after each edit and each render
async function loadLayout() {
  const id = store.current.peek();
  const card = store.card.peek();
  if (!id || !card) return;
  const seq = ++describeSeq;
  try {
    const view = await api.describeDraft(id, card, store.edits.peek(), store.face.peek());
    if (seq !== describeSeq || id !== store.current.peek()) return;
    batch(() => {
      store.layout.value = view;
      // A selection that is no longer drawn is let go
      if (store.selected.peek() && !itemOf(view, store.selected.peek())) store.selected.value = "";
    });
  } catch {
    if (seq === describeSeq) store.layout.value = null;
  }
}

// adopt takes up what the app says of the draft after an edit: whether it loads and
// what is found in it. An edit to the canvas or the template's own fields changes what
// the details show, which are read again
async function adopt(id, res, ops) {
  if (id !== store.current.peek()) return;
  const view = store.view.peek();
  if (view) store.view.value = { ...view, valid: res.valid, diagnostics: res.diagnostics || [] };
  if (!res.valid) {
    abandonRender();
    clearPreview();
    finish("error", "This template does not load", "The problems are listed on the right");
  }
  if (ops.some((op) => op.kind === "canvas" || op.kind === "template" || op.op === "replace")) await refreshView();
}

// refreshView reads the draft's details again, and the list it is in
async function refreshView() {
  const id = store.current.peek();
  if (!id) return;
  try {
    const [view] = await Promise.all([api.openDraft(id), loadDrafts()]);
    if (id !== store.current.peek()) return;
    // The card and what is open stay as they are, and the layouts, the name and the
    // problems are what changed
    const was = store.view.peek();
    store.view.value = { ...view, meta: was ? was.meta : view.meta, valid: was ? was.valid : view.valid, diagnostics: was ? was.diagnostics : view.diagnostics };
  } catch {
    // The details are a convenience
  }
}

async function loadSource() {
  const id = store.current.peek();
  if (!id) return;
  try {
    const src = await api.draftSource(id);
    if (id !== store.current.peek()) return;
    const docs = {};
    for (const f of src.files) {
      try {
        docs[f.name] = JSON.parse(f.content);
      } catch {
        docs[f.name] = null;
      }
    }
    store.source.value = { files: src.files, docs };
  } catch {
    store.source.value = null;
  }
}

async function loadAssets() {
  const id = store.current.peek();
  if (!id) return;
  try {
    const assets = await api.draftAssets(id);
    if (id === store.current.peek()) store.assets.value = assets;
  } catch {
    if (id === store.current.peek()) store.assets.value = null;
  }
}

// changesAssets says whether edits can change what the draft holds in images, fonts and
// parts, so they are read again
const changesAssets = (ops) =>
  ops.some((op) => ["put", "drop", "add", "delete", "duplicate", "replace"].includes(op.op) || op.kind === "template" || op.kind === "part");

function select(ref) {
  flushNudge();
  store.selected.value = ref || "";
}

// --- locking and snapping ---

let savedMarks = "";

// saveMarks keeps the marks with the draft, when they are not what it has already
function saveMarks() {
  const id = store.current.peek();
  const marks = store.marks.peek();
  const text = JSON.stringify([id, marks]);
  if (!id || text === savedMarks) return;
  savedMarks = text;
  api.setDraftMarks(id, marks.locked, marks.snap).catch((err) => toast(err.message, "err", 6000));
}

// lockedRefs are the items of the layout shown that cannot be picked on the card
const lockedRefs = (view) => refsFor(store.marks.peek().locked, view.layout);

const isLocked = (view, item) => !!view && !!item && isMarked(store.marks.peek().locked, view.layout, refOf(item));

// setMark locks or unlocks the selection, or makes it a line others snap to or not
function setMark(which, on) {
  setMarkOf(selectedItem(), which, on);
}

// setMarkOf does the same for any item in the list
function setMarkOf(item, which, on) {
  const view = store.layout.peek();
  if (!view || !item) return;
  flushNudge();
  const marks = store.marks.peek();
  store.marks.value = { ...marks, [which]: withMark(marks[which], view.layout, refOf(item), on) };
  saveMarks();
}

// --- edits ---

let chain = Promise.resolve();

// scheduleRender draws the card again once the edits stop for a moment
function scheduleRender() {
  clearTimeout(renderTimer);
  renderTimer = setTimeout(render, 150);
}

function syncHistory() {
  store.canUndo.value = history.canUndo;
  store.canRedo.value = history.canRedo;
}

// settle takes up the draft after edits were made, however they were made: what the
// app says of it, what it draws, and what it holds
async function settle(id, res, ops) {
  const marks = afterOps(store.marks.peek(), ops);
  if (JSON.stringify(marks) !== JSON.stringify(store.marks.peek())) {
    store.marks.value = marks;
    saveMarks();
  }
  await adopt(id, res, ops);
  const reads = [loadLayout(), loadSource()];
  if (changesAssets(ops)) reads.push(loadAssets());
  await Promise.all(reads);
  scheduleRender();
}

// commit makes edits to the draft, one batch after another, and remembers how to take
// them back. merge marks an edit that joins the one before it when they are close.
// folded is what an earlier change that belongs to this one undoes, so the two are one
// step of undo. It resolves to whether the edits were made
function commit(ops, label, merge, folded) {
  const id = store.current.peek();
  const run = async () => {
    try {
      const res = await api.applyDraft(id, ops);
      if (folded) history.record({ ops: [], inverse: [...res.inverse, ...folded], label });
      else history.record({ ops, inverse: res.inverse, label, merge });
      syncHistory();
      await settle(id, res, ops);
      return true;
    } catch (err) {
      toast(err.message, "err", 6000);
      await loadLayout();
      return false;
    } finally {
      store.live.value = null;
    }
  };
  chain = chain.then(run, run);
  return chain;
}

// recordApplied remembers a change the app made on its own, such as an image brought
// in, which it describes by the edits that undo it. The edits that make it again are
// known once it has been undone
function recordApplied(res, label, ops = [{ op: "put" }]) {
  const id = store.current.peek();
  history.record({ ops: [], inverse: res.inverse, label });
  syncHistory();
  const run = () => settle(id, res, ops);
  chain = chain.then(run, run);
  return chain;
}

// replay makes the edits of an undo or a redo, and keeps what the app says undoes
// them. One the app refuses leaves the history where it was
function replay(entry, undoing) {
  const id = store.current.peek();
  const ops = undoing ? entry.inverse : entry.ops;
  const run = async () => {
    try {
      const res = await api.applyDraft(id, ops);
      if (undoing) entry.ops = res.inverse;
      else entry.inverse = res.inverse;
      await settle(id, res, ops);
    } catch (err) {
      toast(err.message, "err", 6000);
      if (undoing) history.redo();
      else history.undo();
    }
    syncHistory();
  };
  chain = chain.then(run, run);
  return chain;
}

function undo() {
  flushNudge();
  const entry = history.undo();
  if (!entry) return;
  syncHistory();
  app.status.value = `Undid: ${entry.label}`;
  replay(entry, true);
}

function redo() {
  flushNudge();
  const entry = history.redo();
  if (!entry) return;
  syncHistory();
  app.status.value = `Redid: ${entry.label}`;
  replay(entry, false);
}

// --- nudging ---

let nudge = null;

// nudgeSelected moves the selection a little, showing it at once and saving the whole
// run of keys as one edit once they stop
function nudgeSelected(dx, dy) {
  const item = selectedItem();
  const view = store.layout.peek();
  if (!item || !view || !view.editable || isLocked(view, item)) return;
  if (nudge && nudge.ref !== refOf(item)) flushNudge();
  nudge = nudge || { ref: refOf(item), item, dx: 0, dy: 0, timer: 0 };
  nudge.dx += dx;
  nudge.dy += dy;
  store.live.value = { rect: moveRect(item.delivered, nudge.dx, nudge.dy), guides: [] };
  clearTimeout(nudge.timer);
  nudge.timer = setTimeout(flushNudge, 250);
}

function flushNudge() {
  if (!nudge) return;
  const n = nudge;
  nudge = null;
  clearTimeout(n.timer);
  if (!n.dx && !n.dy) {
    store.live.value = null;
    return;
  }
  const view = store.layout.peek();
  commit([{ op: "move", layout: view.layout, kind: n.item.kind, key: n.item.key, dx: n.dx, dy: n.dy }], `Move ${nameOf(n.item)}`, "nudge");
}

// --- the pointer ---

let gesture = null;

// A press that moves less than this many pixels on screen is a click
const CLICK_SLOP = 3;
// Edges snap to a line within this many pixels on screen
const SNAP_PX = 6;

function overlayBox() {
  return $("ed-overlay").getBoundingClientRect();
}

// nativePoint is where an event is on the finished image, in its own pixels
function nativePoint(e) {
  const r = overlayBox();
  const c = store.layout.peek().delivered;
  return { x: ((e.clientX - r.left) * c.width) / r.width, y: ((e.clientY - r.top) * c.height) / r.height };
}

function onPointerDown(e) {
  const view = store.layout.peek();
  // A drag that pans is the viewport's, which hears it from the stage
  if (e.button !== 0 || !view || panning()) return;
  flushNudge();
  const overlay = $("ed-overlay");
  const handle = e.target.closest && e.target.closest(".ed-handle");
  const current = selectedItem();
  if (handle && current && view.editable && !isLocked(view, current)) {
    gesture = { kind: "resize", handle: handle.dataset.handle, item: current, start: { x: e.clientX, y: e.clientY }, rect0: current.delivered, moved: false };
  } else {
    const p = nativePoint(e);
    const hit = hitTest(view.items, p, lockedRefs(view));
    store.selected.value = hit ? refOf(hit) : "";
    if (!hit) return;
    gesture = { kind: view.editable ? "move" : "click", item: hit, start: { x: e.clientX, y: e.clientY }, rect0: hit.delivered, moved: false, p };
  }
  try {
    // Capture keeps the drag going when the pointer leaves the card. A press with no
    // pointer behind it, such as one a script sends, has nothing to capture
    overlay.setPointerCapture(e.pointerId);
  } catch {
    // The gesture still ends on the overlay's own events
  }
  e.preventDefault();
}

function onPointerMove(e) {
  const view = store.layout.peek();
  if (!view) return;
  if (!gesture) {
    const hit = e.buttons || panning() ? null : hitTest(view.items, nativePoint(e), lockedRefs(view));
    store.hover.value = hit ? refOf(hit) : "";
    return;
  }
  const g = gesture;
  const sx = e.clientX - g.start.x;
  const sy = e.clientY - g.start.y;
  if (!g.moved && Math.hypot(sx, sy) < CLICK_SLOP) return;
  g.moved = true;
  if (g.kind === "click") return;
  const r = overlayBox();
  const per = view.delivered.width / r.width; // pixels of the finished image for one on screen
  let dx = Math.round(sx * per);
  let dy = Math.round(sy * per);
  $("ed-overlay").classList.add("moving");
  if (g.kind === "resize") {
    store.live.value = { rect: resizeRect(g.rect0, g.handle, dx, dy), guides: [] };
    return;
  }
  let guides = [];
  if (!e.altKey) {
    const snapped = snapMove(g.rect0, dx, dy, snapTargets(view.items, view.delivered, refOf(g.item), nameOf, refsFor(store.marks.peek().snap, view.layout)), SNAP_PX * per);
    ({ dx, dy, guides } = snapped);
  }
  store.live.value = { rect: moveRect(g.rect0, dx, dy), guides, dx, dy };
}

function endGesture() {
  gesture = null;
  $("ed-overlay").classList.remove("moving");
}

function onPointerUp(e) {
  const g = gesture;
  if (!g) return;
  const view = store.layout.peek();
  endGesture();
  if ($("ed-overlay").hasPointerCapture?.(e.pointerId)) $("ed-overlay").releasePointerCapture(e.pointerId);
  if (!g.moved) return;
  const live = store.live.peek();
  if (!live || g.kind === "click") {
    store.live.value = null;
    return;
  }
  const target = { layout: view.layout, kind: g.item.kind, key: g.item.key };
  if (g.kind === "move" && (live.dx || live.dy)) {
    commit([{ op: "move", ...target, dx: live.dx, dy: live.dy }], `Move ${nameOf(g.item)}`, "");
  } else if (g.kind === "resize") {
    commit([{ op: "resize", ...target, rect: live.rect }], `Resize ${nameOf(g.item)}`, "");
  } else {
    store.live.value = null;
  }
}

function cancelGesture() {
  endGesture();
  store.live.value = null;
}

// layersOf are the layers of the layout shown, from the bottom of the stack
const layersOf = (view) => view.items.filter((it) => it.kind === "layer");

// moveLayer puts a layer at a place in the stack, where 0 is the bottom
function moveLayer(item, to) {
  const view = store.layout.peek();
  if (!view || !view.editable || item.kind !== "layer" || to === null) return;
  const at = layersOf(view).findIndex((it) => refOf(it) === refOf(item));
  commit([{ op: "reorder", layout: view.layout, kind: "layer", key: item.key, to }], `${to > at ? "Raise" : "Lower"} ${nameOf(item)}`, "");
}

// shiftLayer raises a layer a place when by is 1, and lowers it when it is -1
function shiftLayer(item, by) {
  const view = store.layout.peek();
  if (!view) return;
  const layers = layersOf(view);
  moveLayer(item, moveTarget(layers.findIndex((it) => refOf(it) === refOf(item)), layers.length, by));
}

// the keys of the layers or the text boxes in the layout shown, which a new one must not use
function takenKeys(view, kind) {
  return view.items.filter((it) => it.kind === kind).map((it) => it.key);
}

function duplicateItem(item) {
  const view = store.layout.peek();
  if (!view || !item || !view.editable || item.kind === "symbol") return;
  const op = duplicateOp(view, item, takenKeys(view, item.kind));
  const ref = `${item.kind}:${item.kind === "art" ? Number(item.key) + 1 : op.as}`;
  commit([op], `Duplicate ${nameOf(item)}`, "").then((made) => {
    if (made) select(ref);
  });
}

function deleteItem(item) {
  const view = store.layout.peek();
  if (!view || !item || !view.editable || item.kind === "symbol") return;
  commit([deleteOp(view, item)], deleteLabel(item, nameOf(item)), "").then((gone) => {
    if (gone && store.selected.peek() === refOf(item)) select("");
  });
}

function onKey(e) {
  if (app.mode.peek() !== "editor" || e.defaultPrevented || isTyping() || document.querySelector("dialog[open]")) return;
  const mod = e.metaKey || e.ctrlKey;
  if (mod && !e.altKey && e.key.toLowerCase() === "z") {
    e.preventDefault();
    if (e.shiftKey) redo();
    else undo();
    return;
  }
  if (mod && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "y") {
    e.preventDefault();
    redo();
    return;
  }
  if (mod || e.altKey) return;
  if (e.key === "Escape") {
    if (gesture) cancelGesture();
    else if (store.selected.peek()) select("");
    return;
  }
  // The view: plus and minus step the zoom, and 0, 1 and 2 are fit, 100% and 200%, as in Single
  const zoomKeys = { "+": () => viewport.step(1), "=": () => viewport.step(1), "-": () => viewport.step(-1), 0: () => viewport.reset(), 1: () => viewport.setZoom(1), 2: () => viewport.setZoom(2) };
  if (viewport && zoomKeys[e.key]) {
    e.preventDefault();
    zoomKeys[e.key]();
    return;
  }
  if (e.key.toLowerCase() === "l" && !e.shiftKey && store.selected.peek()) {
    e.preventDefault();
    const view = store.layout.peek();
    setMark("locked", !isLocked(view, selectedItem()));
    return;
  }
  const n = nudgeFor(e.key, e.shiftKey);
  if (n && store.selected.peek()) {
    e.preventDefault();
    nudgeSelected(n.dx, n.dy);
  }
}

// --- drawing the layout ---

function drawOverlay() {
  const view = store.layout.value;
  const sel = store.selected.value;
  const hover = store.hover.value;
  const live = store.live.value;
  store.marks.value;
  const overlay = $("ed-overlay");
  if (!view) {
    overlay.replaceChildren();
    return;
  }
  const canvas = view.delivered;
  const box = (rect, cls, ...children) => {
    const p = asPercent(rect, canvas);
    return h("div", { class: `ed-box ${cls}`, style: `left:${p.left}%;top:${p.top}%;width:${p.width}%;height:${p.height}%` }, ...children);
  };
  const out = [];
  const over = hover && hover !== sel ? itemOf(view, hover) : null;
  if (over && over.drawn) out.push(box(over.delivered, "hover"));
  const item = itemOf(view, sel);
  if (item) {
    const rect = live ? live.rect : item.delivered;
    if (rect.width > 0 && rect.height > 0) {
      let handles = [];
      // A layer has no size to change, and neither does a text box shown by its ink
      const locked = isLocked(view, item);
      if (view.editable && item.kind !== "layer" && !item.ink && !locked) {
        const per = overlayBox().width / canvas.width;
        handles = visibleHandles(rect.width * per, rect.height * per).map((name) => h("div", { class: "ed-handle", dataset: { handle: name } }));
      }
      out.push(box(rect, `sel${item.drawn ? "" : " idle"}${locked ? " locked" : ""}`, ...handles));
    }
  }
  if (live) {
    // The items a guide lines the dragged one up with are outlined too
    const seen = new Set();
    for (const g of live.guides) {
      for (const r of g.boxes || []) {
        const key = `${r.x},${r.y},${r.width},${r.height}`;
        if (!seen.has(key)) out.push(box(r, "snap"));
        seen.add(key);
      }
    }
    for (const g of live.guides) {
      const at = g.axis === "x" ? (g.at / canvas.width) * 100 : (g.at / canvas.height) * 100;
      const label = g.to ? `${g.edge} to ${g.to}` : g.edge;
      out.push(h("div", { class: `ed-guide ${g.axis}`, style: g.axis === "x" ? `left:${at}%` : `top:${at}%` }, h("span", { class: "ed-guide-label" }, label)));
    }
  }
  overlay.replaceChildren(...out);
  overlay.classList.toggle("can-move", view.editable && !!hover);
}

function drawLayers() {
  const view = store.layout.value;
  const marks = store.marks.value;
  const list = $("ed-layers");
  list.hidden = !view;
  $("ed-add").disabled = !view || !view.editable;
  $("ed-layers-none").hidden = !!view;
  if (!view) {
    list.replaceChildren();
    return;
  }
  const layers = layersOf(view);
  list.replaceChildren(...[...view.items].reverse().map((it) => layerRow(view, marks, it, layers)));
  markSelected(store.selected.peek());
}

// layerRow is one row of the list: a grip to drag a layer by, an icon for what the item
// is, its name, which selects it, and the toggles for locking it and snapping to it
// and its menu
function layerRow(view, marks, item, layers) {
  const ref = refOf(item);
  const name = nameOf(item);
  const locked = isMarked(marks.locked, view.layout, ref);
  const snaps = isMarked(marks.snap, view.layout, ref);
  const draggable = item.kind === "layer" && view.editable && layers.length > 1;
  const toggle = (which, on, glyphOn, glyphOff, label, tip) =>
    h(
      "button",
      { class: "ed-row-btn", type: "button", "aria-pressed": String(on), "aria-label": label, "data-tip": tip, dataset: { toggle: which }, onclick: () => setMarkOf(item, which, !on) },
      icon(on ? glyphOn : glyphOff),
    );
  const at = layers.findIndex((it) => refOf(it) === ref);
  const menu = rowMenu(item, { editable: view.editable, at, layers: layers.length });
  const more = h(
    "button",
    {
      class: `ed-row-btn${menu.length ? "" : " empty"}`,
      type: "button",
      "aria-label": `More for ${name}`,
      "aria-haspopup": "menu",
      "aria-expanded": "false",
      "data-tip": "More",
      disabled: !menu.length,
      dataset: { menu: "more" },
      onclick: (e) => openRowMenu(e.currentTarget, item),
    },
    icon("more"),
  );
  return h(
    "li",
    {
      class: `ed-layer${item.drawn ? "" : " undrawn"}`,
      title: item.why || "",
      dataset: { ref },
      // A press on the row away from its controls selects it too
      onclick: (e) => {
        if (!e.target.closest("button, .ed-layer-grip")) select(ref);
      },
    },
    draggable
      ? h("span", { class: "ed-layer-grip", "aria-hidden": "true", "data-tip": "Drag to move", onpointerdown: (e) => startRowDrag(e, item) }, icon("grip"))
      : h("span", { class: "ed-layer-space", "aria-hidden": "true" }),
    h(
      "button",
      { class: "ed-layer-select", type: "button", "aria-current": "false", onclick: () => select(ref) },
      h("span", { class: "ed-layer-kind", role: "img", "aria-label": KIND_LABEL[item.kind] || item.kind, title: KIND_LABEL[item.kind] || item.kind }, icon(kindIcon(item.kind))),
      h("span", { class: "ed-layer-name" }, name),
    ),
    toggle("locked", locked, "lock", "lock-open", `Lock ${name} on the card`, locked ? "Locked, so the card cannot pick or move it" : "Lock on the card"),
    toggle("snap", snaps, "grid-on", "grid-off", `Snap other items to ${name}`, snaps ? "Others snap to it" : "Snap other items to it"),
    more,
  );
}

// openRowMenu shows what can be done to an item of the list
function openRowMenu(anchor, item) {
  const view = store.layout.peek();
  if (!view) return;
  const layers = layersOf(view);
  const entries = rowMenu(item, { editable: view.editable, at: layers.findIndex((it) => refOf(it) === refOf(item)), layers: layers.length });
  const act = {
    duplicate: () => duplicateItem(item),
    delete: () => deleteItem(item),
    raise: () => shiftLayer(item, 1),
    lower: () => shiftLayer(item, -1),
  };
  const body = h(
    "div",
    { class: "menu", role: "menu" },
    ...entries.map((m) =>
      h(
        "button",
        {
          class: "menu-item",
          type: "button",
          role: "menuitem",
          onclick: () => {
            closePopover();
            act[m.id]();
          },
        },
        m.label,
      ),
    ),
  );
  openPopover(anchor, body, { align: "end" });
}

// --- dragging a layer by its grip ---

let rowDrag = null;

function startRowDrag(e, item) {
  if (e.button !== 0) return;
  const view = store.layout.peek();
  if (!view || !view.editable) return;
  e.preventDefault();
  const grip = e.currentTarget;
  const rows = [...$("ed-layers").querySelectorAll(".ed-layer")].filter((row) => itemOf(view, row.dataset.ref)?.kind === "layer");
  const from = rows.findIndex((row) => row.dataset.ref === refOf(item));
  if (from < 0) return;
  rowDrag = { item, rows, from, slot: null, to: null };
  try {
    grip.setPointerCapture(e.pointerId);
  } catch {
    // The drag still follows the events the grip is sent
  }
  rows[from].classList.add("dragging");
  $("ed-layers").classList.add("dragging");
  grip.addEventListener("pointermove", moveRowDrag);
  grip.addEventListener("pointerup", endRowDrag);
  grip.addEventListener("pointercancel", cancelRowDrag);
}

// showDrop draws the line where the dragged layer would land
function showDrop() {
  const { rows, slot } = rowDrag;
  rows.forEach((row, i) => {
    row.classList.toggle("drop-before", slot === i && rowDrag.to !== null);
    row.classList.toggle("drop-after", slot === rows.length && i === rows.length - 1 && rowDrag.to !== null);
  });
}

function moveRowDrag(e) {
  if (!rowDrag) return;
  const centers = rowDrag.rows.map((row) => {
    const r = row.getBoundingClientRect();
    return r.top + r.height / 2;
  });
  const target = reorderTarget(centers, e.clientY, rowDrag.from);
  rowDrag.slot = target ? target.slot : null;
  rowDrag.to = target ? target.to : null;
  showDrop();
}

function finishRowDrag(e) {
  const d = rowDrag;
  rowDrag = null;
  const grip = e.currentTarget;
  grip.removeEventListener("pointermove", moveRowDrag);
  grip.removeEventListener("pointerup", endRowDrag);
  grip.removeEventListener("pointercancel", cancelRowDrag);
  if (grip.hasPointerCapture?.(e.pointerId)) grip.releasePointerCapture(e.pointerId);
  $("ed-layers").classList.remove("dragging");
  d.rows.forEach((row) => row.classList.remove("dragging", "drop-before", "drop-after"));
  return d;
}

function endRowDrag(e) {
  if (!rowDrag) return;
  const d = finishRowDrag(e);
  if (d.to !== null) moveLayer(d.item, d.to);
}

function cancelRowDrag(e) {
  if (rowDrag) finishRowDrag(e);
}

// markSelected shows which row is selected without rebuilding the list, so a row
// keeps the keyboard focus it has. The selected row, or the first when none is, has
// the tab stops, and the arrow keys move between rows
function markSelected(sel) {
  const rows = [...$("ed-layers").querySelectorAll(".ed-layer")];
  rows.forEach((row, i) => {
    const on = row.dataset.ref === sel;
    const active = on || (!sel && i === 0);
    row.classList.toggle("selected", on);
    row.querySelector(".ed-layer-select").setAttribute("aria-current", String(on));
    for (const control of row.querySelectorAll("button")) control.tabIndex = active ? 0 : -1;
  });
}

function onLayersKey(e) {
  const selectButton = e.target.closest?.(".ed-layer-select");
  if (!selectButton) return;
  const rows = [...$("ed-layers").querySelectorAll(".ed-layer")];
  const row = selectButton.closest(".ed-layer");
  const at = rows.indexOf(row);
  if (e.altKey && !e.shiftKey && !e.metaKey && !e.ctrlKey && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
    // Alt with an arrow moves a layer in the stack, which is what dragging does
    e.preventDefault();
    e.stopPropagation();
    const item = itemOf(store.layout.peek(), row.dataset.ref);
    if (item && item.kind === "layer") shiftLayer(item, e.key === "ArrowUp" ? 1 : -1);
    return;
  }
  // An arrow with a modifier is a nudge, which the page handles wherever the focus is
  if (e.shiftKey || e.altKey || e.metaKey || e.ctrlKey) return;
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    e.stopPropagation();
    const next = rows[Math.min(rows.length - 1, Math.max(0, at + (e.key === "ArrowDown" ? 1 : -1)))];
    if (next) {
      select(next.dataset.ref);
      next.querySelector(".ed-layer-select").focus();
    }
  }
}

function drawSelection() {
  const view = store.layout.value;
  const item = itemOf(view, store.selected.value);
  const live = store.live.value;
  const marks = store.marks.value;
  $("ed-selection-none").hidden = !!item;
  $("ed-selection").hidden = !item;
  $("ed-edit-note").hidden = !(view && !view.editable);
  $("ed-edit-note").textContent = view && !view.editable ? view.reason : "";
  if (!item) return;
  const rect = live ? live.rect : item.delivered;
  $("ed-selection-name").textContent = nameOf(item);
  const locked = isMarked(marks.locked, view.layout, refOf(item));
  const kind = item.drawn ? KIND_LABEL[item.kind] : `${KIND_LABEL[item.kind]}, not drawn: ${item.why}`;
  $("ed-selection-kind").textContent = locked ? `${kind}. Locked, so the card cannot pick or move it` : kind;
  const blank = rect.width === 0 && rect.height === 0;
  $("ed-sel-x").textContent = blank ? "" : String(rect.x);
  $("ed-sel-y").textContent = blank ? "" : String(rect.y);
  $("ed-sel-w").textContent = blank ? "" : String(rect.width);
  $("ed-sel-h").textContent = blank ? "" : String(rect.height);
}

// --- the levers ---

// How long typing in a field waits before the change is made
const LEVER_DELAY_MS = 400;

const layoutFileOf = (view) => `layouts/${view.layout}.json`;

// entryOf finds a layer in a layout's stack by its id, or its part's name
function entryOf(layoutDoc, key) {
  return ((layoutDoc && layoutDoc.stack) || []).find((e) => (e.id || e.part) === key) || null;
}

// nodeOf is the part of the files an item's fields are in, with what else its fields
// need to be read and written
function nodeOf(item, view, src) {
  const layoutDoc = src.docs[layoutFileOf(view)];
  const template = src.docs["template.json"];
  if (!layoutDoc) return null;
  switch (item.kind) {
    case "text":
      return { node: (layoutDoc.textBoxes || {})[item.key], layoutDoc, template };
    case "art":
      return { node: (layoutDoc.art || [])[Number(item.key)], layoutDoc, template };
    case "symbol":
      return { node: (layoutDoc.symbols || {})[item.key], layoutDoc, template };
    case "layer": {
      const entry = entryOf(layoutDoc, item.key);
      const part = entry && template && template.parts ? template.parts[entry.part] : null;
      return { node: entry, layoutDoc, template, part: entry ? entry.part : "", partNode: part };
    }
  }
  return null;
}

// A form is built once for what it edits, and its fields are filled from the files
// each time they change, except one the person is typing in
let leverForms = [];

function field(label, id, control, extra) {
  return h("div", { class: "ed-lever" }, h("label", { for: id }, label), control, extra);
}

// control makes the input for a field
function controlFor(f, id) {
  if (f.type === "choice") {
    const sel = h("select", { id });
    if (!f.required) sel.append(new Option(f.empty ? `Default (${f.empty})` : "Default", ""));
    for (const o of f.options) sel.append(new Option(o.label, o.value));
    return sel;
  }
  return h("input", { id, type: "text", autocomplete: "off", spellcheck: "false", inputMode: f.type === "int" ? "numeric" : f.type === "number" ? "decimal" : "text" });
}

// buildForm draws the fields of some groups and returns what keeps them in step with
// the files. spec says where a field's value is, what an edit of it is made of and
// whether the form can be changed
function buildForm(container, scope, groups, spec) {
  const form = { controls: [], spec, groups: [] };
  const nodes = groups.map((g) => {
    const box = h("div", { class: "ed-lever-group" });
    const title = h("div", { class: "ed-lever-title" }, g.title);
    const rows = h("div", { class: "ed-lever-group" });
    const entry = { group: g, rows };
    if (g.shadow) {
      const toggle = h("input", { id: `${scope}-shadow-on`, type: "checkbox" });
      toggle.addEventListener("change", () => {
        const ops = shadowOps(spec.target, toggle.checked);
        // A checkbox does not always take the focus when it is clicked, so it is marked
        // as waiting for its change to be saved, and the files' old state is not put
        // back over it in the meantime
        toggle.dataset.pending = "1";
        commit(ops, `${toggle.checked ? "Add" : "Remove"} the shadow of ${spec.target.key}`, "").finally(() => {
          delete toggle.dataset.pending;
          fill(form);
        });
      });
      entry.toggle = toggle;
      box.append(h("label", { class: "ed-shadow-toggle", for: toggle.id }, toggle, g.title));
    } else {
      box.append(title);
    }
    const made = [];
    for (const f of g.fields) {
      const id = `${scope}-${f.id}`;
      const c = controlFor(f, id);
      const err = h("p", { class: "ed-lever-error", role: "alert", hidden: true });
      const ctl = { f, c, err, timer: 0 };
      form.controls.push(ctl);
      if (f.type === "choice") c.addEventListener("change", () => apply(form, ctl));
      else {
        c.addEventListener("input", () => {
          clearTimeout(ctl.timer);
          ctl.timer = setTimeout(() => apply(form, ctl), LEVER_DELAY_MS);
        });
        c.addEventListener("change", () => {
          clearTimeout(ctl.timer);
          apply(form, ctl);
        });
      }
      made.push([f.id, field(f.label, id, c, err)]);
    }
    // A pair of fields that can be kept in proportion is drawn with a bracket joining
    // them and a padlock on it, which is the one a layer's row has
    const pair = g.ratio && spec.ratio ? made.filter(([id]) => g.ratio.includes(id)) : [];
    for (const [id, el] of made) {
      if (pair.length === 2 && id === pair[0][0]) rows.append(ratioPair(form, spec, pair.map(([, row]) => row)));
      else if (!pair.some(([pid]) => pid === id)) rows.append(el);
    }
    box.append(rows);
    form.groups.push(entry);
    return box;
  });
  container.replaceChildren(...nodes);
  return form;
}

// ratioPair is two rows of fields with a bracket from the middle of one to the middle of
// the other, and a padlock on the bracket. Locked, a change to one is made to the other
// as well so the two stay in proportion
function ratioPair(form, spec, rows) {
  const lock = h("button", { class: "ed-row-btn ed-ratio-lock", type: "button", "aria-pressed": "false", "aria-label": "Keep the width and height in proportion", dataset: { ratio: "lock" } });
  lock.addEventListener("click", () => {
    const now = spec.ratio.get();
    if (now.locked) {
      spec.ratio.set({ locked: false });
    } else {
      const w = valueIn(form, form.controls.find((c) => c.f.id === "width").f);
      const ht = valueIn(form, form.controls.find((c) => c.f.id === "height").f);
      if (!(w > 0 && ht > 0)) return;
      spec.ratio.set({ locked: true, ratio: w / ht });
    }
    syncRatio(form);
  });
  form.ratioLock = lock;
  return h("div", { class: "ed-ratio" }, h("div", { class: "ed-ratio-rows" }, ...rows), h("div", { class: "ed-ratio-link" }, h("span", { class: "ed-ratio-bracket", "aria-hidden": "true" }), lock));
}

// syncRatio shows whether the proportion is kept, as a layer's padlock shows it is locked
function syncRatio(form) {
  if (!form.ratioLock) return;
  const on = !!form.spec.ratio.get().locked;
  form.ratioLock.setAttribute("aria-pressed", String(on));
  form.ratioLock.setAttribute("data-tip", on ? "Width and height are kept in proportion" : "Keep width and height in proportion");
  form.ratioLock.disabled = !!form.spec.disabled;
  form.ratioLock.replaceChildren(icon(on ? "lock" : "lock-open"));
}

// valueIn is what the files hold for a field
function valueIn(form, f) {
  const at = form.spec.at(f);
  if (!at) return undefined;
  if (f.at) return layerAt(form.spec.entry(), form.spec.origin())[f.path[1]];
  return readPath(at, f.path);
}

// fill shows the files' values in the fields, leaving the one in use alone
function fill(form) {
  syncRatio(form);
  for (const entry of form.groups) {
    if (entry.toggle) {
      const on = !!readPath(form.spec.at(entry.group.fields[0]), ["text", "shadow"]);
      if (!entry.toggle.dataset.pending) entry.toggle.checked = on;
      entry.rows.hidden = !(entry.toggle.dataset.pending ? entry.toggle.checked : on);
    }
  }
  for (const ctl of form.controls) {
    if (document.activeElement === ctl.c) continue;
    ctl.c.value = displayValue(ctl.f, valueIn(form, ctl.f));
    ctl.c.disabled = !!form.spec.disabled || (ctl.f.lockable && form.spec.lock === ctl.f.id);
    ctl.c.readOnly = !!(ctl.f.lockable && form.spec.lockLicense);
    ctl.c.removeAttribute("aria-invalid");
    ctl.err.hidden = true;
  }
}

// apply makes the change a field's text asks for, or says why it cannot be
function apply(form, ctl) {
  const { f, c, err } = ctl;
  const parsed = parseField(f, c.value);
  if (!parsed.ok) {
    c.setAttribute("aria-invalid", "true");
    err.textContent = parsed.error;
    err.hidden = false;
    return;
  }
  c.removeAttribute("aria-invalid");
  err.hidden = true;
  const now = valueIn(form, f);
  if (parsed.unset ? now === undefined : now === parsed.value) return;
  // With the proportion kept, the other side of the canvas follows
  const kept = form.spec.ratio && form.spec.ratio.get();
  if (kept && kept.locked && !parsed.unset && (f.id === "width" || f.id === "height")) {
    const sides = ["width", "height"].map((id) => valueIn(form, form.controls.find((c) => c.f.id === id).f));
    // The shape the canvas has now is the one to keep, taken once and held
    if (!(kept.ratio > 0)) {
      if (!(sides[0] > 0 && sides[1] > 0)) return;
      kept.ratio = sides[0] / sides[1];
      form.spec.ratio.set(kept);
    }
    const ops = sizeOps(form.spec.target, keepRatio(f.id, parsed.value, kept.ratio), { width: sides[0], height: sides[1] });
    if (ops.length) commit(ops, "Change the size of the canvas", "");
    return;
  }
  const ops = editOps(form.spec.target, f, parsed, { at: layerAt(form.spec.entry(), form.spec.origin()), part: form.spec.part });
  commit(ops, editLabel(form.spec.target, f), `lever:${form.spec.target.kind}:${form.spec.target.key}:${f.id}`);
}

let itemForm = null;
let itemFormKey = "";
let canvasForm = null;
let canvasFormKey = "";
// whether each layout's width and height are kept in proportion, which they are unless it
// was let go of, and the proportion, which is for this page and not kept with the draft
const ratioLocks = new Map();
const ratioKey = () => `${store.current.peek()}|${(store.layout.peek() || {}).layout}`;
let templateForm = null;
let templateFormKey = "";

function ctxFor(view, src) {
  const vocab = store.vocab.peek() || {};
  const layoutDoc = src.docs[layoutFileOf(view)] || {};
  const template = src.docs["template.json"] || {};
  return { vocab, templateRoles: Object.keys(template.roles || {}), stackIds: (layoutDoc.stack || []).map((e) => e.id || e.part) };
}

function drawLevers() {
  const view = store.layout.value;
  const src = store.source.value;
  const sel = store.selected.value;
  const vocab = store.vocab.value;
  const meta = (store.view.value || {}).meta || {};
  const item = view ? itemOf(view, sel) : null;
  const ready = view && src && vocab && src.docs[layoutFileOf(view)];

  // the selection's levers
  const box = $("ed-levers");
  const key = ready && item ? `${view.layout}|${sel}|${view.editable}` : "";
  if (!key) {
    if (itemFormKey) box.replaceChildren();
    itemForm = null;
    itemFormKey = "";
  } else {
    if (key !== itemFormKey) {
      const ctx = ctxFor(view, src);
      const found = nodeOf(item, view, src);
      itemForm = found
        ? buildForm(box, "lever", fieldGroups(item.kind, ctx), {
            target: { layout: view.layout, kind: item.kind, key: item.key },
            disabled: !view.editable,
            part: found.part,
            at: (f) => (f.on === "part" ? nodeOf(item, view, store.source.peek()).partNode : nodeOf(item, view, store.source.peek()).node),
            entry: () => nodeOf(item, view, store.source.peek()).node,
            origin: () => {
              const f2 = nodeOf(item, view, store.source.peek());
              return f2 && f2.partNode ? f2.partNode.origin : null;
            },
          })
        : null;
      if (!found) box.replaceChildren();
      itemFormKey = key;
    }
    if (itemForm) fill(itemForm);
  }

  // the layout's canvas
  const ckey = ready ? `${view.layout}|${view.editable}` : "";
  if (!ckey) {
    canvasForm = null;
    canvasFormKey = "";
    $("ed-canvas-levers").replaceChildren();
  } else {
    if (ckey !== canvasFormKey) {
      canvasForm = buildForm($("ed-canvas-levers"), "canvas", fieldGroups("canvas", ctxFor(view, src)), {
        target: { layout: view.layout, kind: "canvas", key: "" },
        disabled: !view.editable,
        at: () => (store.source.peek().docs[layoutFileOf(view)] || {}).canvas,
        entry: () => null,
        origin: () => null,
        // The proportion is kept until it is let go of, and it is the canvas's own shape
        // the first time an edit needs it
        ratio: { get: () => ratioLocks.get(ratioKey()) || { locked: true, ratio: null }, set: (v) => ratioLocks.set(ratioKey(), v) },
      });
      canvasFormKey = ckey;
    }
    fill(canvasForm);
  }

  // the template's own fields
  const tkey = src && src.docs["template.json"] ? `${store.current.value}|${meta.licenseLocked}` : "";
  if (!tkey) {
    templateForm = null;
    templateFormKey = "";
    $("ed-template-levers").replaceChildren();
  } else {
    if (tkey !== templateFormKey) {
      templateForm = buildForm($("ed-template-levers"), "template", fieldGroups("template", { vocab: vocab || {} }), {
        target: { layout: "", kind: "template", key: "" },
        lockLicense: !!meta.licenseLocked,
        at: () => store.source.peek().docs["template.json"],
        entry: () => null,
        origin: () => null,
      });
      templateFormKey = tkey;
    }
    fill(templateForm);
  }
  drawCredits();
}

// --- credits ---

function drawCredits() {
  const src = store.source.value;
  const meta = (store.view.value || {}).meta || {};
  const list = $("ed-credits");
  const template = src && src.docs["template.json"];
  if (!template) {
    list.replaceChildren();
    return;
  }
  // A credit being typed in is left as it is
  if (list.contains(document.activeElement) && document.activeElement.tagName === "INPUT") return;
  const credits = template.credits || [];
  const locked = meta.lockedCredits || 0;
  list.replaceChildren(
    ...credits.map((c, i) => {
      const kept = i < locked;
      const input = (key, label, value) => {
        const el = h("input", { type: "text", "aria-label": `Credit ${i + 1} ${label}`, value: value || "", readOnly: kept, autocomplete: "off", spellcheck: "false" });
        if (!kept) {
          let timer = 0;
          const send = () => {
            clearTimeout(timer);
            const now = store.source.peek().docs["template.json"].credits;
            const next = creditEdit(now, i, key, el.value.trim(), locked);
            // The same text again, as when leaving a field that has already been saved, is
            // not a change
            if (next && JSON.stringify(next) !== JSON.stringify(now)) commit([{ op: "set", layout: "", kind: "template", key: "", path: ["credits"], value: next }], `Change credit ${i + 1}`, `credit:${i}:${key}`);
          };
          el.addEventListener("input", () => {
            clearTimeout(timer);
            timer = setTimeout(send, LEVER_DELAY_MS);
          });
          el.addEventListener("change", send);
        }
        return el;
      };
      const remove = kept
        ? null
        : h(
            "button",
            {
              type: "button",
              class: "btn subtle icon-only",
              "aria-label": `Remove credit ${i + 1}`,
              onclick: () => {
                const next = creditRemove(store.source.peek().docs["template.json"].credits, i, locked);
                if (next) commit([{ op: next.length ? "set" : "unset", layout: "", kind: "template", key: "", path: ["credits"], ...(next.length ? { value: next } : {}) }], `Remove credit ${i + 1}`, "");
              },
            },
            h("span", { "aria-hidden": "true" }, "×"),
          );
      return h(
        "div",
        { class: "ed-credit" },
        h("div", { class: "ed-credit-head" }, input("name", "name", c.name), remove),
        input("url", "link", c.url),
        input("note", "note", c.note),
        kept ? h("span", { class: "ed-credit-kept" }, "Kept from the template this was copied from") : null,
      );
    }),
  );
}

function addCredit() {
  const src = store.source.peek();
  const template = src && src.docs["template.json"];
  if (!template) return;
  commit([{ op: "set", layout: "", kind: "template", key: "", path: ["credits"], value: creditAdd(template.credits) }], "Add a credit", "");
}

// --- the JSON editor ---

const json = { open: false, file: "", dirty: false, timer: 0, seq: 0 };

const fileLabel = (name) => (name === "template.json" ? name : name.replace(/^layouts\//, "").replace(/\.json$/, ""));

function setJsonStatus(text) {
  $("ed-json-status").textContent = text;
}

function updateGutter() {
  const ta = $("ed-json-text");
  const lines = ta.value.split("\n").length;
  const gutter = $("ed-json-gutter");
  if (gutter.dataset.lines !== String(lines)) {
    gutter.dataset.lines = String(lines);
    gutter.textContent = Array.from({ length: lines }, (_, i) => i + 1).join("\n");
  }
  gutter.scrollTop = ta.scrollTop;
}

function drawJsonTabs() {
  const src = store.source.value;
  const tabs = $("ed-json-tabs");
  if (!src) {
    tabs.replaceChildren();
    return;
  }
  tabs.replaceChildren(
    ...src.files.map((f) =>
      h(
        "button",
        {
          type: "button",
          class: "chip",
          role: "tab",
          "aria-selected": String(f.name === json.file),
          title: f.name,
          onclick: () => showJsonFile(f.name),
        },
        fileLabel(f.name),
      ),
    ),
  );
}

// showJsonFile puts a file's text in the editor and says what is wrong with it now
async function showJsonFile(name) {
  const src = store.source.peek();
  const file = src && src.files.find((f) => f.name === name);
  if (!file) return;
  json.file = name;
  json.dirty = false;
  const ta = $("ed-json-text");
  ta.value = file.content;
  ta.scrollTop = 0;
  updateGutter();
  drawJsonTabs();
  setJsonStatus("");
  checkJson(file.content, false);
}

// syncJson brings the editor's text up to the files after an edit made elsewhere, unless
// the person has typed something that is not saved
function syncJson() {
  const src = store.source.value;
  if (!json.open || !src) return;
  drawJsonTabs();
  const file = src.files.find((f) => f.name === json.file);
  if (!file) {
    if (src.files.length) showJsonFile(src.files[0].name);
    return;
  }
  const ta = $("ed-json-text");
  if (!json.dirty && ta.value !== file.content) {
    ta.value = file.content;
    updateGutter();
  }
}

function drawJsonProblems(problems) {
  const ta = $("ed-json-text");
  $("ed-json-problems").replaceChildren(
    ...problems.map((p) => {
      const where = p.line ? `Line ${p.line}, column ${p.column}` : p.path;
      const li = h("li", { class: p.severity === "warning" ? "warning" : "error" }, where ? `${where}: ${p.message}` : p.message);
      if (p.line) {
        li.dataset.line = String(p.line);
        li.addEventListener("click", () => {
          const at = ta.value.split("\n").slice(0, p.line - 1).reduce((n, l) => n + l.length + 1, 0) + (p.column - 1);
          ta.focus();
          ta.setSelectionRange(at, at);
        });
      }
      return li;
    }),
  );
}

// checkJson asks the app what the text would do, shows the problems, and when the text
// can be saved, saves it
async function checkJson(text, save) {
  const id = store.current.peek();
  const file = json.file;
  if (!id || !file) return;
  const seq = ++json.seq;
  let res;
  try {
    res = await api.checkDraftFile(id, file, text);
  } catch (err) {
    setJsonStatus(err.message);
    return;
  }
  if (seq !== json.seq) return;
  drawJsonProblems(res.problems);
  const errors = res.problems.some((p) => p.severity !== "warning");
  if (!save) {
    setJsonStatus(errors ? "Has problems" : "");
    return;
  }
  if (!res.writable) {
    setJsonStatus("Not saved, fix the problems first");
    return;
  }
  setJsonStatus("Saving…");
  await commit([{ op: "replace", file, content: text }], `Edit ${fileLabel(file)}`, `json:${file}`);
  if (seq === json.seq && $("ed-json-text").value === text) {
    json.dirty = false;
    setJsonStatus(errors ? "Saved, with problems" : "Saved");
  }
}

function onJsonInput() {
  const ta = $("ed-json-text");
  json.dirty = true;
  updateGutter();
  setJsonStatus("Checking…");
  clearTimeout(json.timer);
  const text = ta.value;
  json.timer = setTimeout(() => checkJson(text, true), 450);
}

function toggleJson(open) {
  json.open = open;
  $("ed-json-pane").hidden = !open;
  $("ed-json").setAttribute("aria-expanded", String(open));
  if (!open) return;
  const src = store.source.peek();
  if (src && src.files.length) showJsonFile(json.file && src.files.some((f) => f.name === json.file) ? json.file : src.files[0].name);
  else loadSource().then(() => syncJson());
}

// enter reads what the mode shows each time it is opened, so a draft made or
// changed elsewhere is never stale, and opens the newest draft when none is open
async function enter() {
  await Promise.all([loadDrafts(), loadPresets(), loadVocab()]);
  const list = store.drafts.peek() || [];
  const current = store.current.peek();
  if (current && list.some((d) => d.id === current)) return;
  if (list.length) await open(list[0].id);
}

export const editor = { newTemplate, copyTemplate };

// initView wires the camera controls and the keys that pan and zoom the card's view
function initView() {
  const stage = $("ed-stage");
  viewport = createViewport({ stage, canvas: $("ed-canvas"), img: $("ed-img"), label: $("ed-zoom-label"), isPan: panning });
  $("ed-zoom-in").addEventListener("click", () => viewport.step(1));
  $("ed-zoom-out").addEventListener("click", () => viewport.step(-1));
  $("ed-zoom-fit").addEventListener("click", () => viewport.reset());
  $("ed-zoom-label").addEventListener("click", () => viewport.cycle());
  $("ed-tool-select").addEventListener("click", () => {
    tool.pan.value = false;
  });
  $("ed-tool-pan").addEventListener("click", () => {
    tool.pan.value = true;
  });
  effect(() => {
    const on = tool.pan.value;
    $("ed-tool-select").setAttribute("aria-pressed", String(!on));
    $("ed-tool-pan").setAttribute("aria-pressed", String(on));
  });
  effect(() => {
    stage.classList.toggle("pan-mode", tool.pan.value || tool.space.value);
    // Whatever was being hovered is not now
    if (tool.pan.value || tool.space.value) store.hover.value = "";
  });

  // Holding Space pans for as long as it is held, while the pointer is over the view, so
  // Space still presses a button anywhere else. It lets go on its own when the window does
  stage.addEventListener("pointerenter", () => {
    tool.over = true;
  });
  stage.addEventListener("pointerleave", () => {
    tool.over = false;
  });
  document.addEventListener("keydown", (e) => {
    if (e.key !== " " || e.repeat || !tool.over || app.mode.peek() !== "editor" || isTyping() || document.querySelector("dialog[open]")) return;
    e.preventDefault();
    tool.space.value = true;
  });
  document.addEventListener("keyup", (e) => {
    if (e.key !== " " || !tool.space.peek()) return;
    e.preventDefault();
    tool.space.value = false;
  });
  window.addEventListener("blur", () => {
    tool.space.value = false;
  });
}

let layoutTimer = 0;

// keepLayout saves how a part of the editor is laid out with the other layout settings
function keepLayout(key, list) {
  const s = app.settings.peek();
  app.settings.value = { ...s, splits: { ...(s.splits || {}), [key]: list } };
  clearTimeout(layoutTimer);
  layoutTimer = setTimeout(() => api.saveSettings(app.settings.peek()).catch(() => {}), 400);
}

// initFold makes a section of the left column open and collapse with its arrow, which is
// kept under key with the rest of the layout. It starts collapsed
function initFold(sectionId, toggleId, bodyId, key) {
  const box = $(sectionId);
  const toggle = $(toggleId);
  const body = $(bodyId);
  const saved = (app.settings.peek().splits || {})[key];
  const show = (open) => {
    box.classList.toggle("collapsed", !open);
    toggle.setAttribute("aria-expanded", String(open));
    body.hidden = !open;
  };
  show(Array.isArray(saved) && saved[0] === 0);
  toggle.addEventListener("click", () => {
    const open = toggle.getAttribute("aria-expanded") !== "true";
    show(open);
    keepLayout(key, [open ? 0 : 1]);
  });
}

export function initEditor() {
  $("ed-empty-new").addEventListener("click", newTemplate);
  $("ed-new").addEventListener("click", newTemplate);
  $("ed-empty-copy").addEventListener("click", () => {
    location.hash = "templates";
  });
  $("ed-rename").addEventListener("click", rename);
  $("ed-delete").addEventListener("click", remove);
  const sections = initSections($("ed-sections"), (app.settings.peek().splits || {}).editorSections, (list) => keepLayout("editorSections", list));
  initFold("ed-canvas-sec", "ed-canvas-toggle", "ed-canvas-body", "editorCanvas");
  // The test card, its properties and the search are sections that collapse and can be
  // given the height their reader likes, as the right column's are
  initSections($("ed-left-sections"), (app.settings.peek().splits || {}).editorLeft, (list) => keepLayout("editorLeft", list), { ids: LEFT_SECTIONS, defaults: LEFT_DEFAULTS });
  // The test card's fields are the same form Single edits a card with
  cardEditor = createCardEditor({ idPrefix: "ed", base: store.card, edits: store.edits, reference: store.reference, onEnter: keepCardEdits });
  cardEditor.mount($("ed-card-form"));
  $("ed-fine-revert").addEventListener("click", () => {
    store.edits.value = store.reference.peek();
  });
  effect(keepCardEdits);
  effect(() => {
    $("ed-fine-revert").hidden = !cardEditor.dirty.value;
  });
  // A template that does not load has its problems shown, however the column was left
  let seen = "";
  effect(() => {
    const view = store.view.value;
    const key = view ? `${view.draft.id}:${view.valid}` : "";
    if (key === seen) return;
    seen = key;
    if (view && !view.valid) sections.open("problems");
  });
  $("ed-draft-select").addEventListener("change", (e) => open(e.target.value));

  const overlay = $("ed-overlay");
  overlay.addEventListener("pointerdown", onPointerDown);
  overlay.addEventListener("pointermove", onPointerMove);
  overlay.addEventListener("pointerup", onPointerUp);
  overlay.addEventListener("pointercancel", cancelGesture);
  // A press on the stage around the card lets go of the selection
  $("ed-stage").addEventListener("pointerdown", (e) => {
    if (e.button === 0 && !panning() && !e.target.closest("#ed-canvas")) store.selected.value = "";
  });
  initView();
  overlay.addEventListener("pointerleave", () => {
    if (!gesture) store.hover.value = "";
  });
  $("ed-layers").addEventListener("keydown", onLayersKey);
  $("ed-undo").addEventListener("click", undo);
  $("ed-redo").addEventListener("click", redo);
  initAssets({ store, commit, recordApplied, select, loadAssets, nameOf, itemOf, takenKeys, flushNudge });
  document.addEventListener("keydown", onKey);
  $("ed-credit-add").addEventListener("click", addCredit);
  $("ed-json").addEventListener("click", () => toggleJson(!json.open));
  $("ed-json-close").addEventListener("click", () => toggleJson(false));
  $("ed-json-text").addEventListener("input", onJsonInput);
  $("ed-json-text").addEventListener("scroll", () => {
    $("ed-json-gutter").scrollTop = $("ed-json-text").scrollTop;
  });

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
  effect(drawOverlay);
  effect(drawLayers);
  effect(() => markSelected(store.selected.value));
  effect(drawSelection);
  effect(drawLevers);
  effect(syncJson);
  effect(() => {
    $("ed-undo").disabled = !store.canUndo.value;
    $("ed-redo").disabled = !store.canRedo.value;
  });
  effect(() => {
    if (app.mode.value === "editor") enter();
  });
  document.addEventListener("mimic:resolution-changed", () => {
    if (app.mode.peek() === "editor" && store.card.peek()) render();
  });
}

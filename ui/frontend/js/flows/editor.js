import { api } from "../api.js";
import { $, h, isTyping } from "../dom.js";
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
import { creditAdd, creditEdit, creditRemove, displayValue, editLabel, editOps, fieldGroups, layerAt, parseField, readPath, shadowOps } from "../editorlevers.js";

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
};

const history = createHistory();
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
    $("ed-canvas").hidden = false;
    $("ed-placeholder").hidden = true;
    loadLayout();
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
  });
  clearPreview();
  store.source.value = null;
  loadSource();
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
    const view = await api.describeDraft(id, card, store.face.peek());
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

function select(ref) {
  flushNudge();
  store.selected.value = ref || "";
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

// commit makes edits to the draft, one batch after another, and remembers how to take
// them back. merge marks an edit that joins the one before it when they are close
function commit(ops, label, merge) {
  const id = store.current.peek();
  const run = async () => {
    try {
      const res = await api.applyDraft(id, ops);
      history.record({ ops, inverse: res.inverse, label, merge });
      syncHistory();
      await adopt(id, res, ops);
      await Promise.all([loadLayout(), loadSource()]);
      scheduleRender();
    } catch (err) {
      toast(err.message, "err", 6000);
      await loadLayout();
    } finally {
      store.live.value = null;
    }
  };
  chain = chain.then(run, run);
  return chain;
}

// replay makes the edits of an undo or a redo. One the app refuses leaves the history
// where it was
function replay(ops, undoing) {
  const id = store.current.peek();
  const run = async () => {
    try {
      const res = await api.applyDraft(id, ops);
      await adopt(id, res, ops);
      await Promise.all([loadLayout(), loadSource()]);
      scheduleRender();
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
  replay(entry.inverse, true);
}

function redo() {
  flushNudge();
  const entry = history.redo();
  if (!entry) return;
  syncHistory();
  app.status.value = `Redid: ${entry.label}`;
  replay(entry.ops, false);
}

// --- nudging ---

let nudge = null;

// nudgeSelected moves the selection a little, showing it at once and saving the whole
// run of keys as one edit once they stop
function nudgeSelected(dx, dy) {
  const item = selectedItem();
  const view = store.layout.peek();
  if (!item || !view || !view.editable) return;
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
  if (e.button !== 0 || !view) return;
  flushNudge();
  const overlay = $("ed-overlay");
  const handle = e.target.closest && e.target.closest(".ed-handle");
  const current = selectedItem();
  if (handle && current && view.editable) {
    gesture = { kind: "resize", handle: handle.dataset.handle, item: current, start: { x: e.clientX, y: e.clientY }, rect0: current.delivered, moved: false };
  } else {
    const p = nativePoint(e);
    const hit = hitTest(view.items, p);
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
    const hit = e.buttons ? null : hitTest(view.items, nativePoint(e));
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
    const snapped = snapMove(g.rect0, dx, dy, snapTargets(view.items, view.delivered, refOf(g.item), nameOf), SNAP_PX * per);
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

function reorder(by) {
  const view = store.layout.peek();
  const item = selectedItem();
  if (!view || !item || item.kind !== "layer" || !view.editable) return;
  const layers = view.items.filter((it) => it.kind === "layer");
  const at = layers.findIndex((it) => refOf(it) === refOf(item));
  const to = at + by;
  if (to < 0 || to >= layers.length) return;
  commit([{ op: "reorder", layout: view.layout, kind: "layer", key: item.key, to }], `${by > 0 ? "Raise" : "Lower"} ${nameOf(item)}`, "");
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
      if (view.editable && item.kind !== "layer" && !item.ink) {
        const per = overlayBox().width / canvas.width;
        handles = visibleHandles(rect.width * per, rect.height * per).map((name) => h("div", { class: "ed-handle", dataset: { handle: name } }));
      }
      out.push(box(rect, `sel${item.drawn ? "" : " idle"}`, ...handles));
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
  const list = $("ed-layers");
  // A list with nothing in it is hidden, since a listbox has to hold options
  list.hidden = !view;
  $("ed-layers-none").hidden = !!view;
  if (!view) {
    list.replaceChildren();
    return;
  }
  list.replaceChildren(
    ...[...view.items].reverse().map((it) =>
      h(
        "li",
        {
          class: `ed-layer${it.drawn ? "" : " undrawn"}`,
          role: "option",
          "aria-selected": "false",
          tabIndex: -1,
          title: it.why || "",
          dataset: { ref: refOf(it) },
          onclick: () => select(refOf(it)),
        },
        h("span", { class: "ed-layer-name" }, nameOf(it)),
        h("span", { class: "ed-layer-kind" }, KIND_LABEL[it.kind] || it.kind),
      ),
    ),
  );
  markSelected(store.selected.peek());
}

// markSelected shows which row is selected without rebuilding the list, so a row
// keeps the keyboard focus it has
function markSelected(sel) {
  const rows = [...$("ed-layers").querySelectorAll(".ed-layer")];
  rows.forEach((row, i) => {
    const on = row.dataset.ref === sel;
    row.setAttribute("aria-selected", String(on));
    row.tabIndex = on || (!sel && i === 0) ? 0 : -1;
  });
}

function onLayersKey(e) {
  // An arrow with a modifier is a nudge, which the page handles wherever the focus is
  if (e.shiftKey || e.altKey || e.metaKey || e.ctrlKey) return;
  const rows = [...$("ed-layers").querySelectorAll(".ed-layer")];
  const at = rows.indexOf(document.activeElement);
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    e.stopPropagation();
    const next = rows[Math.min(rows.length - 1, Math.max(0, at + (e.key === "ArrowDown" ? 1 : -1)))];
    if (next) {
      select(next.dataset.ref);
      next.focus();
    }
  } else if ((e.key === "Enter" || e.key === " ") && at >= 0) {
    e.preventDefault();
    select(rows[at].dataset.ref);
  }
}

function drawSelection() {
  const view = store.layout.value;
  const item = itemOf(view, store.selected.value);
  const live = store.live.value;
  $("ed-selection-none").hidden = !!item;
  $("ed-selection").hidden = !item;
  $("ed-edit-note").hidden = !(view && !view.editable);
  $("ed-edit-note").textContent = view && !view.editable ? view.reason : "";
  if (!item) return;
  const rect = live ? live.rect : item.delivered;
  $("ed-selection-name").textContent = nameOf(item);
  $("ed-selection-kind").textContent = item.drawn ? KIND_LABEL[item.kind] : `${KIND_LABEL[item.kind]}, not drawn: ${item.why}`;
  const blank = rect.width === 0 && rect.height === 0;
  $("ed-sel-x").textContent = blank ? "" : String(rect.x);
  $("ed-sel-y").textContent = blank ? "" : String(rect.y);
  $("ed-sel-w").textContent = blank ? "" : String(rect.width);
  $("ed-sel-h").textContent = blank ? "" : String(rect.height);
  const layers = view.items.filter((it) => it.kind === "layer");
  const at = layers.findIndex((it) => refOf(it) === refOf(item));
  const reorderable = item.kind === "layer" && view.editable;
  $("ed-reorder").hidden = !reorderable;
  $("ed-raise").disabled = !reorderable || at >= layers.length - 1;
  $("ed-lower").disabled = !reorderable || at <= 0;
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
      rows.append(field(f.label, id, c, err));
    }
    box.append(rows);
    form.groups.push(entry);
    return box;
  });
  container.replaceChildren(...nodes);
  return form;
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
  const ops = editOps(form.spec.target, f, parsed, { at: layerAt(form.spec.entry(), form.spec.origin()), part: form.spec.part });
  commit(ops, editLabel(form.spec.target, f), `lever:${form.spec.target.kind}:${form.spec.target.key}:${f.id}`);
}

let itemForm = null;
let itemFormKey = "";
let canvasForm = null;
let canvasFormKey = "";
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

export function initEditor() {
  $("ed-empty-new").addEventListener("click", newTemplate);
  $("ed-new").addEventListener("click", newTemplate);
  $("ed-empty-copy").addEventListener("click", () => {
    location.hash = "templates";
  });
  $("ed-rename").addEventListener("click", rename);
  $("ed-delete").addEventListener("click", remove);
  $("ed-draft-select").addEventListener("change", (e) => open(e.target.value));

  const overlay = $("ed-overlay");
  overlay.addEventListener("pointerdown", onPointerDown);
  overlay.addEventListener("pointermove", onPointerMove);
  overlay.addEventListener("pointerup", onPointerUp);
  overlay.addEventListener("pointercancel", cancelGesture);
  // A press on the stage around the card lets go of the selection
  $("ed-stage").addEventListener("pointerdown", (e) => {
    if (e.button === 0 && !e.target.closest("#ed-canvas")) store.selected.value = "";
  });
  overlay.addEventListener("pointerleave", () => {
    if (!gesture) store.hover.value = "";
  });
  $("ed-layers").addEventListener("keydown", onLayersKey);
  $("ed-undo").addEventListener("click", undo);
  $("ed-redo").addEventListener("click", redo);
  $("ed-raise").addEventListener("click", () => reorder(1));
  $("ed-lower").addEventListener("click", () => reorder(-1));
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

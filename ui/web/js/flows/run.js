import { api } from "../api.js";
import { $, h } from "../dom.js";
import { app, signal, effect, batch } from "../state.js";
import { keyedRows } from "../components/keyed.js";
import { syncChips } from "../components/chips.js";
import { toast } from "../components/toast.js";

// The Run Console: watches a batch while it renders and stays readable after.
// It only reports, and the actions it offers start new work from what it found.
// The server keeps the latest run, so a reloaded page picks it back up and the
// event stream replays the run from its start

const GLYPH = {
  queued: { text: "○", cls: "faint" },
  fetching: { text: "●", cls: "active" },
  rendering: { text: "●", cls: "active" },
  writing: { text: "●", cls: "active" },
  done: { text: "✓", cls: "ok" },
  failed: { text: "✗", cls: "err" },
  skipped: { text: "–", cls: "faint" },
};

const STAGE = { art: "art download", render: "render", write: "writing the file" };

const inFlight = (s) => s === "fetching" || s === "rendering" || s === "writing";

const store = {
  run: signal(null), // the run's snapshot without its cards
  cards: [], // [{ id, index, state: signal(runCard) }]
  version: signal(0),
  filter: signal("all"),
  query: signal(""),
  selected: signal(null), // a card index the user picked
  latestDone: signal(null), // the most recent card to finish, followed while running
  expanded: new Set(),
  now: signal(Date.now()),
  // stopping is set from a Stop press until the run settles, since a card mid-render finishes first
  stopping: signal(false),
};

let rows = null;
let stream = null;
let ticker = 0;
let logLines = [];

const bump = () => (store.version.value = store.version.peek() + 1);
const running = () => {
  const r = store.run.peek();
  return !!r && !r.finished;
};

function duration(ms) {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${String(s % 60).padStart(2, "0")}s`;
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, "0")}m`;
}

function counts() {
  const c = { total: store.cards.length, queued: 0, active: 0, done: 0, failed: 0, skipped: 0, ms: 0 };
  for (const card of store.cards) {
    const s = card.state.peek();
    if (inFlight(s.status)) c.active++;
    else if (c[s.status] !== undefined) c[s.status]++;
    if (s.status === "done") c.ms += s.ms || 0;
  }
  c.settled = c.done + c.failed + c.skipped;
  return c;
}

// --- attaching to a run ---

export async function attach(view) {
  if (stream) stream.close();
  const { cards, ...meta } = view;
  store.cards = cards.map((c) => ({ id: String(c.index), index: c.index, state: signal(c) }));
  store.expanded.clear();
  logLines = [];
  $("run-log").textContent = "";
  batch(() => {
    store.run.value = meta;
    store.stopping.value = false;
    store.filter.value = "all";
    store.query.value = "";
    store.selected.value = null;
    store.latestDone.value = null;
  });
  $("run-filter").value = "";
  rows.reset(store.cards);
  bump();
  startTicker();

  const job = api.runEvents(view.id, (step, frac, e) => {
    if (e.card) {
      const card = store.cards[e.card.index];
      if (card) {
        card.state.value = e.card;
        if (e.card.status === "done") store.latestDone.value = e.card.index;
        bump();
      }
    }
    if (e.log) appendLog(e.log);
  });
  stream = job;
  try {
    await job;
  } catch {
    // A dropped stream leaves the last state on screen, and a reload recovers
  }
  if (stream !== job) return;
  stream = null;
  const latest = await api.latestRun().catch(() => null);
  if (latest && latest.id === view.id) {
    const { cards: finalCards, ...finalMeta } = latest;
    for (const c of finalCards) {
      const card = store.cards[c.index];
      if (card) card.state.value = c;
    }
    store.run.value = finalMeta;
    bump();
  }
  stopTicker();
}

function startTicker() {
  stopTicker();
  ticker = setInterval(() => (store.now.value = Date.now()), 1000);
}

function stopTicker() {
  clearInterval(ticker);
  store.now.value = Date.now();
}

function appendLog(line) {
  logLines.push(line);
  const log = $("run-log");
  const stick = log.scrollTop + log.clientHeight >= log.scrollHeight - 4;
  log.append(`${line}\n`);
  if (stick) log.scrollTop = log.scrollHeight;
}

// --- rows ---

function buildRow(card) {
  const glyph = h("span", { class: "glyph" });
  const name = h("span", { class: "name" });
  const detail = h("span", { class: "detail tabular" });
  const bar = h("div", { class: "progress" }, h("div", { class: "bar" }));
  const err = h("div", { class: "row-error" });
  const li = h(
    "li",
    {
      tabindex: "-1",
      onClick: () => {
        const s = card.state.peek();
        if (s.status === "failed") {
          if (store.expanded.has(card.index)) store.expanded.delete(card.index);
          else store.expanded.add(card.index);
          card.state.value = { ...s };
        }
        store.selected.value = card.index;
      },
    },
    h("div", { class: "run-line" }, glyph, name, bar, detail),
    err,
  );
  const disposeState = effect(() => {
    const s = card.state.value;
    const g = GLYPH[s.status] || GLYPH.queued;
    glyph.textContent = g.text;
    glyph.className = `glyph ${g.cls}`;
    name.textContent = s.name;
    li.dataset.status = s.status;
    bar.hidden = s.status !== "rendering";
    bar.firstElementChild.style.width = `${Math.round((s.frac || 0) * 100)}%`;
    if (s.status === "done") detail.textContent = `${((s.ms || 0) / 1000).toFixed(1)}s`;
    else if (s.status === "failed") detail.textContent = STAGE[s.stage] ? `${STAGE[s.stage]} failed` : "failed";
    else if (s.status === "rendering") detail.textContent = `${Math.round((s.frac || 0) * 100)}%`;
    else if (s.status === "fetching") detail.textContent = "fetching art";
    else if (s.status === "writing") detail.textContent = "writing";
    else detail.textContent = s.status;
    detail.className = `detail tabular ${s.status === "failed" ? "err-text" : s.status === "done" ? "dim" : "faint"}`;
    const open = s.status === "failed" && store.expanded.has(card.index);
    err.hidden = !open;
    if (open) {
      err.replaceChildren(
        h("div", {}, h("span", { class: "dim" }, "Stage "), STAGE[s.stage] || s.stage || "unknown"),
        h("div", { class: "mono" }, s.error || "No error text"),
      );
    }
  });
  const disposeSel = effect(() => {
    li.classList.toggle("selected", store.selected.value === card.index);
  });
  return {
    el: li,
    dispose() {
      disposeState();
      disposeSel();
    },
  };
}

function matches(card) {
  const s = card.state.peek();
  const f = store.filter.peek();
  if (f === "active" && !inFlight(s.status) && s.status !== "queued") return false;
  if (f !== "all" && f !== "active" && s.status !== f) return false;
  const q = store.query.peek().trim().toLowerCase();
  return !q || s.name.toLowerCase().includes(q);
}

// --- actions ---

async function stop() {
  const r = store.run.peek();
  if (!r || !running()) return;
  store.stopping.value = true;
  await api.stopRun(r.id).catch(() => {});
  appendLog("stop requested, finishing the cards already rendering");
}

async function retryFailed() {
  const r = store.run.peek();
  if (!r) return;
  try {
    const view = await api.retryRun(r.id);
    attach(view);
  } catch (err) {
    toast(err.message, "err", 6000);
  }
}

// copyLog uses the clipboard API, falling back to a selection copy where the
// page is not allowed the clipboard
function copyLog() {
  const text = logLines.join("\n");
  const fallback = () => {
    const area = h("textarea", { class: "sr-only", readonly: true });
    area.value = text;
    document.body.append(area);
    area.select();
    const ok = document.execCommand("copy");
    area.remove();
    toast(ok ? "Log copied" : "Could not copy the log", ok ? "ok" : "err");
  };
  if (!navigator.clipboard) return fallback();
  navigator.clipboard.writeText(text).then(() => toast("Log copied", "ok"), fallback);
}

function saveLog() {
  const r = store.run.peek();
  const blob = new Blob([`${logLines.join("\n")}\n`], { type: "text/plain" });
  const a = h("a", { href: URL.createObjectURL(blob), download: `mimic-${r ? r.id : "run"}.log` });
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}

// --- view ---

function initView() {
  rows = keyedRows($("run-rows"), buildRow);

  $("run-stop").addEventListener("click", stop);
  $("run-open").addEventListener("click", () => {
    const r = store.run.peek();
    if (r) api.openRunFolder(r.id).catch(() => toast("Could not open the folder", "err"));
  });
  $("run-retry").addEventListener("click", retryFailed);
  // The buttons sit in the log's summary, so a press must not also toggle it
  const inSummary = (fn) => (e) => {
    e.preventDefault();
    fn();
  };
  $("run-log-copy").addEventListener("click", inSummary(copyLog));
  $("run-log-save").addEventListener("click", inSummary(saveLog));
  $("run-filter").addEventListener("input", (e) => (store.query.value = e.target.value));
  $("run-empty-list").addEventListener("click", () => (location.hash = "list"));

  // Header: progress while running, a summary once finished
  effect(() => {
    store.version.value;
    const r = store.run.value;
    const now = store.now.value;
    $("run-empty").hidden = !!r;
    $("run-body").hidden = !r;
    if (!r) return;

    const c = counts();
    const live = !r.finished;
    const end = r.finished ? new Date(r.finished).getTime() : now;
    const elapsed = end - new Date(r.started).getTime();

    $("run-title").textContent = r.label ? `Run · ${r.label}` : "Run";
    const pill = $("run-state");
    const stopping = live && store.stopping.value;
    pill.textContent = stopping ? "stopping" : live ? "running" : r.stopped ? "stopped" : c.failed ? "finished with failures" : "finished";
    pill.className = `pill ${live ? "active" : r.stopped || c.failed ? "warn" : "ok"}`;

    $("run-bar").hidden = !live;
    $("run-bar").firstElementChild.style.width = `${(c.settled / Math.max(c.total, 1)) * 100}%`;
    $("run-count").textContent = `${c.settled} / ${c.total}`;
    $("run-count").hidden = !live;
    const failed = $("run-failed");
    failed.textContent = `${c.failed} failed`;
    failed.hidden = c.failed === 0;
    $("run-elapsed").textContent = duration(elapsed);

    // The estimate spreads the mean time of finished cards over the workers
    let eta = "";
    if (live && c.done > 0) {
      const remaining = c.total - c.settled;
      eta = `about ${duration(((c.ms / c.done) * remaining) / Math.max(r.concurrency, 1))} left`;
    }
    $("run-eta").textContent = eta;
    $("run-eta").hidden = !eta;

    const summary = $("run-summary");
    summary.hidden = live;
    if (!live) {
      const parts = [`${c.done} done`];
      if (c.failed) parts.push(`${c.failed} failed`);
      if (c.skipped) parts.push(`${c.skipped} skipped`);
      summary.textContent = `${parts.join(" · ")} in ${duration(elapsed)}`;
    }
    $("run-meta").textContent = [r.id, r.template, `${r.dpi} dpi`, r.report].filter(Boolean).join(" · ");
    $("run-out").textContent = r.outDir;
    $("run-out").title = r.outDir;

    $("run-stop").hidden = !live;
    $("run-stop").disabled = stopping;
    $("run-stop").dataset.tip = stopping ? "Waiting for the cards already rendering to finish" : "";
    $("run-retry").hidden = live || c.failed === 0;
    $("run-retry-label").textContent = `Retry ${c.failed} failed`;
    $("run-review").hidden = live;

    const filter = store.filter.value;
    store.query.value;
    const chips = [{ label: "All", value: "all", count: c.total }];
    if (live) chips.push({ label: "In progress", value: "active", count: c.active + c.queued });
    chips.push({ label: "Done", value: "done", count: c.done }, { label: "Failed", value: "failed", count: c.failed });
    if (c.skipped) chips.push({ label: "Skipped", value: "skipped", count: c.skipped });
    syncChips($("run-chips"), chips, filter, (v) => (store.filter.value = v));

    rows.show(store.cards, matches);

    // The rail and status bar carry a running batch into every other mode
    $("rail-run-dot").hidden = !live;
    if (live) {
      app.status.value = `Rendering ${c.settled} of ${c.total} cards`;
      app.progress.value = c.settled / Math.max(c.total, 1);
    }
  });

  // Clicking the failure count shows the failures
  $("run-failed").addEventListener("click", () => (store.filter.value = "failed"));

  // The side panel shows the picked card, or follows the latest one to finish
  effect(() => {
    store.version.value;
    const r = store.run.value;
    const picked = store.selected.value;
    const index = picked ?? store.latestDone.value;
    const img = $("run-image");
    const caption = $("run-caption");
    const card = index == null ? null : store.cards[index];
    const s = card ? card.state.peek() : null;
    if (!r || !s || s.status !== "done") {
      img.hidden = true;
      $("run-placeholder").hidden = false;
      caption.textContent = s && s.status === "failed" ? `${s.name} failed at ${STAGE[s.stage] || s.stage}` : s ? `${s.name} is ${s.status}` : "";
      return;
    }
    const src = api.runImageURL(r.id, index);
    if (img.dataset.src !== src) {
      img.dataset.src = src;
      img.src = src;
    }
    img.alt = s.name;
    img.hidden = false;
    $("run-placeholder").hidden = true;
    caption.textContent = s.file;
  });

  // A finished run hands the status bar back
  let wasLive = false;
  effect(() => {
    const r = store.run.value;
    const live = !!r && !r.finished;
    if (wasLive && !live) {
      const c = counts();
      app.status.value = `Run finished: ${c.done} done${c.failed ? `, ${c.failed} failed` : ""}${c.skipped ? `, ${c.skipped} skipped` : ""}.`;
      app.progress.value = null;
      if (app.mode.peek() !== "run") toast(app.status.peek(), c.failed ? "err" : "ok", 5000);
    }
    wasLive = live;
  });
}

export async function initRun() {
  initView();
  document.addEventListener("mimic:run-started", (e) => {
    attach(e.detail);
    location.hash = "run";
  });
  const latest = await api.latestRun().catch(() => null);
  if (latest) attach(latest);
}

export const runConsole = { stop };

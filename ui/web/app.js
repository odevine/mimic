"use strict";

// The browser is the single driver of UI state: it holds the selected card's
// original JSON for reset and the current form values, and it supersedes stale
// work itself with an AbortController on each new search or render. The server
// handles each request independently.

const $ = (id) => document.getElementById(id);

// Editable fields, paired with the card.Data JSON keys the search endpoint
// returns. The form direction reads these; the render body sends them under the
// edits object with the same short names
const FIELDS = [
  ["name", "Name"],
  ["manaCost", "ManaCost"],
  ["colors", null], // joined from the Colors array below
  ["typeLine", "TypeLine"],
  ["oracle", "OracleText"],
  ["flavor", "FlavorText"],
  ["power", "Power"],
  ["toughness", "Toughness"],
  ["loyalty", "Loyalty"],
  ["artist", "Artist"],
  ["setCode", "SetCode"],
  ["collector", "CollectorNumber"],
  ["rarity", "Rarity"],
  ["released", "ReleasedAt"],
  ["language", "Language"],
];

const state = {
  results: [],
  selected: null, // the selected card.Data, kept for reset
  searchAbort: null,
  renderSource: null, // current render EventSource, closed when superseded
};

// setStatus writes the footer status line
function setStatus(text) {
  $("status").textContent = text;
}

// fillForm populates the editor from a card.Data object
function fillForm(card) {
  for (const [field, key] of FIELDS) {
    const el = $("f-" + field);
    if (field === "colors") {
      el.value = (card.Colors || []).join("");
    } else {
      el.value = card[key] || "";
    }
  }
}

// readEdits reads the editor into the edits object the render body carries
function readEdits() {
  const edits = {};
  for (const [field] of FIELDS) {
    edits[field] = $("f-" + field).value;
  }
  return edits;
}

// showResults renders the results list and reports the count
function showResults(query, results) {
  state.results = results;
  const list = $("results-list");
  list.innerHTML = "";
  results.forEach((res, i) => {
    const li = document.createElement("li");
    li.textContent = res.text;
    li.tabIndex = 0;
    li.addEventListener("click", () => selectResult(i, li));
    li.addEventListener("keydown", (e) => {
      if (e.key === "Enter") selectResult(i, li);
    });
    list.appendChild(li);
  });
  if (results.length === 0) setStatus(`No cards match ${JSON.stringify(query)}`);
  else if (results.length === 1) setStatus("1 result");
  else setStatus(`${results.length} results`);
}

// runSearch queries the server, superseding any in-flight search
async function runSearch(query) {
  query = query.trim();
  if (query === "") return;
  if (state.searchAbort) state.searchAbort.abort();
  const ac = new AbortController();
  state.searchAbort = ac;

  $("search-btn").disabled = true;
  setStatus(`Searching for ${JSON.stringify(query)}…`);
  try {
    const resp = await fetch(`/api/search?q=${encodeURIComponent(query)}`, { signal: ac.signal });
    if (!resp.ok) throw new Error(await resp.text());
    const results = await resp.json();
    showResults(query, results);
    loadRecents();
  } catch (err) {
    if (err.name === "AbortError") return;
    setStatus("Search failed: " + err.message);
  } finally {
    if (state.searchAbort === ac) {
      state.searchAbort = null;
      $("search-btn").disabled = false;
    }
  }
}

// selectResult loads the chosen card into the editor and renders it. The card's
// full JSON is kept so a reset restores it and edits re-render from it
function selectResult(i, li) {
  const res = state.results[i];
  if (!res) return;
  document.querySelectorAll("#results-list li.selected").forEach((el) => el.classList.remove("selected"));
  if (li) li.classList.add("selected");

  state.selected = res.card;
  fillForm(res.card);
  $("render-btn").disabled = false;
  $("reset-btn").disabled = false;
  render();
}

// render posts the current editor values over the selected card and drives the
// progress bar and preview from the job's event stream
function render() {
  if (!state.selected) return;
  const body = { base: state.selected, edits: readEdits() };

  $("save-link").hidden = true;
  beginProgress("Rendering " + ($("f-name").value || "card") + "…");

  fetch("/api/render", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
    .then((resp) => {
      if (!resp.ok) return resp.text().then((t) => { throw new Error(t); });
      return resp.json();
    })
    .then(({ jobId }) => watchRender(jobId))
    .catch((err) => {
      endProgress();
      setStatus("Render failed: " + err.message);
    });
}

// watchRender opens the render job's SSE stream, superseding any earlier one,
// and swaps in the finished image on the done event
function watchRender(jobId) {
  if (state.renderSource) state.renderSource.close();
  const es = new EventSource(`/api/render/${jobId}/events`);
  state.renderSource = es;

  es.onmessage = (ev) => {
    const e = JSON.parse(ev.data);
    if (e.done) {
      es.close();
      if (state.renderSource === es) state.renderSource = null;
      endProgress();
      if (e.error) {
        setStatus("Render failed: " + e.error);
        return;
      }
      const name = $("f-name").value || "card";
      $("preview-img").src = `/api/render/${jobId}/image?ts=${Date.now()}`;
      const save = $("save-link");
      save.href = `/api/render/${jobId}/image?download=1`;
      save.hidden = false;
      setStatus(e.artMissing ? `Rendered ${name} (art unavailable)` : `Rendered ${name}`);
      return;
    }
    stepProgress(e.step, e.frac);
  };
  es.onerror = () => {
    es.close();
    if (state.renderSource === es) state.renderSource = null;
    endProgress();
  };
}

// resetEdits restores the editor to the selected card and re-renders
function resetEdits() {
  if (!state.selected) return;
  fillForm(state.selected);
  render();
}

function beginProgress(text) {
  const p = $("render-progress");
  p.value = 0;
  p.hidden = false;
  setStatus(text);
}
function stepProgress(step, frac) {
  $("render-progress").value = frac;
  if (step) setStatus(step + "…");
}
function endProgress() {
  $("render-progress").hidden = true;
}

// loadRecents refreshes the search box's suggestion list
async function loadRecents() {
  try {
    const resp = await fetch("/api/recents");
    const recents = await resp.json();
    const dl = $("recents");
    dl.innerHTML = "";
    (recents || []).forEach((q) => {
      const opt = document.createElement("option");
      opt.value = q;
      dl.appendChild(opt);
    });
  } catch {
    // suggestions are a convenience; ignore a failure
  }
}

// loadActiveTemplate refreshes the top-bar indicator
async function loadActiveTemplate() {
  try {
    const resp = await fetch("/api/template/active");
    const t = await resp.json();
    $("template-label").textContent = "Template: " + t.label;
  } catch {
    // leave the label as is on a failure
  }
}

// --- Templates manager ---

function openTemplates() {
  $("templates-modal").hidden = false;
  $("templates-status").textContent = "";
  $("templates-progress").hidden = true;
  $("templates-list").innerHTML = "<p>Loading catalog…</p>";
  loadTemplates();
}

function closeTemplates() {
  $("templates-modal").hidden = true;
}

async function loadTemplates() {
  let rows;
  try {
    const resp = await fetch("/api/templates");
    rows = await resp.json();
  } catch (err) {
    $("templates-list").innerHTML = "<p>Failed to load templates.</p>";
    return;
  }
  const list = $("templates-list");
  list.innerHTML = "";
  if (!rows || rows.length === 0) {
    list.innerHTML = "<p>No catalog available. Connect to fetch templates.</p>";
    return;
  }
  for (const row of rows) {
    const section = document.createElement("div");
    section.className = "template-section";
    const h = document.createElement("h3");
    h.textContent = row.name;
    section.appendChild(h);
    if (row.reason) {
      const r = document.createElement("p");
      r.className = "reason";
      r.textContent = row.reason;
      section.appendChild(r);
    }
    for (const v of row.versions || []) {
      section.appendChild(versionRow(row.name, v));
    }
    list.appendChild(section);
  }
}

function versionRow(name, v) {
  const line = document.createElement("div");
  line.className = "version-row";
  const label = document.createElement("span");
  label.textContent = v.label;
  line.appendChild(label);

  let trailing;
  if (v.active) {
    trailing = document.createElement("span");
    trailing.className = "active-tag";
    trailing.textContent = "active";
  } else if (v.action === "select" || v.action === "download") {
    trailing = document.createElement("button");
    trailing.type = "button";
    trailing.textContent = v.action === "download" ? "Download & select" : "Select";
    if (v.action === "download") trailing.className = "primary";
    trailing.addEventListener("click", () => selectTemplate(name, v.version));
  } else {
    trailing = document.createElement("span");
    trailing.className = "reason";
    trailing.textContent = v.reason || "";
  }
  line.appendChild(trailing);
  return line;
}

function selectTemplate(name, version) {
  const prog = $("templates-progress");
  prog.value = 0;
  prog.hidden = false;
  $("templates-status").textContent = `Preparing ${name} ${version}…`;

  fetch("/api/template/select", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, version }),
  })
    .then((resp) => {
      if (!resp.ok) return resp.text().then((t) => { throw new Error(t); });
      return resp.json();
    })
    .then(({ jobId }) => {
      const es = new EventSource(`/api/template/select/${jobId}/events`);
      es.onmessage = (ev) => {
        const e = JSON.parse(ev.data);
        if (e.done) {
          es.close();
          prog.hidden = true;
          if (e.error) {
            $("templates-status").textContent = "Failed: " + e.error;
            return;
          }
          loadActiveTemplate();
          closeTemplates();
          return;
        }
        prog.value = e.frac;
        if (e.step) $("templates-status").textContent = e.step;
      };
      es.onerror = () => {
        es.close();
        prog.hidden = true;
        $("templates-status").textContent = "Failed: connection lost";
      };
    })
    .catch((err) => {
      prog.hidden = true;
      $("templates-status").textContent = "Failed: " + err.message;
    });
}

// --- wiring ---

$("search-form").addEventListener("submit", (e) => {
  e.preventDefault();
  runSearch($("search-input").value);
});

$("editor-form").addEventListener("submit", (e) => {
  e.preventDefault();
  render();
});

// Enter in a single-line field previews; the multiline fields keep Enter for
// newlines and rely on the render button
for (const [field] of FIELDS) {
  const el = $("f-" + field);
  if (el.tagName === "TEXTAREA") continue;
  el.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      render();
    }
  });
}

$("reset-btn").addEventListener("click", resetEdits);
$("templates-btn").addEventListener("click", openTemplates);
$("templates-close").addEventListener("click", closeTemplates);
$("templates-modal").addEventListener("click", (e) => {
  if (e.target === $("templates-modal")) closeTemplates();
});

loadRecents();
loadActiveTemplate();
$("search-input").focus();

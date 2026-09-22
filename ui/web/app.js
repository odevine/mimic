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
  // res is the resolution settings as the server resolved them against the
  // active template, refreshed whenever either could have changed
  res: null,
  // preview is the finished preview job and the dpi the server rendered it at,
  // so a save whose output resolution already matches downloads it rather than
  // rendering the same pixels a second time
  preview: null,
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

// startRender posts the current editor values over the selected card for one
// target and resolves to the job id and the dpi the server rendered it at
function startRender(target) {
  const body = { base: state.selected, edits: readEdits(), target };
  return fetch("/api/render", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).then((resp) => {
    if (!resp.ok) return resp.text().then((t) => { throw new Error(t); });
    return resp.json();
  });
}

// render draws the preview at the preview resolution and drives the progress
// bar from the job's event stream
function render() {
  if (!state.selected) return;

  state.preview = null;
  $("save-btn").hidden = true;
  beginProgress("Rendering " + ($("f-name").value || "card") + "…");

  startRender("preview")
    .then(({ jobId, dpi }) => watchRender(jobId, dpi))
    .catch((err) => {
      endProgress();
      setStatus("Render failed: " + err.message);
    });
}

// watchRender opens the preview job's SSE stream, superseding any earlier one,
// and swaps in the finished image on the done event
function watchRender(jobId, dpi) {
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
      state.preview = { jobId, dpi };
      $("save-btn").hidden = false;
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

// save downloads the card at the output resolution. The preview on screen was
// rendered smaller, so saving renders again at the full size, unless the two
// resolutions agree and the preview already holds those pixels
function save() {
  if (!state.selected) return;
  const outputDPI = state.res && state.res.output ? state.res.output.dpi : 0;
  if (state.preview && state.preview.dpi === outputDPI) {
    downloadJob(state.preview.jobId);
    return;
  }

  $("save-btn").disabled = true;
  beginProgress(`Rendering at ${sizeLabel(state.res && state.res.output)}…`);

  startRender("output")
    .then(({ jobId }) => watchSave(jobId))
    .catch((err) => {
      endSave();
      setStatus("Save failed: " + err.message);
    });
}

// watchSave drives the progress bar from the output job's stream and starts the
// download once it finishes. It does not touch the preview image, so the small
// render stays on screen while the full one is prepared
function watchSave(jobId) {
  const es = new EventSource(`/api/render/${jobId}/events`);
  es.onmessage = (ev) => {
    const e = JSON.parse(ev.data);
    if (e.done) {
      es.close();
      endSave();
      if (e.error) {
        setStatus("Save failed: " + e.error);
        return;
      }
      downloadJob(jobId);
      setStatus(`Saved ${$("f-name").value || "card"}`);
      return;
    }
    stepProgress(e.step, e.frac);
  };
  es.onerror = () => {
    es.close();
    endSave();
    setStatus("Save failed: connection lost");
  };
}

// downloadJob hands the browser a finished job's PNG. The response carries an
// attachment disposition, so the click downloads rather than navigating
function downloadJob(jobId) {
  const a = document.createElement("a");
  a.href = `/api/render/${jobId}/image?download=1`;
  a.download = "";
  document.body.appendChild(a);
  a.click();
  a.remove();
}

function endSave() {
  endProgress();
  $("save-btn").disabled = false;
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

// --- Resolution ---

// sizeLabel is how a resolution reads everywhere it is shown: the pixels it
// produces, with the dpi those pixels print at
function sizeLabel(res) {
  if (!res) return "";
  return `${res.width} × ${res.height} (${res.dpi} dpi)`;
}

// loadResolution refreshes the settings from the server. They are resolved
// against the active template, so a template switch changes them and this runs
// again
async function loadResolution() {
  try {
    const resp = await fetch("/api/resolution");
    if (!resp.ok) throw new Error(await resp.text());
    state.res = await resp.json();
  } catch {
    // The picker reports its own failure when opened; a stale set of settings
    // is better than none, and the server clamps whatever it is sent anyway
  }
}

function openResolution() {
  $("resolution-modal").hidden = false;
  $("resolution-status").textContent = "";
  loadResolution().then(fillResolution);
}

function closeResolution() {
  $("resolution-modal").hidden = true;
}

// fillResolution builds both pickers from the presets and selects the entry
// matching each current setting, falling back to the custom row for a dpi that
// is not one of the presets
function fillResolution() {
  if (!state.res) {
    $("resolution-status").textContent = "Could not read the template's resolution.";
    return;
  }
  for (const which of ["preview", "output"]) {
    const select = $(which + "-select");
    select.innerHTML = "";
    for (const preset of state.res.presets || []) {
      const opt = document.createElement("option");
      opt.value = String(preset.dpi);
      opt.textContent = sizeLabel(preset) + (preset.native ? " · native" : "");
      select.appendChild(opt);
    }
    const custom = document.createElement("option");
    custom.value = "custom";
    custom.textContent = "Custom…";
    select.appendChild(custom);

    const dpi = state.res[which].dpi;
    const known = (state.res.presets || []).some((p) => p.dpi === dpi);
    select.value = known ? String(dpi) : "custom";
    $(which + "-dpi").value = String(dpi);
    syncResolutionRow(which);
  }
}

// syncResolutionRow shows the custom dpi input only when custom is chosen and
// updates the pixel size the current choice produces
function syncResolutionRow(which) {
  const custom = $(which + "-select").value === "custom";
  $(which + "-custom").hidden = !custom;
  $(which + "-size").textContent = sizeLabel(chosenResolution(which));
}

// chosenResolution is the resolution a picker currently describes: the chosen
// preset, or the pixel size a custom dpi works out to against the template's
// own. It clamps the same way the server does, so the label never promises a
// size a save would not produce
function chosenResolution(which) {
  const presets = (state.res && state.res.presets) || [];
  const value = $(which + "-select").value;
  if (value !== "custom") {
    return presets.find((p) => String(p.dpi) === value);
  }
  const native = presets[presets.length - 1];
  if (!native) return null;
  let dpi = parseInt($(which + "-dpi").value, 10);
  if (!Number.isFinite(dpi)) return null;
  dpi = Math.min(Math.max(dpi, state.res.minDpi), state.res.maxDpi);
  const factor = dpi / native.dpi;
  return {
    dpi,
    width: Math.round(native.width * factor),
    height: Math.round(native.height * factor),
  };
}

// saveResolution stores both settings and re-renders the preview, since the
// preview on screen was rendered at the resolution that just changed
function saveResolution() {
  const preview = chosenResolution("preview");
  const output = chosenResolution("output");
  if (!preview || !output) {
    $("resolution-status").textContent = "Enter a dpi for each.";
    return;
  }
  $("resolution-save").disabled = true;
  fetch("/api/resolution", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ previewDpi: preview.dpi, outputDpi: output.dpi }),
  })
    .then((resp) => {
      if (!resp.ok) return resp.text().then((t) => { throw new Error(t); });
      return resp.json();
    })
    .then((res) => {
      state.res = res;
      closeResolution();
      render();
    })
    .catch((err) => {
      $("resolution-status").textContent = "Failed: " + err.message;
    })
    .finally(() => {
      $("resolution-save").disabled = false;
    });
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
    if (row.description) {
      const d = document.createElement("p");
      d.className = "description";
      d.textContent = row.description;
      section.appendChild(d);
    }
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
          // A template authored at another size resolves both resolutions
          // differently, so the stale ones do not outlive the switch
          loadResolution();
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
$("save-btn").addEventListener("click", save);
$("templates-btn").addEventListener("click", openTemplates);
$("templates-close").addEventListener("click", closeTemplates);
$("templates-modal").addEventListener("click", (e) => {
  if (e.target === $("templates-modal")) closeTemplates();
});

$("resolution-btn").addEventListener("click", openResolution);
$("resolution-close").addEventListener("click", closeResolution);
$("resolution-save").addEventListener("click", saveResolution);
$("resolution-modal").addEventListener("click", (e) => {
  if (e.target === $("resolution-modal")) closeResolution();
});
for (const which of ["preview", "output"]) {
  $(which + "-select").addEventListener("change", () => {
    // Moving off a preset seeds the custom box with that preset's dpi, so the
    // number starts somewhere sensible rather than at whatever was last typed
    const chosen = $(which + "-select").value;
    if (chosen !== "custom") $(which + "-dpi").value = chosen;
    syncResolutionRow(which);
  });
  $(which + "-dpi").addEventListener("input", () => syncResolutionRow(which));
}

loadRecents();
loadActiveTemplate();
loadResolution();
$("search-input").focus();

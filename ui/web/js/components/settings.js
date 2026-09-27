import { api } from "../api.js";
import { $, h } from "../dom.js";
import { app } from "../state.js";
import { toast } from "./toast.js";
import { openCardData, cardDataChoice, initCardData } from "./cardData.js";
import { loadActiveTemplate } from "./templatePicker.js";
import { describeShape } from "../supports.js";

// The settings panel. Resolutions go through their own endpoint, which clamps
// them against the active template, and the interface settings go through
// /api/settings. Both persist in prefs.json

export function applyTheme(theme) {
  const t = theme || "dark";
  document.documentElement.dataset.theme = t;
  try {
    localStorage.setItem("mimic.theme", t);
  } catch {
    // a blocked store only costs the pre-paint theme on the next load
  }
}

// sizeLabel is how a resolution reads everywhere: the pixels it produces, with
// the dpi those pixels print at
export function sizeLabel(res) {
  if (!res) return "";
  return `${res.width} × ${res.height} px at ${res.dpi} dpi`;
}

export async function loadResolution() {
  try {
    app.resolution.value = await api.resolution();
  } catch {
    // The server clamps whatever it is sent, so stale settings are harmless
  }
}

// chosenResolution is the resolution a picker currently describes: a preset, or
// the size a custom dpi works out to against the template's own, clamped the
// way the server clamps it
function chosenResolution(which) {
  const res = app.resolution.peek();
  const presets = (res && res.presets) || [];
  const value = $(`${which}-select`).value;
  if (value !== "custom") return presets.find((p) => String(p.dpi) === value);
  const native = presets[presets.length - 1];
  if (!native) return null;
  let dpi = parseInt($(`${which}-dpi`).value, 10);
  if (!Number.isFinite(dpi)) return null;
  dpi = Math.min(Math.max(dpi, res.minDpi), res.maxDpi);
  const f = dpi / native.dpi;
  return { dpi, width: Math.round(native.width * f), height: Math.round(native.height * f) };
}

function syncRow(which) {
  const custom = $(`${which}-select`).value === "custom";
  $(`${which}-dpi`).hidden = !custom;
  $(`${which}-size`).textContent = sizeLabel(chosenResolution(which));
}

function fillResolution() {
  const res = app.resolution.peek();
  if (!res) {
    $("settings-status").textContent = "Could not read the template's resolution.";
    return;
  }
  for (const which of ["preview", "output"]) {
    const select = $(`${which}-select`);
    select.replaceChildren();
    for (const p of res.presets || []) {
      select.append(new Option(`${p.width} × ${p.height} · ${p.dpi} dpi${p.native ? " · native" : ""}`, String(p.dpi)));
    }
    select.append(new Option("Custom…", "custom"));
    const dpi = res[which].dpi;
    select.value = (res.presets || []).some((p) => p.dpi === dpi) ? String(dpi) : "custom";
    $(`${which}-dpi`).value = String(dpi);
    syncRow(which);
  }
}

async function open() {
  $("settings-status").textContent = "";
  const s = app.settings.peek();
  $("theme-select").value = s.theme || "dark";
  $("expand-printings").checked = !!s.expandPrintings;
  $("concurrency-input").value = s.concurrency ? String(s.concurrency) : "";
  $("output-dir-input").value = s.outputDir || "";
  openCardData(s.cardData);
  $("settings-dialog").showModal();
  await Promise.all([loadResolution().then(fillResolution), fillFaceTemplates()]);
}

async function save() {
  const preview = chosenResolution("preview");
  const output = chosenResolution("output");
  if (!preview || !output) {
    $("settings-status").textContent = "Enter a dpi for each resolution.";
    return;
  }
  const before = app.resolution.peek();
  const resChanged = !before || before.preview.dpi !== preview.dpi || before.output.dpi !== output.dpi;

  $("settings-save").disabled = true;
  try {
    if (resChanged) app.resolution.value = await api.saveResolution(preview.dpi, output.dpi);
    const next = {
      ...app.settings.peek(),
      theme: $("theme-select").value,
      expandPrintings: $("expand-printings").checked,
      concurrency: Math.min(Math.max(parseInt($("concurrency-input").value, 10) || 0, 0), 6),
      outputDir: $("output-dir-input").value.trim(),
      cardData: cardDataChoice(),
    };
    app.settings.value = await api.saveSettings(next);
    const facesChanged = await saveFaceTemplates();
    applyTheme(app.settings.peek().theme);
    $("settings-dialog").close();
    toast("Settings saved", "ok");
    if (resChanged) document.dispatchEvent(new CustomEvent("mimic:resolution-changed"));
    if (facesChanged) {
      await loadActiveTemplate();
      document.dispatchEvent(new CustomEvent("mimic:template-changed"));
    }
  } catch (err) {
    $("settings-status").textContent = `Failed: ${err.message}`;
  } finally {
    $("settings-save").disabled = false;
  }
}

export function initSettings() {
  initCardData();
  $("settings-btn").addEventListener("click", open);
  $("settings-save").addEventListener("click", save);
  // Previewing the theme as it is picked, reverted on cancel
  $("theme-select").addEventListener("change", () => applyTheme($("theme-select").value));
  $("settings-dialog").addEventListener("close", () => applyTheme(app.settings.peek().theme));
  for (const which of ["preview", "output"]) {
    $(`${which}-select`).addEventListener("change", () => {
      const v = $(`${which}-select`).value;
      if (v !== "custom") $(`${which}-dpi`).value = v;
      syncRow(which);
    });
    $(`${which}-dpi`).addEventListener("input", () => syncRow(which));
  }
}

// --- face templates ---

// faceRows holds the list the dialog opened with, so save sends only the
// shapes whose choice changed
let faceRows = [];

// shapeLabel names a face shape for its row, as in "Transform front faces"
function shapeLabel(row) {
  const text = describeShape({ role: row.role, kind: row.kind });
  return text.charAt(0).toUpperCase() + text.slice(1);
}

const choiceValue = (c) => (c ? `${c.name}@${c.version || ""}` : "");

async function fillFaceTemplates() {
  const box = $("face-templates");
  try {
    faceRows = await api.faceTemplates();
  } catch (err) {
    faceRows = [];
    box.replaceChildren(h("p", { class: "desc" }, `Could not read the installed templates: ${err.message}`));
    return;
  }
  if (!faceRows.length) {
    box.replaceChildren(h("p", { class: "desc" }, "No installed template renders anything beyond standard cards."));
    return;
  }
  box.replaceChildren(
    ...faceRows.map((row) => {
      const id = `face-${row.key.replace(/[^a-z0-9]/gi, "-")}`;
      const select = h("select", { id, "data-key": row.key });
      // Standard cards always render with the active template, so their row
      // offers exact versions and no automatic choice
      if (!row.primary) select.append(new Option(`Automatic · ${row.using || "none"}`, ""));
      for (const o of row.options) {
        if (!row.primary) select.append(new Option(`${o.name} · newest installed`, `${o.name}@`));
        for (const v of o.versions) select.append(new Option(`${o.name} · ${v === "local" ? "local" : v}`, `${o.name}@${v}`));
      }
      const current = choiceValue(row.chosen);
      // An active template with nothing installed, such as placeholders, still
      // shows as the current choice
      if (row.primary && ![...select.options].some((o) => o.value === current)) select.prepend(new Option(row.using, current));
      select.value = current;
      return h("div", { class: "setting" }, h("label", { for: id }, shapeLabel(row)), select);
    }),
  );
}

// saveFaceTemplates sends each changed choice and reports whether any was
// changed, so the caller can refresh what the page knows about templates
async function saveFaceTemplates() {
  let changed = false;
  for (const row of faceRows) {
    const select = document.querySelector(`#face-templates select[data-key="${row.key}"]`);
    if (!select || select.value === choiceValue(row.chosen)) continue;
    const [name, version] = select.value ? select.value.split("@") : ["", ""];
    await api.setFaceTemplate(row.key, name, version);
    changed = true;
  }
  return changed;
}

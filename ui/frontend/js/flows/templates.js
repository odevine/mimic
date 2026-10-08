import { api } from "../api.js";
import { $, h, icon } from "../dom.js";
import { app, effect } from "../state.js";
import { toast } from "../components/toast.js";
import { loadActiveTemplate } from "../components/templatePicker.js";
import { describeShape } from "../supports.js";

// The Templates mode keeps everything about templates in one place. Library
// lists the catalog and installs or activates a version, Defaults picks which
// installed template renders each kind of face, and Overrides holds the
// per-template and global overrides

const TABS = ["library", "defaults", "overrides"];
let tab = "library";
let busy = false;

// changed tells the rest of the page the templates moved, the same event a
// switch in any other place sends
async function changed() {
  await loadActiveTemplate();
  document.dispatchEvent(new CustomEvent("mimic:template-changed"));
}

// --- library ---

// versionRow draws one version with what can be done with it. A template that
// renders only other faces, such as transform, is never the active template,
// so its versions are downloaded or shown as installed rather than selected
function versionRow(t, v) {
  const row = h("div", { class: `tp-row${v.active ? " active" : ""}` }, h("span", { class: "tp-label" }, v.label));
  if (v.active) {
    row.append(h("span", { class: "tp-reason" }, icon("check", "icon"), "active"));
  } else if (!t.standard && v.selectable && v.cached) {
    row.append(h("span", { class: "tp-reason" }, icon("check", "icon"), "installed"));
  } else if (v.action === "select" || v.action === "download") {
    const label = v.action === "download" ? "Download" : "Select";
    row.append(
      h(
        "button",
        {
          type: "button",
          class: `btn${v.action === "download" ? "" : " subtle"}`,
          style: "min-height:0;height:24px",
          onclick: () => select(t, v),
        },
        v.action === "download" ? icon("download") : null,
        label,
      ),
    );
  } else {
    row.append(h("span", { class: "tp-reason" }, icon("lock", "icon gate-glyph"), v.reason || "unavailable"));
  }
  return row;
}

async function loadLibrary() {
  const list = $("tm-library-list");
  let rows;
  try {
    rows = await api.templates();
  } catch {
    list.replaceChildren(h("p", { class: "dim tm-empty" }, "Could not load the catalog."));
    return;
  }
  if (!rows || rows.length === 0) {
    list.replaceChildren(h("p", { class: "dim tm-empty" }, "No catalog available. Connect to the internet to fetch templates."));
    return;
  }
  list.replaceChildren(
    ...rows.map((t) =>
      h(
        "div",
        { class: "tp-template" },
        h(
          "div",
          { class: "tp-head" },
          h("div", { class: "tp-name" }, t.name),
          t.description ? h("div", { class: "tp-desc" }, t.description) : null,
          t.reason ? h("div", { class: "tp-desc" }, t.reason) : null,
        ),
        h("div", { class: "tp-versions" }, ...(t.versions || []).map((v) => versionRow(t, v))),
      ),
    ),
  );
}

// select activates a version, downloading it first when it is not cached,
// with the download's progress under the list. A template that renders only
// other faces is installed and left inactive, and the faces it supports pick
// it up from there
async function select(t, v) {
  const name = t.name;
  const install = !t.standard;
  if (busy) return;
  busy = true;
  const foot = $("tm-library-foot");
  const bar = h("div", { class: "bar" });
  const text = h("span", {}, `Preparing ${name} ${v.version}…`);
  foot.hidden = false;
  foot.replaceChildren(text, h("div", { class: "progress" }, bar));
  try {
    const { jobId } = await api.selectTemplate(name, v.version, install);
    await api.selectEvents(jobId, (step, frac) => {
      bar.style.width = `${frac * 100}%`;
      if (step) text.textContent = step;
    });
    foot.hidden = true;
    toast(install ? `Installed ${name} ${v.label}` : `Switched to ${name} ${v.label}`, "ok");
    await changed();
  } catch (err) {
    text.textContent = `Failed: ${err.message}`;
    bar.parentElement.remove();
  } finally {
    busy = false;
  }
}

// --- defaults ---

// shapeLabel names a face shape for its row, as in "Transform front faces"
function shapeLabel(row) {
  const text = describeShape({ role: row.role, kind: row.kind });
  return text.charAt(0).toUpperCase() + text.slice(1);
}

const choiceValue = (c) => (c ? `${c.name}@${c.version || ""}` : "");

async function loadDefaults() {
  const box = $("face-templates");
  let rows;
  try {
    rows = await api.faceTemplates();
  } catch (err) {
    box.replaceChildren(h("p", { class: "dim" }, `Could not read the installed templates: ${err.message}`));
    return;
  }
  box.replaceChildren(
    ...rows.map((row) => {
      const id = `face-${row.key.replace(/[^a-z0-9]/gi, "-")}`;
      const select = h("select", { id, "data-key": row.key });
      // Standard cards always render with the active template, so their row
      // offers exact versions and no automatic choice
      if (!row.primary) select.append(new Option(`Automatic · ${row.using || "none"}`, ""));
      for (const o of row.options) {
        if (!row.primary) select.append(new Option(`${o.name} · newest installed`, `${o.name}@`));
        for (const v of o.versions) select.append(new Option(`${o.name} · ${v}`, `${o.name}@${v}`));
      }
      const current = choiceValue(row.chosen);
      // An active template with nothing installed, such as placeholders, still
      // shows as the current choice
      if (row.primary && ![...select.options].some((o) => o.value === current)) select.prepend(new Option(row.using, current));
      select.value = current;
      select.addEventListener("change", () => setDefault(row, select));
      return h("div", { class: "setting" }, h("label", { for: id }, shapeLabel(row)), select);
    }),
  );
}

// setDefault saves one row's choice as soon as it is picked
async function setDefault(row, select) {
  const [name, version] = select.value ? select.value.split("@") : ["", ""];
  select.disabled = true;
  try {
    await api.setFaceTemplate(row.key, name, version);
    toast(`${shapeLabel(row)} now render with ${select.selectedOptions[0].text}`, "ok");
    await changed();
  } catch (err) {
    toast(err.message, "err", 6000);
    await loadDefaults();
  } finally {
    select.disabled = false;
  }
}

// --- tabs ---

function showTab(next) {
  if (!TABS.includes(next)) next = "library";
  tab = next;
  for (const chip of document.querySelectorAll("#mode-templates [data-tab]")) chip.setAttribute("aria-pressed", String(chip.dataset.tab === tab));
  for (const panel of document.querySelectorAll("#mode-templates [data-tab-panel]")) panel.hidden = panel.dataset.tabPanel !== tab;
  refresh();
}

function refresh() {
  if (app.mode.peek() !== "templates") return;
  if (tab === "library") loadLibrary();
  if (tab === "defaults") loadDefaults();
}

export function initTemplates() {
  for (const chip of document.querySelectorAll("#mode-templates [data-tab]")) {
    chip.addEventListener("click", () => showTab(chip.dataset.tab));
  }
  // Both lists are read when shown, so a download or a switch made elsewhere
  // is never stale here
  effect(() => {
    if (app.mode.value === "templates") refresh();
  });
  document.addEventListener("mimic:template-changed", refresh);
}

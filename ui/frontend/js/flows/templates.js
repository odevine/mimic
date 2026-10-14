import { api } from "../api.js";
import { $, h, icon } from "../dom.js";
import { app, effect } from "../state.js";
import { toast } from "../components/toast.js";
import { loadActiveTemplate } from "../components/templatePicker.js";
import { describeShape, layoutChips } from "../supports.js";
import { editor } from "./editor.js";

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

// sizeLabel says how big a download is, in the units a person reads
function sizeLabel(bytes) {
  if (!bytes) return "";
  return bytes >= 1e6 ? `${Math.round(bytes / 1e6)} MB` : `${Math.max(1, Math.round(bytes / 1e3))} KB`;
}

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
    const size = v.action === "download" ? sizeLabel(v.size) : "";
    const label = v.action === "download" ? `Download${size ? ` · ${size}` : ""}` : "Select";
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
  // An installed version can be copied into the editor and changed there
  if (v.cached && v.selectable) {
    row.append(
      h(
        "button",
        {
          type: "button",
          class: "btn subtle",
          style: "min-height:0;height:24px",
          title: "Make a copy of this template that you can change in the editor",
          onclick: () => editor.copyTemplate(t.name, v.version, t.title || t.name),
        },
        "Edit a copy",
      ),
    );
  }
  return row;
}

// aboutTemplate says who made an installed template and whose art it is cut from. Links open in the browser through the page's one handler
function aboutTemplate(d) {
  const by = [];
  if (d.author && d.author.name) by.push(d.author.url ? h("span", {}, "by ", h("a", { href: d.author.url }, d.author.name)) : `by ${d.author.name}`);
  const lines = [h("div", { class: "tp-desc" }, ...by.flatMap((part, i) => (i ? [" · ", part] : [part])))];
  if (d.credits && d.credits.length) {
    const credits = d.credits.map((c) => (c.url ? h("a", { href: c.url, title: c.note || "" }, c.name) : h("span", { title: c.note || "" }, c.name)));
    lines.push(h("div", { class: "tp-desc" }, "Art from ", ...credits.flatMap((el, i) => (i ? [", ", el] : [el]))));
  }
  return lines;
}

// layoutsOf lists the card layouts a template renders, each marked when this
// template is the one rendering it now. The page knows which template renders
// each shape from the active template's report
function layoutsOf(t) {
  const chips = layoutChips(t.shapes);
  if (!chips.length) return null;
  const faces = app.template.peek()?.faces || {};
  const using = (c) => c.shapes.some((sh) => faces[`${sh.role}/${sh.kind}`] === t.name);
  return h(
    "ul",
    { class: "tp-layouts", "aria-label": "Card layouts" },
    ...chips.map((c) => h("li", { class: `tp-chip${using(c) ? " on" : ""}`, title: using(c) ? `${c.title}, in use now` : c.title }, c.label)),
  );
}

// featureControl draws the control for one choice a template offers: a switch for
// on or off, a menu for one of several, and a slider for a number. Each keeps its
// value as soon as it changes
function featureControl(t, f) {
  const id = `feature-${t.name}-${f.name}`.replace(/[^a-z0-9]/gi, "-");
  const save = async (value, control, show) => {
    control.disabled = true;
    try {
      await api.setTemplateFeature(t.name, f.name, value);
      await changed();
    } catch (err) {
      toast(err.message, "err", 6000);
      await loadLibrary();
    } finally {
      control.disabled = false;
      show?.();
    }
  };
  let control;
  let readout = null;
  if (f.type === "bool") {
    control = h("input", { id, type: "checkbox", checked: f.value === true });
    control.addEventListener("change", () => save(control.checked, control));
    return h("div", { class: "setting tp-feature" }, h("label", { for: id }, f.label), h("span", { class: "inline" }, control));
  }
  if (f.type === "choice") {
    control = h("select", { id }, ...(f.values || []).map((v) => new Option(v, v)));
    control.value = f.value;
    control.addEventListener("change", () => save(control.value, control));
    return h("div", { class: "setting tp-feature" }, h("label", { for: id }, f.label), control);
  }
  control = h("input", { id, type: "range", min: f.min, max: f.max, step: f.step, value: f.value });
  readout = h("span", { class: "tp-readout" }, String(f.value));
  control.addEventListener("input", () => (readout.textContent = control.value));
  control.addEventListener("change", () => save(Number(control.value), control));
  return h("div", { class: "setting tp-feature" }, h("label", { for: id }, f.label), h("span", { class: "inline" }, control, readout));
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
    list.replaceChildren(h("p", { class: "dim tm-empty" }, "No templates are installed, and the catalog lists none this version of the app can use yet. If you are offline, connect to the internet and open this page again."));
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
          h("div", { class: "tp-title" }, h("span", { class: "tp-name" }, t.title || t.name), t.title && t.title !== t.name ? h("span", { class: "tp-id" }, t.name) : null),
          t.description ? h("div", { class: "tp-desc" }, t.description) : null,
          ...(t.details ? aboutTemplate(t.details) : []),
          layoutsOf(t),
        ),
        h("div", { class: "tp-versions" }, ...(t.versions || []).map((v) => versionRow(t, v))),
        t.details && t.details.features && t.details.features.length
          ? h("div", { class: "tp-features" }, ...t.details.features.map((f) => featureControl(t, f)))
          : null,
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
        const title = o.title || o.name;
        if (!row.primary) select.append(new Option(`${title} · newest installed`, `${o.name}@`));
        for (const v of o.versions) select.append(new Option(`${title} · ${v}`, `${o.name}@${v}`));
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
  document.dispatchEvent(new CustomEvent("mimic:templates-tab", { detail: tab }));
  refresh();
}

// showOverrides opens the Overrides tab, from the chip in the top bar
export function showOverrides() {
  location.hash = "templates";
  showTab("overrides");
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

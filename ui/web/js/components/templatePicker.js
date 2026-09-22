import { api } from "../api.js";
import { $, h, icon } from "../dom.js";
import { app } from "../state.js";
import { openPopover, closePopover, repositionPopover } from "./popover.js";
import { toast } from "./toast.js";

// The active template as a dropdown in the top bar. It lists every template and
// version the catalog knows, with an unselectable version dimmed and its reason
// shown, the same rows the server resolves for the manager

let busy = false;

export async function loadActiveTemplate() {
  try {
    app.template.value = await api.activeTemplate();
  } catch {
    // keep the last known template on a failure
  }
}

function versionRow(t, v, foot) {
  const row = h("div", { class: `tp-row${v.active ? " active" : ""}` }, h("span", { class: "tp-label" }, v.label));
  if (v.active) {
    row.append(h("span", { class: "tp-reason" }, icon("check", "icon"), "active"));
  } else if (v.action === "select" || v.action === "download") {
    const label = v.action === "download" ? "Download" : "Select";
    row.append(
      h(
        "button",
        {
          type: "button",
          class: `btn${v.action === "download" ? "" : " subtle"}`,
          style: "min-height:0;height:24px",
          onclick: () => select(t.name, v, foot),
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

function render(rows, foot) {
  const body = h("div", { class: "tp-list" });
  if (!rows || rows.length === 0) {
    body.append(h("p", { class: "dim", style: "padding:12px" }, "No catalog available. Connect to the internet to fetch templates."));
    return body;
  }
  for (const t of rows) {
    const block = h(
      "div",
      { class: "tp-template" },
      h(
        "div",
        { class: "tp-head" },
        h("div", { class: "tp-name" }, t.name),
        t.description ? h("div", { class: "tp-desc" }, t.description) : null,
        t.reason ? h("div", { class: "tp-desc" }, t.reason) : null,
      ),
    );
    const versions = h("div", { class: "tp-versions" });
    for (const v of t.versions || []) versions.append(versionRow(t, v, foot));
    block.append(versions);
    body.append(block);
  }
  return body;
}

async function select(name, v, foot) {
  if (busy) return;
  busy = true;
  const bar = h("div", { class: "bar" });
  const text = h("span", {}, `Preparing ${name} ${v.version}…`);
  foot.hidden = false;
  foot.replaceChildren(text, h("div", { class: "progress" }, bar));
  repositionPopover();
  try {
    const { jobId } = await api.selectTemplate(name, v.version);
    await api.selectEvents(jobId, (step, frac) => {
      bar.style.width = `${frac * 100}%`;
      if (step) text.textContent = step;
    });
    await loadActiveTemplate();
    closePopover();
    toast(`Switched to ${name} ${v.label}`, "ok");
    // Everything resolved against the template refreshes through this event
    document.dispatchEvent(new CustomEvent("mimic:template-changed"));
  } catch (err) {
    text.textContent = `Failed: ${err.message}`;
    bar.parentElement.remove();
  } finally {
    busy = false;
  }
}

async function open() {
  const trigger = $("template-trigger");
  const foot = h("div", { class: "tp-foot", hidden: true });
  const list = h("div", { class: "tp-list" }, h("p", { class: "dim", style: "padding:12px" }, "Loading catalog…"));
  const el = openPopover(trigger, [list, foot], { cls: "template-picker", focus: false });
  if (!el) return;
  try {
    const rows = await api.templates();
    list.replaceWith(render(rows, foot));
  } catch {
    list.replaceChildren(h("p", { class: "dim", style: "padding:12px" }, "Could not load the catalog."));
  }
  repositionPopover();
  const first = el.querySelector("button");
  if (first) first.focus();
}

export function initTemplatePicker() {
  $("template-trigger").addEventListener("click", open);
}

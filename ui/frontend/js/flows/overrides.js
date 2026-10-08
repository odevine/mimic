import { api } from "../api.js";
import { $, h, icon } from "../dom.js";
import { app } from "../state.js";
import { toast } from "../components/toast.js";
import { openPopover, closePopover } from "../components/popover.js";
import { loadResolution } from "../components/settings.js";
import { showOverrides } from "./templates.js";
import { list } from "./list.js";
import { FIELD_LABELS } from "../components/fieldLabels.js";

// Global overrides: rules that apply to every card you render, in order from the
// top. A rule fires when all its conditions hold, and sets or clears fields. A
// field you edit yourself, on a card or a list row, keeps your value. The rules
// save a moment after each change, and the app checks them whole, so a rule that
// does not check says why and nothing is saved until it does. Presets bundle the
// rules with the render settings

const OPS = [
  ["contains", "contains"],
  ["notContains", "does not contain"],
  ["equals", "is"],
  ["notEquals", "is not"],
  ["startsWith", "starts with"],
  ["endsWith", "ends with"],
  ["matches", "matches pattern"],
  ["empty", "is empty"],
  ["notEmpty", "is not empty"],
];
const NO_VALUE = new Set(["empty", "notEmpty"]);

let rules = []; // the list being edited, which the app has or will have
let counts = []; // how many list rows each rule holds for
let rowTotal = 0;
let presets = [];
let saveTimer = 0;
let countTimer = 0;
let listEl = null;

const enabled = () => rules.filter((r) => !r.disabled).length;

// --- the rule cards ---

function select(options, value, label, onChange) {
  const el = h("select", { "aria-label": label }, options.map(([v, text]) => h("option", { value: v }, text)));
  el.value = value;
  el.addEventListener("change", () => onChange(el.value));
  return el;
}

const fieldSelect = (value, label, onChange) => select(Object.entries(FIELD_LABELS), value, label, onChange);

function textInput(value, label, placeholder, onInput) {
  const el = h("input", { value, "aria-label": label, placeholder, spellcheck: false });
  el.addEventListener("input", () => onInput(el.value));
  return el;
}

function conditionLine(rule, c) {
  const value = textInput(c.value || "", "Value", c.op === "matches" ? "pattern" : "text", (v) => {
    c.value = v;
    changed();
  });
  value.hidden = NO_VALUE.has(c.op);
  return h(
    "div",
    { class: "rule-line" },
    fieldSelect(c.field, "Field", (v) => {
      c.field = v;
      changed();
    }),
    select(OPS, c.op, "Comparison", (v) => {
      c.op = v;
      value.hidden = NO_VALUE.has(v);
      changed();
    }),
    value,
    iconButton("close", "Remove this condition", () => {
      rule.when.splice(rule.when.indexOf(c), 1);
      render();
      changed();
    }),
  );
}

function actionLine(rule, a) {
  const value = textInput(a.value || "", "Value", "new value", (v) => {
    a.value = v;
    changed();
  });
  value.hidden = a.op === "clear";
  return h(
    "div",
    { class: "rule-line" },
    select(
      [
        ["set", "Set"],
        ["clear", "Clear"],
      ],
      a.op,
      "Action",
      (v) => {
        a.op = v;
        value.hidden = v === "clear";
        changed();
      },
    ),
    fieldSelect(a.field, "Field", (v) => {
      a.field = v;
      changed();
    }),
    value,
    iconButton("close", "Remove this action", () => {
      rule.then.splice(rule.then.indexOf(a), 1);
      render();
      changed();
    }),
  );
}

function iconButton(name, label, onClick) {
  return h("button", { class: "btn subtle icon-only", type: "button", "aria-label": label, "data-tip": label, onClick }, icon(name));
}

function ruleCard(rule, index) {
  const on = h("input", { type: "checkbox", checked: !rule.disabled, "aria-label": "Rule is on" });
  const card = h("div", { class: `rule-card${rule.disabled ? " off" : ""}`, dataset: { id: rule.id } });
  on.addEventListener("change", () => {
    rule.disabled = !on.checked;
    card.classList.toggle("off", rule.disabled);
    changed();
  });
  const name = textInput(rule.name || "", "Rule name", `Rule ${index + 1}`, (v) => {
    rule.name = v;
    changed();
  });
  name.classList.add("rule-name");
  const pill = h("span", { class: "pill idle rule-pill", hidden: true });
  const move = (by) => () => {
    const to = index + by;
    if (to < 0 || to >= rules.length) return;
    [rules[index], rules[to]] = [rules[to], rules[index]];
    render();
    changed();
  };
  const head = h(
    "div",
    { class: "rule-head" },
    on,
    name,
    pill,
    h("span", { class: "spacer" }),
    iconButton("chevron-up", "Move up", move(-1)),
    iconButton("chevron-down", "Move down", move(1)),
    iconButton("trash", "Delete this rule", () => {
      rules.splice(index, 1);
      render();
      changed();
    }),
  );
  head.querySelectorAll("button")[0].disabled = index === 0;
  head.querySelectorAll("button")[1].disabled = index === rules.length - 1;

  const when = h(
    "div",
    { class: "rule-block" },
    h("span", { class: "kw" }, "When"),
    h(
      "div",
      { class: "rule-lines" },
      rule.when.length ? rule.when.map((c) => conditionLine(rule, c)) : h("span", { class: "dim rule-always" }, "Always, for every card"),
      h(
        "button",
        {
          class: "btn subtle rule-add",
          type: "button",
          onClick: () => {
            rule.when.push({ field: "typeLine", op: "contains", value: "" });
            render();
            changed();
          },
        },
        icon("plus"),
        "Condition",
      ),
    ),
  );
  const then = h(
    "div",
    { class: "rule-block" },
    h("span", { class: "kw" }, "Then"),
    h(
      "div",
      { class: "rule-lines" },
      rule.then.map((a) => actionLine(rule, a)),
      h(
        "button",
        {
          class: "btn subtle rule-add",
          type: "button",
          onClick: () => {
            rule.then.push({ op: "set", field: "power", value: "" });
            render();
            changed();
          },
        },
        icon("plus"),
        "Action",
      ),
    ),
  );
  card.append(head, when, then);
  return card;
}

function render() {
  listEl.replaceChildren(
    ...(rules.length ? rules.map(ruleCard) : [h("p", { class: "dim rules-empty" }, "No rules yet. Add one to change cards as they render.")]),
  );
  updatePills();
}

// --- state shown elsewhere ---

// updatePills writes each rule's match count beside its name
function updatePills() {
  listEl.querySelectorAll(".rule-card").forEach((card, i) => {
    const rule = rules[i];
    const pill = card.querySelector(".rule-pill");
    if (!rule || !pill) return;
    const n = counts[i];
    pill.hidden = rowTotal === 0 || n === undefined;
    if (pill.hidden) return;
    const text = `matches ${n} of ${rowTotal}`;
    pill.className = `pill rule-pill ${rule.disabled ? "idle" : n > 0 ? "ok" : "idle"}`;
    pill.textContent = rule.disabled ? "off" : text;
    pill.dataset.tip = rule.disabled ? `Would ${text} rows of the list` : `Holds for ${n} of the ${rowTotal} rows in From a List`;
  });
}

// notify updates the chip in the top bar, the dot on the rail and the buttons
function notify() {
  const n = enabled();
  $("overrides-chip").hidden = n === 0;
  $("overrides-chip-label").textContent = `${n} ${n === 1 ? "rule" : "rules"} on`;
  $("rail-overrides-dot").hidden = n === 0;
  $("rules-disable-all").textContent = n === 0 && rules.length ? "Enable all" : "Disable all";
  $("rules-disable-all").disabled = rules.length === 0;
  const total = rules.length;
  $("rules-status").textContent = total ? `${total} ${total === 1 ? "rule" : "rules"}, ${n} on` : "";
}

function setError(message) {
  $("rules-error").hidden = !message;
  $("rules-error").textContent = message || "";
}

// --- saving and counting ---

// changed runs after any edit: it redraws what depends on the rules and saves
// them a moment later, once typing pauses
function changed() {
  notify();
  setError("");
  clearTimeout(saveTimer);
  saveTimer = setTimeout(save, 500);
  scheduleCounts();
}

async function save() {
  const snapshot = structuredClone(rules);
  try {
    await api.saveRules(snapshot);
    setError("");
    document.dispatchEvent(new CustomEvent("mimic:rules-changed"));
    refreshPresets();
  } catch (err) {
    setError(`Not saved. ${err.message}`);
  }
}

function scheduleCounts() {
  clearTimeout(countTimer);
  countTimer = setTimeout(refreshCounts, 300);
}

// refreshCounts asks the app how many rows of the current list each rule holds
// for, and shows nothing when no list is loaded or the rules do not check yet
async function refreshCounts() {
  const rows = list.ruleRows();
  rowTotal = rows.length;
  counts = [];
  if (rows.length && rules.length) {
    try {
      counts = await api.ruleMatches(structuredClone(rules), rows);
    } catch {
      counts = [];
    }
  }
  updatePills();
}

// --- presets ---

async function refreshPresets() {
  try {
    presets = await api.presets();
  } catch {
    return;
  }
  const current = presets.find((p) => p.current);
  $("preset-label").textContent = current ? current.name : presets.length ? "Custom" : "None";
}

async function applyPreset(name) {
  closePopover();
  try {
    const applied = await api.applyPreset(name);
    rules = applied.rules;
    app.settings.value = applied.settings;
    await loadResolution();
    render();
    notify();
    refreshCounts();
    refreshPresets();
    document.dispatchEvent(new CustomEvent("mimic:rules-changed"));
    document.dispatchEvent(new CustomEvent("mimic:resolution-changed"));
    toast(`Applied ${name}`, "ok");
  } catch (err) {
    toast(err.message, "err", 6000);
  }
}

async function deletePreset(name) {
  closePopover();
  if (!(await api.confirm("Delete preset", `Delete the preset “${name}”? The rules and settings now in force stay as they are.`, "Delete"))) return;
  try {
    await api.deletePreset(name);
    refreshPresets();
  } catch (err) {
    toast(err.message, "err");
  }
}

async function savePreset(name) {
  name = name.trim();
  if (!name) return;
  if (presets.some((p) => p.name.toLowerCase() === name.toLowerCase()) && !(await api.confirm("Replace preset", `Replace the preset “${name}” with the rules and settings now in force?`, "Replace"))) return;
  closePopover();
  // Anything still waiting to save goes first, so the preset holds what is shown
  clearTimeout(saveTimer);
  await save();
  try {
    const saved = await api.savePreset(name);
    toast(`Saved preset ${saved.name}`, "ok");
    refreshPresets();
  } catch (err) {
    toast(err.message, "err", 6000);
  }
}

function presetMenu() {
  const items = presets.map((p) =>
    h(
      "div",
      { class: "preset-item" },
      h(
        "button",
        { class: "menu-item", type: "button", onClick: () => applyPreset(p.name) },
        p.current ? icon("check") : h("span", { class: "icon" }),
        h("span", { class: "truncate" }, p.name),
        h("span", { class: "faint" }, `${p.rules} ${p.rules === 1 ? "rule" : "rules"}`),
      ),
      iconButton("trash", `Delete ${p.name}`, () => deletePreset(p.name)),
    ),
  );
  const name = h("input", { class: "preset-name", placeholder: "Name this preset", "aria-label": "Preset name", spellcheck: false });
  name.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      savePreset(name.value);
    }
  });
  return h(
    "div",
    { class: "menu preset-menu" },
    items.length ? items : h("div", { class: "menu-item faint" }, "No presets saved yet"),
    h("div", { class: "menu-sep" }),
    h("div", { class: "preset-save" }, name, h("button", { class: "btn", type: "button", onClick: () => savePreset(name.value) }, "Save")),
    h("p", { class: "note dim preset-note" }, "A preset holds these rules, the resolutions, the image format and the MPC options."),
  );
}

// --- wiring ---

function disableAll() {
  const turnOn = enabled() === 0;
  for (const r of rules) r.disabled = !turnOn;
  render();
  changed();
}

export async function initOverrides() {
  listEl = $("rules-list");
  $("rule-add").addEventListener("click", () => {
    rules.push({
      id: crypto.randomUUID(),
      name: "",
      when: [{ field: "typeLine", op: "contains", value: "" }],
      then: [{ op: "set", field: "power", value: "" }],
    });
    render();
    changed();
  });
  $("rules-disable-all").addEventListener("click", disableAll);
  $("preset-trigger").addEventListener("click", () => openPopover($("preset-trigger"), presetMenu(), { align: "end", cls: "preset-popover" }));
  $("overrides-chip").addEventListener("click", showOverrides);

  // The counts are read when the panel is shown and whenever the list changes,
  // since nothing else needs them
  document.addEventListener("mimic:templates-tab", (e) => {
    if (e.detail === "overrides") refreshCounts();
  });
  document.addEventListener("mimic:list-changed", () => {
    if (!$("tm-overrides").hidden && app.mode.peek() === "templates") scheduleCounts();
  });
  document.addEventListener("mimic:settings-saved", refreshPresets);
  document.addEventListener("mimic:resolution-changed", refreshPresets);

  try {
    rules = await api.rules();
  } catch {
    rules = [];
  }
  render();
  notify();
  refreshPresets();
}

import { api } from "../api.js";
import { $ } from "../dom.js";
import { signal, effect, batch } from "../state.js";
import { createCardEditor, ALL_FIELDS, valuesOf, isDirty } from "../components/cardEditor.js";

// The row inspector: the same field editor Single uses, beside the review table,
// editing one list row. Each field the row changes from its card, with the global
// rules' changes counted as part of the card, is kept on the row as a field
// override, the way a CSV column is, and a render applies the row's fields over
// the card. A field the user has not changed stays off the row, so a rule still
// reaches it and a rule that is turned off lets go of it

const EDITOR_FIELDS = new Set(ALL_FIELDS.map((s) => s.f));

const base = signal(null);
const edits = signal(null);
const reference = signal({});
const ruled = signal({});

let host = null; // what the list lets the inspector do to its rows
let row = null; // the row being edited
let editor = null;
let disposeWatch = null;
let seq = 0;
let settleTimer = 0;
let writing = false;

export const inspectedRow = () => row;

// ruledFor asks which fields the global rules change on this row's card, leaving
// alone the fields the row sets and reading them when a rule tests the card
async function ruledFor(card, fields) {
  try {
    return (await api.applyRules(card, fields || {})).fields || {};
  } catch {
    return {};
  }
}

// editedFields is what the row sets: every field that differs from the card with
// the rules' changes on it, plus any key the editor does not know, which the row
// came with and keeps
function editedFields(values) {
  const out = {};
  for (const [k, v] of Object.entries((row && row.fields) || {})) if (!EDITOR_FIELDS.has(k)) out[k] = v;
  for (const f of EDITOR_FIELDS) {
    if (isDirty(f, values, reference.peek())) out[f] = values[f];
  }
  return out;
}

// withKept puts the keys the editor does not know, which the row came with, back
// onto fields that came from somewhere else
function withKept(target, fields) {
  const out = {};
  for (const [k, v] of Object.entries(target.fields || {})) if (!EDITOR_FIELDS.has(k)) out[k] = v;
  return { ...out, ...fields };
}

function title() {
  const s = row.state.peek();
  $("inspector-title").textContent = s.card ? s.card.Name : "Row";
  const printing = s.card && s.card.SetCode ? `${s.card.SetCode.toUpperCase()}${s.card.CollectorNumber ? ` ${s.card.CollectorNumber}` : ""}` : "";
  $("inspector-sub").textContent = printing;
  $("inspector-qty").value = String(row.qty);
}

// load points the editor at the row as it stands now
async function load() {
  const target = row;
  const card = target.state.peek().card;
  if (!card) return close();
  const mine = ++seq;
  const fields = target.fields || {};
  const ruledNow = await ruledFor(card, fields);
  if (mine !== seq || row !== target) return;
  const ref = { ...valuesOf(card), ...ruledNow };
  writing = true;
  batch(() => {
    base.value = card;
    ruled.value = ruledNow;
    reference.value = ref;
    edits.value = { ...ref, ...fields };
  });
  writing = false;
  title();
}

// refreshRuled settles the reference after an edit, since a rule may test a
// field the edit changed
function refreshRuled() {
  clearTimeout(settleTimer);
  settleTimer = setTimeout(async () => {
    const target = row;
    if (!target) return;
    const card = target.state.peek().card;
    const fields = target.fields || {};
    const next = await ruledFor(card, fields);
    if (row !== target || JSON.stringify(next) === JSON.stringify(ruled.peek())) return;
    const ref = { ...valuesOf(card), ...next };
    writing = true;
    batch(() => {
      ruled.value = next;
      reference.value = ref;
      edits.value = { ...ref, ...fields };
    });
    writing = false;
  }, 400);
}

export function openInspector(target) {
  if (!target.state.peek().card) return;
  row = target;
  $("list-inspector").hidden = false;
  if (disposeWatch) disposeWatch();
  // A different card on the row, such as another choice for an ambiguous line,
  // loads again
  let first = true;
  disposeWatch = effect(() => {
    const card = target.state.value.card;
    if (first) {
      first = false;
      return;
    }
    if (card && base.peek() !== card) load();
  });
  title();
  load();
}

export function closeInspector() {
  seq++;
  clearTimeout(settleTimer);
  row = null;
  if (disposeWatch) disposeWatch();
  disposeWatch = null;
  $("list-inspector").hidden = true;
  host.onInspectorClosed();
}
const close = closeInspector;

// rowRemoved closes the inspector when its row is gone from the list
export function checkInspected(rows) {
  if (row && !rows.includes(row)) close();
}

export function initInspector(h) {
  host = h;
  editor = createCardEditor({ base, edits, reference, ruled, onEnter() {} });
  editor.mount($("inspector-form"));

  // An edit in the form is an edit to the row
  effect(() => {
    const values = edits.value;
    if (!row || !values || writing || !base.peek()) return;
    host.patchFields(row, editedFields(values));
    refreshRuled();
  });

  $("inspector-close").addEventListener("click", close);
  $("inspector-qty").addEventListener("change", (e) => {
    if (!row) return;
    const n = Math.max(1, Math.min(999, parseInt(e.target.value, 10) || 1));
    e.target.value = String(n);
    host.setQty([row], n);
  });
  $("inspector-revert").addEventListener("click", () => {
    if (!row) return;
    host.patchFields(row, {});
    load();
  });
  $("inspector-single").addEventListener("click", () => {
    if (!row) return;
    const target = row;
    const card = target.state.peek().card;
    host.openInSingle({
      card,
      fields: target.fields || {},
      label: card.Name,
      onApply(fields) {
        host.patchFields(target, withKept(target, fields));
        openInspector(target);
      },
    });
  });
  $("list-inspector").addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !e.target.closest("select")) {
      e.preventDefault();
      close();
    }
  });
  // The rules changed under an open row
  document.addEventListener("mimic:rules-changed", () => {
    if (row) load();
  });
}

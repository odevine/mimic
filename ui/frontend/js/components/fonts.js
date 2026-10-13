import { api } from "../api.js";
import { $, h } from "../dom.js";
import { toast } from "./toast.js";
import { folderNote, mergeOverrides, ownFont, pickedState, pickedValue, roleChoices } from "../fontmodel.js";

// The Fonts setting: which font each text role draws with, chosen from the fonts
// in the fonts folder, and for the app's own folder, adding and removing them.
// Adding and removing write straight to the folder and apply on the next render.
// The font picked for a role is kept with the other settings, by Save

let status = null; // the last Data.Fonts
let picks = null; // the font each role's picker shows, by role, once they have drawn

function roleRow(r) {
  const state = h("span", { class: "fonts-role-state" });
  const show = () => {
    const s = pickedState(r, status.pool, picks[r.role]);
    state.textContent = s.text;
    state.className = `fonts-role-state ${s.tone}`;
  };
  const select = h(
    "select",
    {
      class: "fonts-role-pick",
      "aria-label": `Font for ${r.label}`,
      title: ownFont(r),
      onchange: (e) => {
        picks[r.role] = e.target.value;
        show();
      },
    },
    ...roleChoices(r, status.pool, picks[r.role]).map((c) => h("option", { value: c.value }, c.label)),
  );
  select.value = picks[r.role];
  show();
  return h("li", { class: "fonts-role" }, h("span", { class: "fonts-role-name" }, r.label), select, state, r.error && h("p", { class: "fonts-note err" }, r.error));
}

function poolRow(f) {
  return h(
    "li",
    { class: "fonts-file" },
    h("span", { class: "fonts-file-name" }, f.name),
    h("span", { class: "fonts-file-path" }, f.file),
    f.removable && h("button", { class: "btn subtle", type: "button", "aria-label": `Remove ${f.name}`, onclick: () => remove(f) }, "Remove"),
  );
}

function draw() {
  const list = $("fonts-roles");
  if (!status) {
    $("fonts-source").textContent = "Checking the fonts…";
    list.replaceChildren();
    return;
  }
  $("fonts-source").textContent = folderNote(status.kind, status.path);
  if (!picks) {
    picks = {};
    for (const r of status.roles) picks[r.role] = pickedValue(r, status.pool);
  }
  for (const r of status.roles) if (!(r.role in picks)) picks[r.role] = pickedValue(r, status.pool);

  list.replaceChildren(
    ...(status.roles.length ? status.roles.map(roleRow) : [h("li", { class: "fonts-note" }, "Install a template to choose fonts for its text.")]),
  );
  $("fonts-metrics").hidden = !status.roles.length;

  const pool = $("fonts-pool");
  $("fonts-pool-title").textContent = status.writable ? "Fonts you added" : "Fonts in this folder";
  $("fonts-pool-title").hidden = !status.pool.length;
  pool.replaceChildren(
    ...status.pool.map(poolRow),
    ...status.rejected.map((f) => h("li", { class: "fonts-note err" }, `${f.file} could not be read: ${f.error}`)),
    ...status.duplicates.map((f) => h("li", { class: "fonts-note warn" }, `${f} has the same name as another font, so it is not used.`)),
  );
  $("fonts").classList.toggle("warn", status.rejected.length > 0);
  $("fonts-add").hidden = !status.writable;
  $("fonts-open").hidden = !status.path;
  $("fonts-reset").hidden = !status.writable || !status.pool.some((f) => f.removable);
}

async function refresh() {
  try {
    status = await api.fonts();
  } catch (err) {
    status = null;
    $("fonts-source").textContent = `Could not read the fonts: ${err.message}`;
    $("fonts-roles").replaceChildren();
    return;
  }
  draw();
}

async function add() {
  try {
    const next = await api.addFont();
    if (!next) return;
    status = next;
    draw();
    toast("Font added", "ok");
  } catch (err) {
    toast(err.message, "err", 6000);
  }
}

async function remove(f) {
  try {
    status = await api.removeFont(f.name);
    draw();
  } catch (err) {
    toast(err.message, "err");
  }
}

async function resetAll() {
  const mine = status.pool.filter((f) => f.removable);
  if (!(await api.confirm("Remove added fonts", "Remove every font you added? A role using one goes back to its own font.", "Remove"))) return;
  try {
    for (const f of mine) status = await api.removeFont(f.name);
  } catch (err) {
    toast(err.message, "err");
  }
  refresh();
}

async function openFolder() {
  try {
    await api.openFonts();
  } catch (err) {
    toast(err.message, "err");
  }
}

// openFonts refreshes the row when settings open, with each picker back at the
// saved font
export function openFonts() {
  picks = null;
  refresh();
}

// fontOverrides is the font assignments to save: what is saved now, changed by
// the pickers. It leaves them alone when the row never drew
export function fontOverrides(saved) {
  return mergeOverrides(saved, picks);
}

export function initFonts() {
  $("fonts-add").addEventListener("click", add);
  $("fonts-open").addEventListener("click", openFolder);
  $("fonts-reset").addEventListener("click", resetAll);
}

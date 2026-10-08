import { api } from "../api.js";
import { $, h } from "../dom.js";
import { toast } from "./toast.js";

// The Fonts setting: which font each card role draws with, and for the app's
// own fonts folder, adding and removing them. Changes write straight to the
// folder and apply on the next render, independent of the Save button

// What each role is called here, keyed by its manifest name
const ROLE_LABELS = {
  title: "Title",
  body: "Rules",
  "body-italic": "Flavor",
  mana: "Mana",
  "small-caps": "Artist",
  info: "Collector info",
};

const SOURCE_TEXT = {
  checkout: (p) => `Using ${p} from the checkout, so fonts are changed there rather than here.`,
  flag: (p) => `Using ${p} from the -fonts flag, so fonts are changed there rather than here.`,
  managed: () => "Fonts you add here are kept in the app's fonts folder and used on the next render.",
  none: () => "There is no config folder to keep fonts in, so every role uses its default.",
};

let status = null; // the last GET /api/fonts
let pendingRole = ""; // the role folder the file picker is choosing for

const fontName = (r) => [r.family, r.style].filter(Boolean).join(" ") || r.file || "Unknown font";

function roleRow(r, writable) {
  let source = "Yours";
  if (r.source === "default") source = "Default";
  else if (r.source === "borrowed") source = `Default, from ${ROLE_LABELS[r.borrows] || r.borrows}`;
  else if (r.source === "fallback") source = "Built-in fallback";

  const notes = [];
  if (r.rejected) notes.push(h("p", { class: "fonts-note err" }, `${r.rejected} could not be read: ${r.error}`));
  if (r.ignored && r.ignored.length) {
    notes.push(h("p", { class: "fonts-note warn" }, `Only the first font is used, so ${r.ignored.join(", ")} ${r.ignored.length === 1 ? "is" : "are"} ignored.`));
  }

  return h(
    "li",
    { class: "fonts-role" },
    h("span", { class: "fonts-role-name" }, ROLE_LABELS[r.role] || r.role),
    h("span", { class: "fonts-role-font", title: r.file || "" }, fontName(r)),
    h("span", { class: `fonts-role-source ${r.source}` }, source),
    h(
      "span",
      { class: "fonts-role-actions" },
      writable && h("button", { class: "btn subtle", type: "button", onclick: () => choose(r.folder) }, r.source === "user" ? "Replace" : "Add"),
      writable && r.source === "user" && h("button", { class: "btn subtle", type: "button", onclick: () => remove(r) }, "Remove"),
    ),
    notes,
  );
}

function draw() {
  const box = $("fonts");
  const list = $("fonts-roles");
  if (!status) {
    $("fonts-source").textContent = "Checking the fonts…";
    list.replaceChildren();
    return;
  }
  $("fonts-source").textContent = (SOURCE_TEXT[status.kind] || SOURCE_TEXT.none)(status.path);
  list.replaceChildren(...status.roles.map((r) => roleRow(r, status.writable)));
  box.classList.toggle("warn", status.roles.some((r) => r.rejected));
  $("fonts-open").hidden = !status.path;
  $("fonts-reset").hidden = !status.writable || !status.roles.some((r) => r.source === "user");
}

async function refresh() {
  try {
    status = await api.fonts();
  } catch (err) {
    status = null;
    $("fonts-source").textContent = `Could not read the fonts: ${err.message}`;
    return;
  }
  draw();
}

function choose(folder) {
  pendingRole = folder;
  const input = $("fonts-file");
  input.value = "";
  input.click();
}

async function upload() {
  const file = $("fonts-file").files[0];
  if (!file || !pendingRole) return;
  try {
    status = await api.addFont(pendingRole, file);
    draw();
    toast(`${file.name} added`, "ok");
  } catch (err) {
    toast(err.message, "err", 6000);
  }
}

async function remove(r) {
  try {
    status = await api.removeFont(r.folder);
    draw();
  } catch (err) {
    toast(err.message, "err");
  }
}

async function resetAll() {
  if (!confirm("Remove every font you added? Each role goes back to its default.")) return;
  try {
    for (const r of status.roles.filter((r) => r.source === "user")) status = await api.removeFont(r.folder);
  } catch (err) {
    toast(err.message, "err");
  }
  refresh();
}

async function openFolder() {
  try {
    const resp = await api.openFonts();
    if (!resp.ok) throw new Error((await resp.text()).trim() || resp.statusText);
  } catch (err) {
    toast(err.message, "err");
  }
}

// openFonts refreshes the row when settings open
export function openFonts() {
  refresh();
}

export function initFonts() {
  $("fonts-file").addEventListener("change", upload);
  $("fonts-open").addEventListener("click", openFolder);
  $("fonts-reset").addEventListener("click", resetAll);
}

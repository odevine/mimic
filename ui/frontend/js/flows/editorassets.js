import { api } from "../api.js";
import { $, h, icon } from "../dom.js";
import { effect } from "../state.js";
import { toast } from "../components/toast.js";
import {
  cellNote,
  groupMaterials,
  newArtWindow,
  newLayer,
  newTextBox,
  partNameFor,
  partOfItem,
  partSummary,
  removeFontOps,
  removeImageOps,
  roleChoice,
  roleOps,
} from "../editorassets.js";

// The editor's images, fonts and the things it adds: the grid of a layer's images, the
// template's fonts and which role draws with each, and the dialog that adds a text
// box, an art window or a layer. The editor flow hands over the store and the
// functions that change the draft

const PART_NAME = /^[a-z0-9][a-z0-9_-]*$/;

export function initAssets(host) {
  const { store } = host;

  // --- the images of a layer's part ---

  let thumbSeq = 0;
  // what the grid last drawn was drawn from, so an edit that changes nothing about
  // the part leaves it and its thumbnails alone
  let drawn = "";

  // bring asks for an image file, or takes the one at path, and has the app put it in a part.
  // It resolves to the app's answer, or null when the person cancelled or it was refused, which is said
  async function bring(part, material, slot, path) {
    host.flushNudge();
    const id = store.current.peek();
    try {
      return await (path ? api.importDraftImageAt(id, part, material, slot, path) : api.importDraftImage(id, part, material, slot));
    } catch (err) {
      toast(err.message, "err", 7000);
      return null;
    }
  }

  const sayWarnings = (res) => {
    for (const w of res.warnings || []) toast(w, "warn", 9000);
  };

  async function replaceImage(part, m, label) {
    const res = await bring(part.name, m.key, "");
    if (!res) return;
    sayWarnings(res);
    await host.recordApplied(res, `${m.state === "image" ? "Replace" : "Add"} the ${label} image of ${part.name}`);
  }

  function removeImage(part, m, label) {
    const src = store.source.peek();
    const variants = src && src.docs["template.json"] && src.docs["template.json"].parts && (src.docs["template.json"].parts[part.name] || {}).variants;
    host.commit(removeImageOps(part, m, variants), `Remove the ${label} image of ${part.name}`, "");
  }

  function cell(part, m) {
    const state = m.state;
    const noun = m.label;
    const action = state === "image" || state === "extra" ? `Replace the ${noun} image of ${part.name}` : `Add a ${noun} image to ${part.name}`;
    const thumb = h("span", { class: "ed-cell-thumb", dataset: { file: m.file || "" } }, state === "image" || state === "extra" ? null : state === "missing" ? "+" : "·");
    const main = h(
      "button",
      { class: "ed-cell-main", type: "button", "aria-label": `${action}. ${cellNote(m)}`, "data-tip": action, onclick: () => replaceImage(part, m, noun) },
      thumb,
      h("span", { class: "ed-cell-label" }, noun),
      h("span", { class: "ed-cell-note" }, cellNote(m)),
    );
    const remove =
      m.file && (state === "image" || state === "extra")
        ? h("button", { class: "btn subtle icon-only ed-cell-remove", type: "button", "aria-label": `Remove the ${noun} image of ${part.name}`, "data-tip": "Remove", onclick: () => removeImage(part, m, noun) }, icon("close"))
        : null;
    return h("div", { class: "ed-cell", dataset: { state, material: m.key } }, main, remove);
  }

  function drawImages() {
    const view = store.layout.value;
    const assets = store.assets.value;
    const item = view && store.selected.value ? host.itemOf(view, store.selected.value) : null;
    const part = partOfItem(item, assets);
    const box = $("ed-images");
    if (!part) {
      drawn = "";
      box.hidden = true;
      box.replaceChildren();
      return;
    }
    const key = JSON.stringify(part);
    if (key === drawn && !box.hidden) return;
    drawn = key;
    box.hidden = false;
    const slot = part.slot ? `follows the ${part.slot} colors` : "one image for every card";
    const groups = groupMaterials(part.materials).map((g) =>
      h("div", { class: "ed-grid-group", role: "group", "aria-label": g.title }, h("h5", {}, g.title), h("div", { class: "ed-grid" }, ...g.cells.map((m) => cell(part, m)))),
    );
    box.replaceChildren(
      h("h4", { class: "ed-subheading" }, `Images of ${part.name}`),
      h("p", { class: "ed-images-note dim" }, `${slot}. ${partSummary(part)}`),
      ...(part.problems || []).map((p) => h("p", { class: "ed-problem", role: "status" }, p)),
      ...groups,
    );
    loadThumbs(part);
  }

  // loadThumbs draws each image of a part small and puts it in its cell. A part that
  // is no longer the one shown is let go
  async function loadThumbs(part) {
    const seq = ++thumbSeq;
    const files = part.materials.filter((m) => m.file).map((m) => m.file);
    if (!files.length) return;
    let urls;
    try {
      urls = await api.draftThumbs(store.current.peek(), files, 160);
    } catch {
      return;
    }
    if (seq !== thumbSeq) return;
    for (const thumb of $("ed-images").querySelectorAll(".ed-cell-thumb")) {
      const url = urls[thumb.dataset.file];
      if (!url) continue;
      thumb.replaceChildren(h("img", { src: url, alt: "" }));
    }
  }

  // --- fonts and roles ---

  const fontName = (f) => [f.family, f.style].filter(Boolean).join(" ") || f.id;

  function drawFonts() {
    const assets = store.assets.value;
    const fonts = assets ? assets.fonts : [];
    $("ed-fonts").replaceChildren(
      ...(fonts.length ? fonts.map((f) => fontRow(f, assets)) : [h("p", { class: "dim" }, "The template ships no fonts. Its text uses the engine's own.")]),
    );
    drawRoles(assets);
  }

  function fontRow(f, assets) {
    const name = fontName(f);
    const license = h("input", {
      class: "ed-font-license",
      value: f.license || "",
      placeholder: "License",
      spellcheck: "false",
      autocomplete: "off",
      "aria-label": `License of the font ${name}`,
      onchange: (e) => {
        const text = e.target.value.trim();
        const op = { kind: "template", path: ["fonts", f.id, "license"] };
        host.commit([text ? { op: "set", ...op, value: text } : { op: "unset", ...op }], `Set the license of the font ${name}`, "");
      },
    });
    return h(
      "div",
      { class: `ed-font${f.missing ? " missing" : ""}` },
      h(
        "div",
        { class: "ed-font-head" },
        h("span", { class: "ed-font-name", title: f.file }, name, f.missing ? " (file missing)" : ""),
        h("button", { class: "btn subtle icon-only", type: "button", "aria-label": `Remove the font ${name}`, "data-tip": "Remove", onclick: () => host.commit(removeFontOps(f, assets), `Remove the font ${name}`, "") }, icon("trash")),
      ),
      license,
    );
  }

  const builtinLabel = (id) => `Built-in ${id.replace(/^mimic-/, "").replace(/-/g, " ")}`;

  function drawRoles(assets) {
    const box = $("ed-roles");
    if (!assets) {
      box.replaceChildren();
      return;
    }
    box.replaceChildren(
      ...assets.roles.map((r) => {
        const id = `ed-role-${r.role}`;
        const select = h("select", { id, onchange: (e) => host.commit(roleOps(r, e.target.value, r.label), `Set the font for ${r.label.toLowerCase()}`, "") });
        select.append(new Option(r.default ? `Default (${builtinLabel(r.default)})` : "Default", ""));
        for (const b of assets.builtins) select.append(new Option(builtinLabel(b.id), `builtin:${b.id}`));
        for (const f of assets.fonts) select.append(new Option(fontName(f), `font:${f.id}`));
        select.value = roleChoice(r);
        return h("div", { class: "ed-lever" }, h("label", { for: id }, r.label), select);
      }),
    );
  }

  async function addFont() {
    host.flushNudge();
    let res;
    try {
      res = await api.importDraftFont(store.current.peek(), []);
    } catch (err) {
      toast(err.message, "err", 7000);
      return;
    }
    if (!res) return;
    sayWarnings(res);
    const font = (store.assets.peek() || { fonts: [] }).fonts.find((f) => f.id === res.id);
    await host.recordApplied(res, `Add the font ${font ? fontName(font) : res.id}`);
    toast("Added. Choose where it is used under Text roles", "ok");
  }

  // --- adding things ---

  const addError = (text) => {
    $("ed-add-error").textContent = text || "";
    $("ed-add-error").hidden = !text;
  };

  function showAddKind() {
    const kind = $("ed-add-kind").value;
    $("ed-add-text").hidden = kind !== "text";
    $("ed-add-art").hidden = kind !== "art";
    $("ed-add-layer").hidden = kind !== "layer";
    showNewPart();
  }

  function showNewPart() {
    $("ed-add-newpart").hidden = $("ed-add-part").value !== "";
  }

  function openAdd() {
    const view = store.layout.peek();
    if (!view || !view.editable) return;
    const vocab = store.vocab.peek() || { draws: [], slots: [] };
    const assets = store.assets.peek() || { parts: [] };
    addError("");
    fill($("ed-add-draws"), vocab.draws.map((d) => [d, d]));
    const layers = view.items.filter((it) => it.kind === "layer");
    fill($("ed-add-after"), [["", "Nothing, the bottom of the card"], ...layers.map((l) => [l.key, l.key])]);
    $("ed-add-after").value = layers.length ? layers[layers.length - 1].key : "";
    fill($("ed-add-part"), [...assets.parts.map((p) => [p.name, p.name]), ["", "A new part from a PNG image"]]);
    $("ed-add-part").value = assets.parts.length ? assets.parts[0].name : "";
    fill($("ed-add-slot"), [["", "Nothing, one image for every card"], ...vocab.slots.map((s) => [s, `The ${s} colors`])]);
    $("ed-add-partname").value = "";
    $("ed-add-kind").value = "text";
    showAddKind();
    $("ed-add-dialog").showModal();
    $("ed-add-kind").focus();
  }

  function fill(select, options) {
    select.replaceChildren(...options.map(([value, label]) => new Option(label, value)));
  }

  async function confirmAdd() {
    const view = store.layout.peek();
    if (!view) return;
    const kind = $("ed-add-kind").value;
    const btn = $("ed-add-confirm");
    addError("");
    btn.disabled = true;
    try {
      if (kind === "text") {
        const op = newTextBox(view, $("ed-add-draws").value, host.takenKeys(view, "text"));
        if (await host.commit([op], `Add the text box ${op.key}`, "")) host.select(`text:${op.key}`);
      } else if (kind === "art") {
        const count = view.items.filter((it) => it.kind === "art").length;
        const op = newArtWindow(view, $("ed-add-after").value, count);
        if (await host.commit([op], "Add an art window", "")) host.select(`art:${op.key}`);
      } else if (!(await addLayer(view))) {
        return;
      }
      $("ed-add-dialog").close();
    } finally {
      btn.disabled = false;
    }
  }

  // addLayer adds a layer for an existing part, or for a new one whose first image
  // is chosen now. It resolves to false when the dialog should stay open
  async function addLayer(view) {
    const entries = view.items.filter((it) => it.kind === "layer").map((it) => ({ id: it.key, part: it.part }));
    const chosen = $("ed-add-part").value;
    if (chosen) {
      const op = newLayer(view.layout, chosen, entries);
      const made = await host.commit([op], `Add the layer ${op.key}`, "");
      if (made) host.select(`layer:${op.key}`);
      return made;
    }
    const name = $("ed-add-partname").value.trim();
    const wrong = newPartProblem(name);
    if (wrong) {
      addError(wrong);
      return false;
    }
    return await makePartLayer(view, name, $("ed-add-slot").value, "");
  }

  // newPartProblem says what is wrong with a name for a part that is to be made, or "" when it is fine
  function newPartProblem(name) {
    if (!PART_NAME.test(name)) return "A part's name is lowercase letters, digits, underscores and hyphens";
    const assets = store.assets.peek() || { parts: [] };
    if (assets.parts.some((p) => p.name === name)) return `The template has a part named ${name} already. Choose it above, or give this one another name`;
    return "";
  }

  // makePartLayer brings an image in as the first image of a new part and adds a layer for it,
  // which is one step of undo. The image is chosen now, or is the file at path. It resolves to
  // false when the image was not brought in
  async function makePartLayer(view, name, slot, path) {
    const entries = view.items.filter((it) => it.kind === "layer").map((it) => ({ id: it.key, part: it.part }));
    const res = await bring(name, "any", slot, path);
    if (!res) return false;
    sayWarnings(res);
    const op = newLayer(view.layout, name, entries);
    const made = await host.commit([op], `Add the layer ${op.key}`, "", res.inverse);
    if (!made) await host.recordApplied(res, `Add the image of ${name}`);
    else host.select(`layer:${op.key}`);
    return true;
  }

  // --- images dropped on the window ---

  // dropped holds the PNG files dropped and not yet added, the first being the one the dialog is for
  let dropped = [];
  let advancing = false;

  const fileName = (path) => path.split(/[\\/]/).pop();

  // takeDrops starts the dialog for dropped files that are images, and says so when none is. It
  // does nothing when the editor is not what is showing, or a template that cannot be changed
  function takeDrops(files) {
    if (!host.droppable()) return;
    const pngs = files.filter((f) => /\.png$/i.test(f));
    if (!pngs.length) {
      toast("The editor takes layer images as PNG. Export the layer as a PNG first", "warn", 6000);
      return;
    }
    const open = $("ed-drop-dialog").open;
    dropped = open ? [...dropped, ...pngs] : pngs;
    if (!open) askDrop();
  }

  function dropError(text) {
    $("ed-drop-error").textContent = text || "";
    $("ed-drop-error").hidden = !text;
  }

  // askDrop asks what the first dropped image is for, with a name for its part from the file's
  function askDrop() {
    const view = store.layout.peek();
    const vocab = store.vocab.peek() || { slots: [] };
    const assets = store.assets.peek() || { parts: [] };
    if (!view || !dropped.length) return;
    const path = dropped[0];
    dropError("");
    $("ed-drop-title").textContent = dropped.length > 1 ? `Add this image, ${dropped.length} to go` : "Add this image";
    $("ed-drop-file").textContent = fileName(path);
    $("ed-drop-name").value = partNameFor(fileName(path), assets.parts.map((p) => p.name));
    fill($("ed-drop-slot"), [["", "Nothing, one image for every card"], ...vocab.slots.map((s) => [s, `The ${s} colors`])]);
    $("ed-drop-dialog").showModal();
    $("ed-drop-name").select();
  }

  async function confirmDrop() {
    const view = store.layout.peek();
    if (!view || !dropped.length) return;
    const name = $("ed-drop-name").value.trim();
    const wrong = newPartProblem(name);
    if (wrong) {
      dropError(wrong);
      return;
    }
    const btn = $("ed-drop-confirm");
    btn.disabled = true;
    try {
      const path = dropped[0];
      const made = await makePartLayer(view, name, $("ed-drop-slot").value, path);
      if (!made) {
        $("ed-drop-dialog").close();
        dropped = [];
        return;
      }
      dropped = dropped.slice(1);
      advancing = true;
      $("ed-drop-dialog").close();
      if (dropped.length) askDrop();
    } finally {
      btn.disabled = false;
    }
  }

  // The card and the layers list light up while a file is dragged over them
  for (const el of [$("ed-stage"), $("ed-sec-layers-body")]) {
    el.addEventListener("dragover", () => el.classList.add("drop"));
    el.addEventListener("dragleave", () => el.classList.remove("drop"));
    el.addEventListener("drop", () => el.classList.remove("drop"));
  }
  api.onFilesDropped((files) => {
    $("ed-stage").classList.remove("drop");
    $("ed-sec-layers-body").classList.remove("drop");
    takeDrops(files);
  });
  $("ed-drop-confirm").addEventListener("click", confirmDrop);
  $("ed-drop-name").addEventListener("input", () => dropError(""));
  $("ed-drop-name").addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      confirmDrop();
    }
  });
  // A dialog closed any other way than by adding an image drops the images still waiting
  $("ed-drop-dialog").addEventListener("close", () => {
    if (advancing) advancing = false;
    else dropped = [];
  });

  $("ed-add").addEventListener("click", openAdd);
  $("ed-add-kind").addEventListener("change", showAddKind);
  $("ed-add-part").addEventListener("change", showNewPart);
  $("ed-add-confirm").addEventListener("click", confirmAdd);
  $("ed-add-partname").addEventListener("input", () => addError(""));
  $("ed-font-add").addEventListener("click", addFont);
  $("ed-add-partname").addEventListener("blur", () => {
    const el = $("ed-add-partname");
    if (el.value.trim()) el.value = partNameFor(el.value, []);
  });

  effect(drawImages);
  effect(drawFonts);
}

// What the editor shows of a draft's images and fonts, and the edits that add and
// remove elements, as pure functions. The app says what each part holds, and this
// turns it into the cells of a variant grid, names for new things and the operations
// to send

const COLORS = { w: "White", u: "Blue", b: "Black", r: "Red", g: "Green" };

// materialLabel is what a material is called to a person: a color, a pair or run of
// colors, or one of the other names the engine has
export function materialLabel(key) {
  if (key === "any") return "Any card";
  if (key.length >= 1 && [...key].every((c) => COLORS[c])) return [...key].map((c) => COLORS[c]).join("-");
  return key.replace(/_/g, " ").replace(/^./, (c) => c.toUpperCase());
}

// groupMaterials sorts a part's materials into the groups a grid is shown in: the
// image for any card, single colors, pairs and runs of colors, and everything else
export function groupMaterials(materials) {
  const groups = [
    { id: "any", title: "Any card", cells: [] },
    { id: "colors", title: "Colors", cells: [] },
    { id: "runs", title: "Pairs and runs", cells: [] },
    { id: "other", title: "Other", cells: [] },
  ];
  for (const m of materials || []) {
    const colors = [...m.key].every((c) => COLORS[c]);
    let at = 3;
    if (m.key === "any") at = 0;
    else if (colors && m.key.length === 1) at = 1;
    else if (colors) at = 2;
    groups[at].cells.push({ ...m, label: materialLabel(m.key) });
  }
  return groups.filter((g) => g.cells.length);
}

// cellNote is the short line under a cell saying what draws the material
export function cellNote(m) {
  switch (m.state) {
    case "image":
      return m.width ? `${m.width} × ${m.height}` : "image";
    case "blended":
      return `blended from ${(m.from || []).map(materialLabel).join(" and ")}`;
    case "any":
      return "uses the any image";
    case "extra":
      return "not read by this slot";
    default:
      return "no image";
  }
}

// partSummary says how many of a part's materials have an image of their own and how
// many nothing draws, for the heading of its grid
export function partSummary(part) {
  const ms = (part && part.materials) || [];
  const own = ms.filter((m) => m.state === "image").length;
  const missing = ms.filter((m) => m.state === "missing").length;
  if (ms.length <= 1) return own ? "1 image" : "no image";
  const parts = [`${own} of ${ms.filter((m) => m.state !== "extra").length} materials have an image`];
  if (missing) parts.push(`${missing} would leave a hole`);
  return parts.join(", ");
}

// partOfItem finds the part a layer draws from in what the app said of the draft's
// images, or null
export function partOfItem(item, assets) {
  if (!item || item.kind !== "layer" || !assets) return null;
  return assets.parts.find((p) => p.name === item.part) || null;
}

// boxKey makes the name of a text box from a name, which is letters, digits and
// underscores starting with a letter, and not one of those taken
export function boxKey(base, taken) {
  const clean = String(base || "box").replace(/[^A-Za-z0-9_]+/g, "_").replace(/^[^A-Za-z]+/, "").replace(/_+$/, "") || "box";
  const has = new Set(taken);
  if (!has.has(clean)) return clean;
  for (let n = 2; ; n++) {
    if (!has.has(`${clean}_${n}`)) return `${clean}_${n}`;
  }
}

// idKey makes the id of a layer, which is lowercase letters, digits, underscores and
// hyphens, and not one of those taken
export function idKey(base, taken) {
  const clean = String(base || "layer").toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^[^a-z0-9]+/, "").replace(/-+$/, "") || "layer";
  const has = new Set(taken);
  if (!has.has(clean)) return clean;
  for (let n = 2; ; n++) {
    if (!has.has(`${clean}-${n}`)) return `${clean}-${n}`;
  }
}

// partNameFor turns a file or a name typed into the name of a part, which is lowercase
// letters, digits, underscores and hyphens, and not one of those taken
export function partNameFor(text, taken) {
  const base = String(text || "")
    .replace(/\.[A-Za-z0-9]+$/, "")
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, "_")
    .replace(/^[^a-z0-9]+/, "")
    .replace(/[_-]+$/, "");
  const name = base || "layer";
  const has = new Set(taken);
  if (!has.has(name)) return name;
  for (let n = 2; ; n++) {
    if (!has.has(`${name}_${n}`)) return `${name}_${n}`;
  }
}

// The standard size of a new text box, as a share of the finished image
const BOX_WIDTH = 0.7;
const BOX_HEIGHT = 0.08;

// newTextBox is the operation that adds a text box drawing what is given, placed in
// the middle of the finished image. The box is named for what it draws, made unique
export function newTextBox(view, draws, taken) {
  const w = Math.round(view.delivered.width * BOX_WIDTH);
  const h = Math.round(view.delivered.height * BOX_HEIGHT);
  const size = Math.max(8, Math.round(h * 0.6));
  const key = boxKey(draws, taken);
  return {
    op: "add",
    layout: view.layout,
    kind: "text",
    key,
    value: { draws, text: { fontSize: size, color: "#000000", align: "left", vAlign: "center" } },
    rect: { x: Math.round((view.delivered.width - w) / 2), y: Math.round((view.delivered.height - h) / 2), width: w, height: h },
  };
}

// newArtWindow is the operation that adds an art window after a layer, at the end of
// the list, covering the upper middle of the finished image
export function newArtWindow(view, after, count) {
  const w = Math.round(view.delivered.width * 0.8);
  const h = Math.round(view.delivered.height * 0.4);
  return {
    op: "add",
    layout: view.layout,
    kind: "art",
    key: String(count),
    value: after ? { after } : {},
    rect: { x: Math.round((view.delivered.width - w) / 2), y: Math.round(view.delivered.height * 0.15), width: w, height: h },
  };
}

// newLayer is the operation that adds a layer drawing a part, just above another
// layer or at the top of the stack. The id is the part's name, made unique
export function newLayer(layout, part, entries, to) {
  const ids = entries.map((e) => e.id || e.part);
  const id = idKey(part, ids);
  const value = id === part ? { part } : { id, part };
  return { op: "add", layout, kind: "layer", key: id, to: to === undefined ? entries.length : to, value };
}

// duplicateOp copies an element, a little way down and to the right so the copy can
// be told from the original, which is a share of the finished image
export function duplicateOp(view, item, taken) {
  const step = Math.max(8, Math.round(view.delivered.width * 0.02));
  const op = { op: "duplicate", layout: view.layout, kind: item.kind, key: item.key, dx: step, dy: step };
  if (item.kind === "text") op.as = boxKey(`${item.key}_copy`, taken);
  else if (item.kind === "layer") op.as = idKey(`${item.key}-copy`, taken);
  return op;
}

// deleteOp takes an element out of the layout
export const deleteOp = (view, item) => ({ op: "delete", layout: view.layout, kind: item.kind, key: item.key });

// whatDeleteTakes says what removing an element does, for a label and a message
export function deleteLabel(item, name) {
  return item.kind === "layer" ? `Delete layer ${name}` : `Delete ${name}`;
}

// roleOps are the edits that make a role draw with a choice: "" is the role's
// default, builtin:<id> an engine font and font:<id> one the template ships. A role
// the template defines keeps its label
export function roleOps(role, choice, label) {
  const base = { op: "set", kind: "template", path: ["roles", role.role] };
  if (!choice) return [{ op: "unset", kind: "template", path: ["roles", role.role] }];
  const [kind, id] = [choice.slice(0, choice.indexOf(":")), choice.slice(choice.indexOf(":") + 1)];
  const value = kind === "font" ? { font: id } : { builtin: id };
  if (!role.standard && label && label !== role.role) value.label = label;
  return [{ ...base, value }];
}

// roleChoice is the choice a role's select shows, in the form roleOps takes
export function roleChoice(role) {
  if (role.font) return `font:${role.font}`;
  if (role.builtin) return `builtin:${role.builtin}`;
  return "";
}

// removeFontOps removes a font from the template: the roles that draw with it go back
// to their defaults, the entry is unset and the file is dropped
export function removeFontOps(font, assets) {
  const ops = [];
  for (const r of assets.roles) {
    if (r.font === font.id) ops.push({ op: "unset", kind: "template", path: ["roles", r.role] });
  }
  ops.push({ op: "unset", kind: "template", path: ["fonts", font.id] });
  ops.push({ op: "drop", file: font.file });
  return ops;
}

// removeImageOps takes an image out of a part. A material named in the part's own
// variants is taken out of them, and the file is dropped
export function removeImageOps(part, material, variants) {
  const ops = [];
  if (variants && Object.prototype.hasOwnProperty.call(variants, material.key)) {
    ops.push({ op: "unset", kind: "part", key: part.name, path: ["variants", material.key] });
  }
  ops.push({ op: "drop", file: material.file });
  return ops;
}

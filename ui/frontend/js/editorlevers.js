// The controls the editor offers for the selected item, as data. A field names the
// place in the layout or template file it reads and writes, how its text is read, and
// the choices it offers, so the form is built from this and the rules for what a
// typed value means and what edit it makes are here to be tested without a document

const ALIGN = ["left", "center", "right"];
const VALIGN = ["top", "center", "bottom"];
const TURNS = ["0", "90", "180", "270"];

const choice = (values) => values.map((v) => ({ value: v, label: v }));

// number fields
const int = (id, label, path, extra = {}) => ({ id, label, path, type: "int", ...extra });
const num = (id, label, path, extra = {}) => ({ id, label, path, type: "number", ...extra });

// A rectangle's four numbers, the place and the size
const rect = (prefix) => [
  int("x", "X", [...prefix, "x"], { required: true }),
  int("y", "Y", [...prefix, "y"], { required: true }),
  int("width", "Width", [...prefix, "width"], { min: 1, required: true }),
  int("height", "Height", [...prefix, "height"], { min: 1, required: true }),
];

// The standard roles are in every template, and a template adds its own
function roleChoices(ctx) {
  const roles = [...(ctx.vocab.fontRoles || []), ...(ctx.templateRoles || []).filter((r) => !(ctx.vocab.fontRoles || []).includes(r))];
  return choice(roles);
}

const SHADOW_DEFAULT = { distance: 0.08, angle: 25 };

// fieldGroups lists the groups of fields for a kind of item. ctx has the vocabulary
// the app sent, the roles the template defines, and the ids of the layout's stack
export function fieldGroups(kind, ctx) {
  const vocab = ctx.vocab || {};
  switch (kind) {
    case "text":
      return [
        { title: "Place", fields: rect(["rect"]) },
        {
          title: "Content",
          fields: [
            { id: "draws", label: "Draws", path: ["draws"], type: "choice", options: choice(vocab.draws || []), empty: "the box's own name" },
            { id: "condition", label: "Shows when", path: ["condition"], type: "condition" },
          ],
        },
        {
          title: "Text",
          fields: [
            { id: "font", label: "Font role", path: ["text", "font"], type: "choice", options: roleChoices(ctx), empty: "body" },
            num("fontSize", "Size", ["text", "fontSize"], { min: 1, required: true }),
            { id: "color", label: "Color", path: ["text", "color"], type: "color" },
            { id: "align", label: "Align", path: ["text", "align"], type: "choice", options: choice(ALIGN), empty: "left" },
            { id: "vAlign", label: "Vertical", path: ["text", "vAlign"], type: "choice", options: choice([...VALIGN, "baseline"]), empty: "top" },
            num("lineSpacing", "Line spacing", ["text", "lineSpacing"], { min: 0, step: 0.05 }),
            num("minFontSize", "Smallest size", ["text", "minFontSize"], { min: 0 }),
            int("paddingX", "Padding across", ["text", "paddingX"], { min: 0 }),
            int("paddingY", "Padding down", ["text", "paddingY"], { min: 0 }),
            num("tracking", "Tracking", ["text", "tracking"]),
          ],
        },
        {
          title: "Shadow",
          shadow: true,
          fields: [
            num("shadowDistance", "Distance", ["text", "shadow", "distance"], { min: 0, step: 0.01, required: true }),
            num("shadowAngle", "Angle", ["text", "shadow", "angle"], { step: 5 }),
            num("shadowOpacity", "Opacity", ["text", "shadow", "opacity"], { min: 0, max: 1, step: 0.05 }),
            { id: "shadowColor", label: "Color", path: ["text", "shadow", "color"], type: "color" },
          ],
        },
      ];
    case "layer":
      return [
        { title: "Place", fields: [{ ...int("x", "X", ["at", "x"]), at: true }, { ...int("y", "Y", ["at", "y"]), at: true }] },
        {
          title: "Appearance",
          fields: [
            num("opacity", "Opacity", ["opacity"], { min: 0, max: 1, step: 0.05 }),
            { id: "mode", label: "Blend", path: ["mode"], type: "choice", options: choice(vocab.blendModes || []), empty: "normal" },
            { id: "condition", label: "Draws when", path: ["condition"], type: "condition" },
          ],
        },
        {
          title: "Image",
          fields: [{ id: "slot", label: "Reads", path: ["slot"], type: "choice", options: choice(vocab.slots || []), empty: "one image for every card", on: "part" }],
        },
      ];
    case "art":
      return [
        { title: "Place", fields: rect([]) },
        {
          title: "Window",
          fields: [
            { id: "after", label: "Above layer", path: ["after"], type: "choice", options: choice(ctx.stackIds || []), required: true },
            { id: "rotate", label: "Turned", path: ["rotate"], type: "choice", options: choice(TURNS), empty: "0" },
          ],
        },
      ];
    case "symbol":
      return [
        { title: "Place", fields: rect([]) },
        {
          title: "Symbol",
          fields: [
            { id: "align", label: "Align", path: ["align"], type: "choice", options: choice(ALIGN), empty: "right" },
            { id: "vAlign", label: "Vertical", path: ["vAlign"], type: "choice", options: choice(VALIGN), empty: "center" },
            num("scale", "Scale", ["scale"], { min: 0.05, max: 1, step: 0.05 }),
            { id: "condition", label: "Draws when", path: ["condition"], type: "condition" },
          ],
        },
      ];
    case "canvas":
      return [
        {
          title: "Size",
          // The width and the height can be kept in proportion, which the page draws
          ratio: ["width", "height"],
          fields: [
            int("width", "Width", ["width"], { min: 1, required: true }),
            int("height", "Height", ["height"], { min: 1, required: true }),
            int("dpi", "Resolution", ["dpi"], { min: 1, max: 10000 }),
          ],
        },
        {
          title: "Rotation",
          fields: [{ id: "rotate", label: "Turned", path: ["rotate"], type: "choice", options: choice(TURNS), empty: "0" }],
        },
      ];
    case "template":
      return [
        {
          title: "Template",
          fields: [
            { id: "name", label: "Name", path: ["name"], type: "text", required: true },
            { id: "description", label: "Description", path: ["description"], type: "text" },
            { id: "authorName", label: "Author", path: ["author", "name"], type: "text" },
            { id: "authorUrl", label: "Author link", path: ["author", "url"], type: "text" },
            { id: "license", label: "License", path: ["license"], type: "text", lockable: true },
          ],
        },
      ];
  }
  return [];
}

// readPath follows keys down a parsed file, and gives undefined where there is
// nothing
export function readPath(doc, path) {
  let at = doc;
  for (const key of path) {
    if (at === null || typeof at !== "object") return undefined;
    at = at[key];
  }
  return at;
}

// displayValue is the text a field shows for what the file holds
export function displayValue(field, value) {
  if (value === undefined || value === null) return "";
  return String(value);
}

// parseField reads what was typed into a field: { ok, unset, value, error }. An empty
// field takes the setting out, so the box goes back to its default, unless the field
// is required
export function parseField(field, text) {
  const t = String(text ?? "").trim();
  if (t === "") return field.required ? { ok: false, error: "This is needed" } : { ok: true, unset: true };
  switch (field.type) {
    case "int": {
      if (!/^-?\d+$/.test(t)) return { ok: false, error: "Use a whole number" };
      return inRange(field, Number(t));
    }
    case "number": {
      if (!/^-?(\d+\.?\d*|\.\d+)$/.test(t)) return { ok: false, error: "Use a number" };
      return inRange(field, Number(t));
    }
    case "color":
      return /^#[0-9A-Fa-f]{6}$/.test(t) ? { ok: true, value: t.toUpperCase() } : { ok: false, error: "Use a color like #112233" };
    case "choice": {
      const known = field.options.some((o) => o.value === t);
      if (!known) return { ok: false, error: "Pick one of the choices" };
      // A turn of 0 is no turn, so it is no setting
      return field.id === "rotate" && t === "0" ? { ok: true, unset: true } : { ok: true, value: field.id === "rotate" ? Number(t) : t };
    }
  }
  return { ok: true, value: t };
}

function inRange(field, n) {
  if (field.min !== undefined && n < field.min) return { ok: false, error: `Use ${field.min} or more` };
  if (field.max !== undefined && n > field.max) return { ok: false, error: `Use ${field.max} or less` };
  return { ok: true, value: n };
}

// editOps makes the operations for a field's new value. target is { layout, kind, key }
// for the item, parsed is what parseField gave, and ctx has the doc the item is in so
// a layer's position can be written whole, since its two numbers go together
export function editOps(target, field, parsed, ctx = {}) {
  const base = { layout: target.layout, kind: target.kind, key: target.key };
  if (field.at) {
    // A layer's position is both numbers or neither, and one it has not set is where
    // its part puts it
    const at = ctx.at || { x: 0, y: 0 };
    const next = { ...at, [field.path[1]]: parsed.value };
    return [{ op: "set", ...base, path: ["at"], value: next }];
  }
  if (field.on === "part") {
    const part = { layout: target.layout, kind: "part", key: ctx.part };
    return parsed.unset ? [{ op: "unset", ...part, path: field.path }] : [{ op: "set", ...part, path: field.path, value: parsed.value }];
  }
  return parsed.unset ? [{ op: "unset", ...base, path: field.path }] : [{ op: "set", ...base, path: field.path, value: parsed.value }];
}

// keepRatio is the canvas that has a width or a height changed to a value while the
// other follows it, so that width over height stays ratio. Neither goes under one pixel
export function keepRatio(id, value, ratio) {
  if (id === "width") return { width: value, height: Math.max(1, Math.round(value / ratio)) };
  return { width: Math.max(1, Math.round(value * ratio)), height: value };
}

// sizeOps are the edits that make a canvas the size next from the size now, for the
// width and the height that differ
export function sizeOps(target, next, now) {
  const base = { layout: target.layout, kind: target.kind, key: target.key };
  return ["width", "height"].filter((k) => next[k] !== now[k]).map((k) => ({ op: "set", ...base, path: [k], value: next[k] }));
}

// shadowOps turns a text box's shadow on, with the usual numbers, or off
export function shadowOps(target, on) {
  const base = { layout: target.layout, kind: target.kind, key: target.key, path: ["text", "shadow"] };
  return on ? [{ op: "set", ...base, value: { ...SHADOW_DEFAULT } }] : [{ op: "unset", ...base }];
}

// the kind of an item as the layout's own, which differs from the kind of a field's
// place for a canvas and the template
export function targetFor(item, layout) {
  return { layout, kind: item.kind, key: item.key };
}

// editLabel names an edit for the undo history
export function editLabel(target, field) {
  const what = target.kind === "template" ? "template" : target.kind === "canvas" ? "canvas" : target.key || target.kind;
  return `Change ${field.label.toLowerCase()} of ${what}`;
}

// A layer's position as the file has it, or where its part puts it
export function layerAt(entry, origin) {
  if (entry && entry.at && Number.isInteger(entry.at.x) && Number.isInteger(entry.at.y)) return { x: entry.at.x, y: entry.at.y };
  return { x: origin?.x || 0, y: origin?.y || 0 };
}

// credits as the form edits them: a list of { name, url, note }, with the first
// `locked` kept, since they came with the template that was copied
export function creditEdit(credits, index, key, value, locked) {
  if (index < locked) return null;
  return (credits || []).map((c, i) => (i === index ? withKey(c, key, value) : c));
}

function withKey(credit, key, value) {
  const next = { ...credit };
  if (value === "" && key !== "name") delete next[key];
  else next[key] = value;
  return next;
}

export function creditAdd(credits) {
  return [...(credits || []), { name: "New credit" }];
}

export function creditRemove(credits, index, locked) {
  if (index < locked || index >= (credits || []).length) return null;
  return credits.filter((_, i) => i !== index);
}

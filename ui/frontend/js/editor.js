// What the template editor shows about a draft, as pure functions of what the app
// sent. Nothing here touches the document, so the wording and the choices live in
// one place that the Node tests reach

// The resolutions a new template can be made at. The engine refuses anything
// outside them, and checking here says so before the dialog closes
export const DPI_MIN = 72;
export const DPI_MAX = 1200;
export const DPI_DEFAULT = 800;

// parseDpi reads the resolution field of the new template dialog. An empty field
// is the default, and anything else must be a whole number in range
export function parseDpi(text) {
  const t = String(text ?? "").trim();
  if (t === "") return { dpi: DPI_DEFAULT, error: "" };
  if (!/^\d+$/.test(t)) return { dpi: 0, error: "Use a whole number of dots per inch" };
  const dpi = Number(t);
  if (dpi < DPI_MIN || dpi > DPI_MAX) return { dpi: 0, error: `Use a resolution from ${DPI_MIN} to ${DPI_MAX} dpi` };
  return { dpi, error: "" };
}

const NAME_MAX = 80;

// forkName suggests a name for a copy of a template
export function forkName(title) {
  const base = String(title || "Template").trim() || "Template";
  const name = `${base} copy`;
  return name.length > NAME_MAX ? name.slice(0, NAME_MAX).trim() : name;
}

// checkName reads a template name from a dialog, and says what is wrong with it
export function checkName(text) {
  const name = String(text ?? "").trim();
  if (!name) return { name: "", error: "Give the template a name" };
  if ([...name].length > NAME_MAX) return { name: "", error: `A name is at most ${NAME_MAX} characters` };
  return { name, error: "" };
}

// draftLabel is a draft's line in the picker
export function draftLabel(d) {
  return d.problem ? `${d.name} (cannot be read)` : d.name;
}

// layoutLine describes a layout's canvas in a line
export function layoutLine(l) {
  const turned = l.rotate ? `, turned ${l.rotate}°` : "";
  return `${l.name} · ${l.width} × ${l.height} px · ${l.dpi} dpi${turned}`;
}

// problems splits a draft's diagnostics into the errors that stop it rendering and
// the warnings that do not, each as a line with the path to it
export function problems(diagnostics) {
  const line = (d) => (d.path ? `${d.path}: ${d.message}` : d.message);
  const out = { errors: [], warnings: [] };
  for (const d of diagnostics || []) (d.severity === "warning" ? out.warnings : out.errors).push(line(d));
  return out;
}

// cardLine is a card's name and set, for the line that says which card the draft
// is being tested with
export function cardLine(card) {
  if (!card) return "";
  return card.SetCode ? `${card.Name} · ${String(card.SetCode).toUpperCase()}` : card.Name;
}

// startingCard picks the card a draft opens with: the one it was last tested with, else
// the first preset, whose card is looked up and so is not here yet
export function startingCard(meta, presets) {
  if (meta && meta.testCard) return { card: meta.testCard, face: meta.face || 0, preset: "" };
  const first = (presets || [])[0];
  return first ? { card: null, face: 0, preset: first.id } : null;
}

// forkedFrom says where a copy began, or "" for one that began blank
export function forkedFrom(d) {
  return d && d.forkedFrom ? `Copy of ${d.forkedFrom}` : "";
}

// A card canvas with its bleed is this many inches across and tall, which turns a
// resolution into the pixel size of the template a new draft starts with
const CANVAS_INCHES = { width: 2.72, height: 3.7 };

// canvasSize is the size in pixels of a new template's canvas at dpi
export function canvasSize(dpi) {
  return { width: Math.round(CANVAS_INCHES.width * dpi), height: Math.round(CANVAS_INCHES.height * dpi) };
}

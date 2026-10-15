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

// --- finishing a template ---

// creditLine is one credit as the export dialog lists it
export function creditLine(c) {
  const name = String(c.name || "").trim() || "Unnamed credit";
  const note = String(c.note || "").trim();
  return note ? `${name}, ${note}` : name;
}

// bytesLabel is a file size in the units a person reads
export function bytesLabel(bytes) {
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(bytes >= 1e7 ? 0 : 1)} MB`;
  return `${Math.max(1, Math.round(bytes / 1e3))} KB`;
}

// useMessage says what Use in mimic did, for the toast
export function useMessage(res, title) {
  const made = res.replaced ? `${title} is up to date in Single, List and Run as ${res.template}` : `${title} is ready in Single, List and Run as ${res.template}`;
  return res.active ? `${made}. It is the template in use, so the next card shows the change.` : `${made}. Choose it in Templates to draw with it.`;
}

// exportMessage says where an export went, for the toast
export function exportMessage(res) {
  const name = String(res.path || "").split(/[\\/]/).pop();
  return `Saved ${name} (${bytesLabel(res.bytes)})`;
}

// releaseFacts are the lines the export dialog states before a bundle is written,
// leaving out what the template does not say
export function releaseFacts(r) {
  const facts = [
    ["Template", r.name],
    ["Id", r.templateId],
    ["Version", r.version],
    ["Author", r.author],
    ["License", r.license],
  ];
  return facts.filter(([, value]) => String(value || "").trim() !== "").map(([label, value]) => ({ label, value }));
}

// The longest name a saved test card can have, which the app also holds it to
export const TEST_CARD_LABEL_MAX = 40;

// checkTestCardLabel reads the name of a test card to save, and says what is wrong
export function checkTestCardLabel(text) {
  const label = String(text ?? "").trim().replace(/\s+/g, " ");
  if (!label) return { label: "", error: "Give the test card a name" };
  if ([...label].length > TEST_CARD_LABEL_MAX) return { label: "", error: `A name is at most ${TEST_CARD_LABEL_MAX} characters` };
  return { label, error: "" };
}

// suggestedLabel is the name a test card is offered under, from the card's own
export function suggestedLabel(card) {
  const name = String((card && card.Name) || "").trim();
  return [...name].slice(0, TEST_CARD_LABEL_MAX).join("").trim();
}

// coverageTile is one preset's place in the coverage strip. An outcome is
// { state: "waiting" | "drawing" | "drawn" | "failed", reason }
export function coverageTile(preset, outcome) {
  const o = outcome || { state: "waiting" };
  const text = { waiting: "Not drawn yet", drawing: "Drawing", drawn: "Drawn", failed: o.reason || "Could not be drawn" }[o.state];
  return { id: preset.id, label: preset.label, state: o.state, text };
}

// coverageSummary is the line above the strip, saying how many cards the template drew
// and which it could not
export function coverageSummary(tiles, stale) {
  const done = tiles.filter((t) => t.state === "drawn").length;
  const failed = tiles.filter((t) => t.state === "failed");
  if (tiles.every((t) => t.state === "waiting")) return "Draws every test card small, to see the frame each one gets";
  const said = `${done} of ${tiles.length} drawn`;
  const missing = failed.length ? `. Not drawn: ${failed.map((t) => t.label).join(", ")}` : "";
  return `${said}${missing}${stale ? ". The template has changed since" : ""}`;
}

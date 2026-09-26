// Whether the active template can render a card. The server classifies every
// card it sends into shapes, one per image it renders to, and reports the
// active template's supports, so this only compares the two

import { app } from "./state.js";

// describeShape names a face's shape the way a person would, for "normal does
// not render planeswalker cards" or "transform front faces"
function describeShape(sh) {
  if (sh.role === "unknown") return "cards with this layout";
  const kind = sh.kind === "standard" ? "" : `${sh.kind.replace(/_/g, " ")} `;
  if (sh.role === "single") return `${kind || "standard "}cards`;
  if (sh.role === "split" || sh.role === "adventure") return `${kind}${sh.role} cards`;
  const [layout, side] = sh.role.split("_");
  return `${kind}${layout === "mdfc" ? "MDFC" : layout} ${side} faces`;
}

// unsupportedBy says why the active template cannot render a card's front
// face, or "" when it can. A card sent without shapes is left to the engine's
// own check at render time. It peeks at the template, so an effect that should
// redraw on a template switch reads app.template itself
export function unsupportedBy(card) {
  const t = app.template.peek();
  const sh = card && card.shapes && card.shapes[0];
  if (!sh || !t || !t.supports || !t.supports.roles) return "";
  if (t.supports.roles.includes(sh.role) && t.supports.kinds.includes(sh.kind)) return "";
  return `${t.name || "This template"} does not render ${describeShape(sh)}`;
}

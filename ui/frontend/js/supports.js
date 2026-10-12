// Whether some installed template can render a card's faces. The app
// classifies every card it sends into shapes, one per image it renders to, and
// reports which template renders each shape, so this only compares the two

import { app } from "./state.js";

// noTemplateInstalled reports whether the app is running with nothing to render
// with, which the app calls the placeholder
export function noTemplateInstalled() {
  return app.template.peek()?.source === "placeholder";
}

// describeShape names a face's shape the way a person would, for "normal does
// not render planeswalker cards" or "transform front faces"
export function describeShape(sh) {
  if (sh.role === "unknown") return "cards with this layout";
  const kind = sh.kind === "standard" ? "" : `${sh.kind.replace(/_/g, " ")} `;
  if (sh.role === "single") return `${kind || "standard "}cards`;
  if (sh.role === "split" || sh.role === "adventure") return `${kind}${sh.role} cards`;
  const [layout, side] = sh.role.split("_");
  return `${kind}${layout === "mdfc" ? "MDFC" : layout} ${side} faces`;
}

const shapeKey = (sh) => `${sh.role}/${sh.kind}`;

// faceSupport reports how many of a card's faces some installed template
// renders, and the shapes of those none does. A card sent without shapes, or
// a template report without faces, is left to the engine's own check at
// render time. It peeks at the template, so an effect that should redraw on a
// template change reads app.template itself
export function faceSupport(card) {
  const shapes = (card && card.shapes) || [];
  const faces = app.template.peek()?.faces;
  if (!shapes.length || !faces) return { total: shapes.length || 1, renderable: shapes.length || 1, missing: [] };
  const missing = shapes.filter((sh) => !faces[shapeKey(sh)]);
  return { total: shapes.length, renderable: shapes.length - missing.length, missing };
}

// unsupportedBy says why none of a card's faces can render, or "" when at
// least one can
export function unsupportedBy(card) {
  const f = faceSupport(card);
  if (f.renderable > 0) return "";
  return noTemplateInstalled() ? "No template is installed" : `No installed template renders ${describeShape(f.missing[0])}`;
}

// partlyUnsupportedBy says which face of a card will be skipped when another
// renders, or "" when every face renders or none does
export function partlyUnsupportedBy(card) {
  const f = faceSupport(card);
  if (f.renderable === 0 || !f.missing.length) return "";
  return `Only ${f.renderable} of ${f.total} faces will render, since no installed template renders ${describeShape(f.missing[0])}`;
}

// faceUnsupportedBy says why one face of a card cannot render, or "" when it
// can
export function faceUnsupportedBy(card, face) {
  const sh = card && card.shapes && card.shapes[face];
  const faces = app.template.peek()?.faces;
  if (!sh || !faces || faces[shapeKey(sh)]) return "";
  return noTemplateInstalled() ? "No template is installed" : `No installed template renders ${describeShape(sh)}`;
}

// The card layouts a template can render, grouped the way a person thinks of
// them: the two faces of a transform card are one layout, and a shape of a kind
// other than standard is named by its kind
const FAMILIES = { single: "Standard", split: "Split", adventure: "Adventure", aftermath: "Aftermath", transform: "Transform", mdfc: "MDFC" };
const FAMILY_ORDER = Object.keys(FAMILIES);
const SIDED = new Set(["transform", "mdfc"]);

const titleCase = (s) => s.replace(/_/g, " ").replace(/^./, (c) => c.toUpperCase());

// layoutChips groups a template's shapes into layouts, each { label, shapes,
// title }, standard cards first. A layout with faces on two sides that a template
// renders only one of says which, as in "Transform (front only)"
export function layoutChips(shapes) {
  const groups = new Map();
  for (const sh of shapes || []) {
    const [family, side] = sh.role.split("_");
    const key = `${sh.kind}|${family}`;
    if (!groups.has(key)) groups.set(key, { kind: sh.kind, family, sides: new Set(), shapes: [] });
    const g = groups.get(key);
    g.shapes.push(sh);
    if (side) g.sides.add(side);
  }
  const rank = (g) => (g.kind === "standard" ? 0 : 1) * 100 + (FAMILY_ORDER.includes(g.family) ? FAMILY_ORDER.indexOf(g.family) : FAMILY_ORDER.length);
  return [...groups.values()]
    .sort((a, b) => rank(a) - rank(b) || a.kind.localeCompare(b.kind))
    .map((g) => {
      const family = FAMILIES[g.family] || titleCase(g.family);
      const standard = g.kind === "standard";
      const qualifiers = [];
      if (!standard && g.family !== "single") qualifiers.push(family.toLowerCase());
      if (SIDED.has(g.family) && g.sides.size === 1) qualifiers.push(`${[...g.sides][0]} only`);
      const label = (standard ? family : titleCase(g.kind)) + (qualifiers.length ? ` (${qualifiers.join(", ")})` : "");
      return { label, shapes: g.shapes, title: `Renders ${g.shapes.map(describeShape).join(" and ")}` };
    });
}

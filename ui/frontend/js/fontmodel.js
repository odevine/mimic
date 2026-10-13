// What the Fonts setting shows and keeps, as plain data in and out. Nothing here
// touches the document, so it is tested under frontend/test

// norm folds case and runs of space, the way the engine finds a font by name
const norm = (s) => String(s || "").toLowerCase().split(/\s+/).filter(Boolean).join(" ");

// poolFont is the font in the folder with this name, whatever its capitals
export function poolFont(pool, name) {
  const want = norm(name);
  return (pool || []).find((f) => norm(f.name) === want) || null;
}

// roleChoices lists what a role's picker offers: its own font first, then every
// font in the folder. A font the role is assigned that the folder lacks stays
// listed and marked, so opening the settings never silently drops the assignment
export function roleChoices(role, pool, picked) {
  const out = [{ value: "", label: `${role.own} (template default)` }];
  for (const f of pool || []) out.push({ value: f.name, label: f.name });
  const value = picked === undefined ? role.override || "" : picked;
  if (value && !poolFont(pool, value)) out.push({ value, label: `${value} (not in the folder)`, missing: true });
  return out;
}

// pickedValue is the picker value for a role's assignment: the folder's own
// spelling of the font when it has it, so the right option is selected
export function pickedValue(role, pool, picked) {
  const value = picked === undefined ? role.override || "" : picked;
  if (!value) return "";
  return poolFont(pool, value)?.name || value;
}

// mergeOverrides is the assignments to save: the saved ones, changed by what each
// picker shows. A role with no picker keeps what it had, and an empty pick clears
// the role. picks is null when the pickers never drew, which leaves everything
export function mergeOverrides(saved, picks) {
  const out = { ...(saved || {}) };
  if (!picks) return out;
  for (const [role, value] of Object.entries(picks)) {
    if (value) out[role] = value;
    else delete out[role];
  }
  return out;
}

// pickedState says what a role draws with when its picker shows value, and
// whether that deserves attention. It reads the folder and not the saved state,
// so it stays right as a choice is made before the settings are saved
export function pickedState(role, pool, value) {
  if (role.source === "fallback") return { text: "Built-in fallback", tone: "warn" };
  if (!value) return { text: role.bundled ? "Shipped with the template" : "Built in", tone: "" };
  const font = poolFont(pool, value);
  if (font) return { text: `Your font, ${font.file}`, tone: "ok" };
  return { text: `${value} is not in the folder, so the template's font is used`, tone: "warn" };
}

// ownFont is how a role's own font reads beside its picker
export function ownFont(role) {
  return role.own || [role.family, role.style].filter(Boolean).join(" ") || "Unknown font";
}

// folderNote says where fonts come from and whether they can be changed here
export function folderNote(kind, path) {
  switch (kind) {
    case "managed":
      return "Fonts you add here are kept in the app's fonts folder, and any role can use any of them.";
    case "checkout":
      return `Using ${path} from the checkout, so fonts are changed there rather than here.`;
    case "flag":
      return `Using ${path} from the -fonts flag, so fonts are changed there rather than here.`;
    default:
      return "There is no config folder to keep fonts in, so every role uses its own font.";
  }
}

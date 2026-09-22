import { icon, h } from "./dom.js";
import { showTooltipNow } from "./components/tooltip.js";
import { app } from "./state.js";

// Applies the server's gate map to the DOM. Three attributes carry a feature
// key: data-feature gates a single control, data-feature-section overlays a
// whole region, and data-feature-indicator marks a control with a lock while
// leaving it usable, which is how a gated mode stays openable from the rail

const GATE_TEXT = {
  planned: "Planned",
  "needs-engine": "Needs engine",
  "needs-template": "Needs template",
};

// A key the server does not list is treated as not built, so a typo in markup
// shows up as a gate rather than as a live control that does nothing
const MISSING = { state: "planned", reason: "Not available in this build" };

export function gateFor(key) {
  return app.capabilities.peek()[key] || MISSING;
}

export function isLive(key) {
  return gateFor(key).state === "live";
}

let reasonSeq = 0;

// describe wires the reason into the accessibility tree so a screen reader
// learns the same thing a hovering user does
function describe(el, g) {
  let id = el.dataset.gateDescId;
  if (!id) {
    id = `gate-reason-${++reasonSeq}`;
    el.dataset.gateDescId = id;
    document.body.append(h("span", { id, class: "sr-only" }));
  }
  document.getElementById(id).textContent = `${GATE_TEXT[g.state] || ""}. ${g.reason || ""}`;
  el.setAttribute("aria-describedby", id);
}

function glyphFor(g) {
  if (g.state === "needs-template") return h("span", { class: "gate-badge gate-mark" }, "template");
  const svg = icon("lock", "icon gate-glyph gate-mark");
  return svg;
}

function clearMarks(el) {
  el.querySelectorAll(":scope > .gate-mark").forEach((m) => m.remove());
}

function applyControl(el, g) {
  clearMarks(el);
  if (g.state === "live") {
    el.classList.remove("gated");
    el.removeAttribute("aria-disabled");
    el.removeAttribute("aria-describedby");
    delete el.dataset.gateState;
    delete el.dataset.gateReason;
    if (el.matches("input, select, textarea")) el.removeAttribute("readonly");
    return;
  }
  el.classList.add("gated");
  el.setAttribute("aria-disabled", "true");
  el.dataset.gateState = g.state;
  el.dataset.gateReason = g.reason || "";
  describe(el, g);
  if (el.matches("input, select, textarea")) {
    // A form control keeps focusability but refuses edits
    el.setAttribute("readonly", "");
  } else {
    el.append(glyphFor(g));
  }
}

function applyIndicator(el, g) {
  clearMarks(el);
  if (g.state === "live") {
    delete el.dataset.gateState;
    delete el.dataset.gateReason;
    return;
  }
  el.dataset.gateState = g.state;
  el.dataset.gateReason = g.reason || "";
  el.append(glyphFor(g));
}

function applySection(el, g) {
  const content = el.querySelector(":scope > .gated-content");
  el.querySelectorAll(":scope > .gate-pill").forEach((p) => p.remove());
  if (g.state === "live") {
    el.classList.remove("gated-section");
    if (content) content.inert = false;
    return;
  }
  el.classList.add("gated-section");
  if (content) content.inert = true;
  el.append(
    h(
      "div",
      { class: "gate-pill", role: "note" },
      icon("lock", "icon gate-glyph"),
      h("span", {}, g.reason || "Not available yet"),
      h("span", { class: "level" }, GATE_TEXT[g.state] || ""),
    ),
  );
}

// applyCapabilities walks root and applies the current gate map. It runs once
// at load and again whenever the map changes, such as after a template switch
export function applyCapabilities(root = document) {
  for (const el of root.querySelectorAll("[data-feature]")) applyControl(el, gateFor(el.dataset.feature));
  for (const el of root.querySelectorAll("[data-feature-indicator]")) applyIndicator(el, gateFor(el.dataset.featureIndicator));
  for (const el of root.querySelectorAll("[data-feature-section]")) applySection(el, gateFor(el.dataset.featureSection));
}

// A gated control swallows activation and shows its reason instead, so a click
// never does nothing silently
export function initGateGuard() {
  const guard = (e) => {
    const el = e.target.closest?.("[data-feature].gated");
    if (!el) return;
    if (e.type === "keydown") {
      if (e.key === "Tab") return;
      // A select changes value on arrow keys, so it refuses every key
      if (el.tagName !== "SELECT" && e.key !== "Enter" && e.key !== " ") return;
    }
    e.preventDefault();
    e.stopImmediatePropagation();
    showTooltipNow(el);
  };
  // A select opens on mousedown, before any click, so that is guarded too
  document.addEventListener("mousedown", (e) => {
    if (e.target.closest?.("select[data-feature].gated")) guard(e);
  }, true);
  document.addEventListener("click", guard, true);
  document.addEventListener("keydown", guard, true);
}

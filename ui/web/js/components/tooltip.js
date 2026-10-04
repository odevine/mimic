import { $ } from "../dom.js";

// One shared tooltip, shown on hover or keyboard focus of anything carrying
// data-tip or a gate reason. A gated control's tip names its gate level above
// the one-sentence reason. It is a manual popover so it shows in the top layer,
// above a modal dialog, which nothing outside that layer can cover

const LEVELS = {
  planned: "Planned",
  "needs-engine": "Needs engine",
  "needs-template": "Needs template",
};

const DELAY = 350;
let timer = 0;
let current = null;

function tipFor(el) {
  const reason = el.dataset.gateReason;
  if (reason) return { level: LEVELS[el.dataset.gateState] || "", text: reason };
  if (el.dataset.tip) return { level: "", text: el.dataset.tip };
  return null;
}

function show(el) {
  const tip = tipFor(el);
  if (!tip) return;
  const box = $("tooltip");
  box.replaceChildren();
  if (tip.level) {
    const lv = document.createElement("span");
    lv.className = "tip-level";
    lv.textContent = tip.level;
    box.append(lv);
  }
  box.append(tip.text);
  // Reopening moves it to the top of the top layer, above a dialog opened since
  if (box.matches(":popover-open")) box.hidePopover();
  box.showPopover();

  const r = el.getBoundingClientRect();
  const b = box.getBoundingClientRect();
  let top = r.bottom + 6;
  if (top + b.height > window.innerHeight - 4) top = r.top - b.height - 6;
  let left = r.left + r.width / 2 - b.width / 2;
  left = Math.max(6, Math.min(left, window.innerWidth - b.width - 6));
  box.style.top = `${top}px`;
  box.style.left = `${left}px`;
  current = el;
}

export function hideTooltip() {
  clearTimeout(timer);
  const box = $("tooltip");
  if (box.matches(":popover-open")) box.hidePopover();
  current = null;
}

// showTooltipNow shows el's tip immediately, used when a gated control is
// clicked so the click never silently does nothing
export function showTooltipNow(el) {
  clearTimeout(timer);
  show(el);
}

const target = (e) => e.target.closest?.("[data-tip], [data-gate-reason]");

export function initTooltips() {
  document.addEventListener("mouseover", (e) => {
    const el = target(e);
    if (el === current) return;
    hideTooltip();
    if (el) timer = setTimeout(() => show(el), DELAY);
  });
  document.addEventListener("focusin", (e) => {
    const el = target(e);
    hideTooltip();
    if (el && el.matches(":focus-visible")) show(el);
  });
  document.addEventListener("focusout", hideTooltip);
  document.addEventListener("mousedown", (e) => {
    if (!target(e)) hideTooltip();
  });
  window.addEventListener("scroll", hideTooltip, true);
}

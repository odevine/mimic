import { h } from "../dom.js";

// A single anchored popover at a time, for dropdowns and pickers. It closes on
// an outside click, Escape, or a resize, and moves focus through its menu items
// with the arrow keys

let active = null;

export function closePopover() {
  if (!active) return;
  const { el, anchor, onClose, cleanup } = active;
  active = null;
  cleanup();
  el.remove();
  anchor.setAttribute("aria-expanded", "false");
  if (onClose) onClose();
}

export function popoverOpen() {
  return active !== null;
}

function place(el, anchor, align) {
  const r = anchor.getBoundingClientRect();
  const b = el.getBoundingClientRect();
  let top = r.bottom + 4;
  if (top + b.height > window.innerHeight - 8 && r.top - b.height - 4 > 8) top = r.top - b.height - 4;
  let left = align === "end" ? r.right - b.width : r.left;
  left = Math.max(8, Math.min(left, window.innerWidth - b.width - 8));
  el.style.top = `${Math.max(8, top)}px`;
  el.style.left = `${left}px`;
}

// openPopover shows content anchored to anchor. Opening the same anchor again
// toggles it closed. It returns the popover element so a caller can refresh its
// contents in place
export function openPopover(anchor, content, { onClose, align = "start", cls = "", focus = true } = {}) {
  if (active && active.anchor === anchor) {
    closePopover();
    return null;
  }
  closePopover();

  const el = h("div", { class: `popover ${cls}`, role: "dialog" }, content);
  document.body.append(el);
  place(el, anchor, align);
  anchor.setAttribute("aria-expanded", "true");

  const onDown = (e) => {
    if (!el.contains(e.target) && !anchor.contains(e.target)) closePopover();
  };
  const onKey = (e) => {
    if (e.key === "Escape") {
      e.stopPropagation();
      closePopover();
      anchor.focus();
      return;
    }
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    const items = [...el.querySelectorAll(".menu-item:not([disabled])")];
    if (!items.length) return;
    e.preventDefault();
    const i = items.indexOf(document.activeElement);
    const next = e.key === "ArrowDown" ? (i + 1) % items.length : (i - 1 + items.length) % items.length;
    items[next].focus();
  };
  const onResize = () => closePopover();

  document.addEventListener("mousedown", onDown, true);
  document.addEventListener("keydown", onKey, true);
  window.addEventListener("resize", onResize);

  active = {
    el,
    anchor,
    onClose,
    cleanup() {
      document.removeEventListener("mousedown", onDown, true);
      document.removeEventListener("keydown", onKey, true);
      window.removeEventListener("resize", onResize);
    },
  };

  if (focus) {
    const first = el.querySelector("[aria-checked='true'], .menu-item:not([disabled]), button:not([disabled])");
    if (first) first.focus();
  }
  return el;
}

// repositionPopover re-anchors the open popover after its content changes size
export function repositionPopover(align = "start") {
  if (active) place(active.el, active.anchor, align);
}

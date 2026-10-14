import { DEFAULTS, MIN_HEIGHT, SECTIONS, nextOpen, pack, resizePair, restFrom, restore } from "../sectionlayout.js";

// The sections of a column that can each be collapsed to their heading and given the
// height their reader prefers. A splitter below an open section drags the line between
// it and the next open one. The state is handed back through onChange to be kept

const STEP = 16;

// initSections wires the column in root, whose sections are the elements with a
// data-sec in the order of SECTIONS. saved is the list pack made, if there is one
export function initSections(root, saved, onChange) {
  const state = restore(saved);
  const secs = SECTIONS.map((id) => root.querySelector(`[data-sec="${id}"]`));
  const splits = [...root.querySelectorAll(".ed-vsplit")];
  const toggles = secs.map((s) => s.querySelector(".ed-sec-toggle"));
  const bodies = secs.map((s) => s.querySelector(".ed-sec-body"));

  const names = secs.map((s) => s.querySelector(".ed-sec-name").textContent.trim());

  const heights = () => secs.map((sec, i) => (state.collapsed[i] ? 0 : sec.getBoundingClientRect().height));

  // report keeps each line's separator values at the height of the section above it,
  // and the range it can be dragged through
  const report = () => {
    const h = heights();
    splits.forEach((split, i) => {
      const j = nextOpen(state.collapsed, i);
      if (j < 0) return;
      split.setAttribute("aria-valuemin", String(MIN_HEIGHT));
      split.setAttribute("aria-valuemax", String(Math.max(MIN_HEIGHT, Math.round(h[i] + h[j] - MIN_HEIGHT))));
      split.setAttribute("aria-valuenow", String(Math.round(h[i])));
    });
  };

  const apply = () => {
    const rest = restFrom(state.collapsed);
    secs.forEach((sec, i) => {
      const collapsed = state.collapsed[i];
      sec.classList.toggle("collapsed", collapsed);
      sec.classList.toggle("rest", i === rest);
      toggles[i].setAttribute("aria-expanded", String(!collapsed));
      bodies[i].hidden = collapsed;
      sec.style.flexGrow = collapsed ? "0" : String(state.weights[i]);
    });
    splits.forEach((split, i) => {
      const j = nextOpen(state.collapsed, i);
      split.hidden = j < 0;
      if (j >= 0) split.setAttribute("aria-label", `Resize ${names[i]} and ${names[j]}`);
    });
    report();
  };
  apply();
  // The column is hidden while the page boots and its heights change with the window,
  // so the values are read again when it is resized
  new ResizeObserver(report).observe(root);

  // toggle collapses an open section and opens a collapsed one. An opened one is given
  // a share of the room in step with what the others have now
  const toggle = (i, keep = true) => {
    if (state.collapsed[i]) {
      const others = state.collapsed.map((c, k) => (!c && k !== i ? k : -1)).filter((k) => k >= 0);
      if (others.length) {
        const have = others.reduce((sum, k) => sum + state.weights[k], 0);
        const was = others.reduce((sum, k) => sum + DEFAULTS.weights[k], 0);
        state.weights[i] = Math.max(1, (DEFAULTS.weights[i] * have) / was);
      }
    }
    state.collapsed[i] = !state.collapsed[i];
    apply();
    if (keep) onChange(pack(state));
  };

  // settle makes the heights the sections have now their weights, so the shares are
  // kept when the window changes, and keeps them
  const settle = (h) => {
    h.forEach((px, k) => {
      if (!state.collapsed[k] && px > 0) state.weights[k] = px;
    });
    apply();
    onChange(pack(state));
  };

  toggles.forEach((btn, i) => btn.addEventListener("click", () => toggle(i)));

  splits.forEach((split, i) => {
    split.tabIndex = 0;
    const drag = (h0, dy) => {
      const j = nextOpen(state.collapsed, i);
      if (j < 0) return null;
      const { hi, hj } = resizePair(h0, i, j, dy, MIN_HEIGHT);
      const next = [...h0];
      next[i] = hi;
      next[j] = hj;
      return next;
    };

    split.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      split.setPointerCapture(e.pointerId);
      const h0 = heights();
      const startY = e.clientY;
      split.classList.add("dragging");
      document.body.classList.add("resizing-rows");
      let last = null;
      const move = (ev) => {
        last = drag(h0, ev.clientY - startY);
        if (!last) return;
        last.forEach((px, k) => {
          if (!state.collapsed[k]) secs[k].style.flexGrow = String(px);
        });
      };
      const up = () => {
        split.removeEventListener("pointermove", move);
        split.removeEventListener("pointerup", up);
        split.removeEventListener("pointercancel", up);
        split.classList.remove("dragging");
        document.body.classList.remove("resizing-rows");
        if (last) settle(last);
      };
      split.addEventListener("pointermove", move);
      split.addEventListener("pointerup", up);
      split.addEventListener("pointercancel", up);
    });

    split.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
      e.preventDefault();
      const next = drag(heights(), e.key === "ArrowDown" ? STEP : -STEP);
      if (next) settle(next);
    });
  });

  return {
    isOpen: (id) => !state.collapsed[SECTIONS.indexOf(id)],
    // open shows a section without keeping that as a choice, for one the page opens
    // itself, such as the problems of a template that does not load
    open(id) {
      const i = SECTIONS.indexOf(id);
      if (i >= 0 && state.collapsed[i]) toggle(i, false);
    },
  };
}

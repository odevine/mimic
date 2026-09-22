// Drag-resizable region splits. Each splitter resizes whichever neighbor is a
// fixed-width region, so the grow region absorbs the change. Widths are handed
// back through onChange for persisting per flow

const MIN = 200;
const STEP = 16;

function fixedNeighbor(splitter) {
  const prev = splitter.previousElementSibling;
  const next = splitter.nextElementSibling;
  if (prev && prev.classList.contains("fixed")) return { region: prev, sign: 1 };
  return { region: next, sign: -1 };
}

function clampWidth(regions, w) {
  const max = Math.max(MIN, regions.clientWidth * 0.55);
  return Math.round(Math.min(Math.max(w, MIN), max));
}

// initSplitters wires every splitter in regions. widths is the persisted list
// of fixed-region widths in document order, and onChange receives the new list
export function initSplitters(regions, widths, onChange) {
  const fixed = [...regions.querySelectorAll(":scope > .region.fixed")];
  (widths || []).forEach((w, i) => {
    if (fixed[i] && w > 0) fixed[i].style.width = `${clampWidth(regions, w)}px`;
  });
  const report = () => onChange(fixed.map((r) => r.getBoundingClientRect().width));

  for (const s of regions.querySelectorAll(":scope > .splitter")) {
    const { region, sign } = fixedNeighbor(s);
    s.tabIndex = 0;

    s.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      s.setPointerCapture(e.pointerId);
      const startX = e.clientX;
      const startW = region.getBoundingClientRect().width;
      s.classList.add("dragging");
      document.body.classList.add("resizing");

      const move = (ev) => {
        region.style.width = `${clampWidth(regions, startW + sign * (ev.clientX - startX))}px`;
      };
      const up = () => {
        s.removeEventListener("pointermove", move);
        s.removeEventListener("pointerup", up);
        s.classList.remove("dragging");
        document.body.classList.remove("resizing");
        report();
      };
      s.addEventListener("pointermove", move);
      s.addEventListener("pointerup", up);
    });

    s.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
      e.preventDefault();
      const d = (e.key === "ArrowRight" ? STEP : -STEP) * sign;
      region.style.width = `${clampWidth(regions, region.getBoundingClientRect().width + d)}px`;
      report();
    });
  }
}

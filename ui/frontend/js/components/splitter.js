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

// regionMin is the narrowest a fixed region may go: MIN, or its CSS min-width
// when that is larger, such as a preview whose toolbar needs the room
function regionMin(region) {
  return Math.max(MIN, parseFloat(getComputedStyle(region).minWidth) || 0);
}

// clampWidth keeps a fixed region between its minimum and the room the rest of
// the row leaves it: the other fixed regions, the splitters, and the grow
// region's own minimum, read from its --grow-min. So a drag never pushes the
// row past the window, and the regions never scroll sideways as one strip
function widthRange(regions, region) {
  let others = 0;
  let growMin = MIN;
  for (const el of regions.children) {
    if (el === region) continue;
    if (el.classList.contains("grow")) {
      growMin = parseFloat(getComputedStyle(el).getPropertyValue("--grow-min")) || MIN;
    } else {
      others += el.getBoundingClientRect().width;
    }
  }
  const min = regionMin(region);
  const room = regions.clientWidth - others - growMin;
  return { min, max: Math.max(min, Math.min(regions.clientWidth * 0.55, room)) };
}

function clampWidth(regions, region, w) {
  const { min, max } = widthRange(regions, region);
  return Math.round(Math.min(Math.max(w, min), max));
}

// initSplitters wires every splitter in regions. widths is the persisted list
// of fixed-region widths in document order, and onChange receives the new list
export function initSplitters(regions, widths, onChange) {
  const fixed = [...regions.querySelectorAll(":scope > .region.fixed")];
  // preferred holds the width each region was last set to, so a window that
  // narrows and widens again gives the regions back the room they had
  const preferred = fixed.map((r, i) => ((widths || [])[i] > 0 ? widths[i] : r.getBoundingClientRect().width));

  // fit sets every fixed region to its preferred width, then clamps each to
  // what the row has room for now
  const fit = () => {
    fixed.forEach((r, i) => (r.style.width = `${Math.max(preferred[i], regionMin(r))}px`));
    fixed.forEach((r) => (r.style.width = `${clampWidth(regions, r, r.getBoundingClientRect().width)}px`));
  };
  // report keeps each splitter's separator values at its region's width and range
  const splitters = [...regions.querySelectorAll(":scope > .splitter")];
  const report = () => {
    for (const s of splitters) {
      const { region } = fixedNeighbor(s);
      const { min, max } = widthRange(regions, region);
      s.setAttribute("aria-valuemin", Math.round(min));
      s.setAttribute("aria-valuemax", Math.round(max));
      s.setAttribute("aria-valuenow", Math.round(region.getBoundingClientRect().width));
    }
  };
  const refit = () => {
    fit();
    report();
  };
  refit();
  // Watching the row rather than the window fits after layout has settled,
  // including a switch between the stacked and side by side layouts
  new ResizeObserver(refit).observe(regions);

  // settle records a region's new width as its preference and persists them all
  const settle = (region) => {
    preferred[fixed.indexOf(region)] = region.getBoundingClientRect().width;
    onChange(preferred.map(Math.round));
    report();
  };

  for (const s of splitters) {
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
        region.style.width = `${clampWidth(regions, region, startW + sign * (ev.clientX - startX))}px`;
        report();
      };
      const up = () => {
        s.removeEventListener("pointermove", move);
        s.removeEventListener("pointerup", up);
        s.classList.remove("dragging");
        document.body.classList.remove("resizing");
        settle(region);
      };
      s.addEventListener("pointermove", move);
      s.addEventListener("pointerup", up);
    });

    s.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
      e.preventDefault();
      const d = (e.key === "ArrowRight" ? STEP : -STEP) * sign;
      region.style.width = `${clampWidth(regions, region, region.getBoundingClientRect().width + d)}px`;
      settle(region);
    });
  }
}

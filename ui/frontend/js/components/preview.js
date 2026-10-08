import { $ } from "../dom.js";

// The preview stage: fit, 100% and 200% stops plus steps between, and drag to
// pan once zoomed. 100% means one image pixel per device pixel, since the
// failures worth zooming for are sub-pixel

const STEPS = [0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4];

let zoom = 0; // 0 is fit
let panning = null;

function label() {
  $("zoom-label").textContent = zoom === 0 ? "Fit" : `${Math.round(zoom * 100)}%`;
}

function apply(anchor) {
  const stage = $("stage");
  const img = $("preview-img");
  const fit = zoom === 0;
  stage.classList.toggle("fit", fit);
  stage.classList.toggle("zoomed", !fit);

  if (fit || !img.naturalWidth) {
    img.style.width = "";
    img.style.height = "";
    label();
    return;
  }

  // Keep the image point under the anchor, or the stage center, still
  const rect = stage.getBoundingClientRect();
  const p = anchor || { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
  const before = img.getBoundingClientRect();
  const fx = before.width ? (p.x - before.left) / before.width : 0.5;
  const fy = before.height ? (p.y - before.top) / before.height : 0.5;

  img.style.width = `${(img.naturalWidth * zoom) / window.devicePixelRatio}px`;
  img.style.height = "auto";

  const after = img.getBoundingClientRect();
  stage.scrollLeft += after.left + fx * after.width - p.x;
  stage.scrollTop += after.top + fy * after.height - p.y;
  label();
}

// fitScale is the zoom fit mode is showing, so stepping in or out from fit
// starts from what is on screen
function fitScale() {
  const img = $("preview-img");
  if (!img.naturalWidth) return 1;
  return (img.getBoundingClientRect().width * window.devicePixelRatio) / img.naturalWidth;
}

export function setZoom(z, anchor) {
  zoom = z;
  apply(anchor);
}

export function zoomStep(dir, anchor) {
  const from = zoom === 0 ? fitScale() : zoom;
  const next = dir > 0 ? STEPS.find((s) => s > from + 0.001) : [...STEPS].reverse().find((s) => s < from - 0.001);
  if (next) setZoom(next, anchor);
}

// cycleZoom walks fit, 100%, 200%
export function cycleZoom() {
  setZoom(zoom === 0 ? 1 : zoom === 1 ? 2 : 0);
}

export function initPreview() {
  const stage = $("stage");
  $("zoom-in").addEventListener("click", () => zoomStep(1));
  $("zoom-out").addEventListener("click", () => zoomStep(-1));
  $("zoom-fit").addEventListener("click", () => setZoom(0));
  $("zoom-label").addEventListener("click", cycleZoom);
  $("preview-img").addEventListener("load", () => apply());

  stage.addEventListener(
    "wheel",
    (e) => {
      if (!(e.ctrlKey || e.metaKey) || $("preview-img").hidden) return;
      e.preventDefault();
      zoomStep(e.deltaY < 0 ? 1 : -1, { x: e.clientX, y: e.clientY });
    },
    { passive: false },
  );

  stage.addEventListener("pointerdown", (e) => {
    if (zoom === 0 || e.button !== 0) return;
    panning = { x: e.clientX, y: e.clientY, l: stage.scrollLeft, t: stage.scrollTop };
    stage.setPointerCapture(e.pointerId);
    stage.classList.add("panning");
  });
  stage.addEventListener("pointermove", (e) => {
    if (!panning) return;
    stage.scrollLeft = panning.l - (e.clientX - panning.x);
    stage.scrollTop = panning.t - (e.clientY - panning.y);
  });
  const end = () => {
    panning = null;
    stage.classList.remove("panning");
  };
  stage.addEventListener("pointerup", end);
  stage.addEventListener("pointercancel", end);
  label();
}

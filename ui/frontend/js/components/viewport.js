import { anchorShift, cycleZoom, stepZoom, wheelZoom, zoomLabel } from "../viewportmath.js";

// A viewport over a picture on a stage: zoom with steps and the wheel, and a pan that
// moves the picture about the stage by any distance, since the stage does not scroll. The
// picture is sized for its zoom, and moved by translating the element that holds it, so
// anything drawn over the picture inside that element goes with it

// createViewport wires a stage. canvas is the element that holds the picture and what is
// drawn over it, img is the picture, and label shows the zoom. isPan says whether a drag
// pans, which the middle button does always, and panning is told when a pan starts and
// ends
export function createViewport({ stage, canvas, img, label, isPan, panning = () => {} }) {
  let zoom = 0; // 0 is fit
  let offset = { x: 0, y: 0 };
  let drag = null;

  const place = () => {
    canvas.style.transform = offset.x || offset.y ? `translate(${offset.x}px, ${offset.y}px)` : "";
  };
  const show = () => {
    label.textContent = zoomLabel(zoom);
  };
  // fitScale is the zoom fit is showing, so stepping from it starts from what is on screen
  const fitScale = () => (img.naturalWidth ? (img.getBoundingClientRect().width * window.devicePixelRatio) / img.naturalWidth : 1);
  const size = () => {
    img.style.width = `${(img.naturalWidth * zoom) / window.devicePixelRatio}px`;
    img.style.height = "auto";
  };

  // setZoom shows the picture at a zoom, keeping the point of it under the anchor, or the
  // middle of the stage, where it was. Fit puts the picture back in the middle
  function setZoom(z, anchor) {
    zoom = z;
    stage.classList.toggle("fit", z === 0);
    if (z === 0) {
      img.style.width = "";
      img.style.height = "";
      offset = { x: 0, y: 0 };
      place();
      show();
      return;
    }
    if (!img.naturalWidth) return show();
    const rect = stage.getBoundingClientRect();
    const at = anchor || { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
    const before = img.getBoundingClientRect();
    size();
    const shift = anchorShift(before, img.getBoundingClientRect(), at);
    offset = { x: offset.x + shift.x, y: offset.y + shift.y };
    place();
    show();
  }

  const current = () => (zoom === 0 ? fitScale() : zoom);

  stage.addEventListener(
    "wheel",
    (e) => {
      // A sideways turn is left to the page, and the picture has to be there to zoom
      if (canvas.hidden || (Math.abs(e.deltaX) > Math.abs(e.deltaY) && !e.ctrlKey)) return;
      e.preventDefault();
      setZoom(wheelZoom(current(), e.deltaY, { mode: e.deltaMode, pinch: e.ctrlKey }), { x: e.clientX, y: e.clientY });
    },
    { passive: false },
  );

  stage.addEventListener("pointerdown", (e) => {
    const middle = e.button === 1;
    if (canvas.hidden || (!middle && !(e.button === 0 && isPan()))) return;
    e.preventDefault();
    drag = { x: e.clientX, y: e.clientY, ox: offset.x, oy: offset.y };
    stage.setPointerCapture(e.pointerId);
    stage.classList.add("panning");
    panning(true);
  });
  stage.addEventListener("pointermove", (e) => {
    if (!drag) return;
    offset = { x: drag.ox + e.clientX - drag.x, y: drag.oy + e.clientY - drag.y };
    place();
  });
  const end = () => {
    if (!drag) return;
    drag = null;
    stage.classList.remove("panning");
    panning(false);
  };
  stage.addEventListener("pointerup", end);
  stage.addEventListener("pointercancel", end);

  // A new picture of the same card keeps the zoom
  img.addEventListener("load", () => {
    if (zoom !== 0) size();
  });
  show();

  return {
    setZoom,
    step(dir) {
      const next = stepZoom(current(), dir);
      if (next) setZoom(next);
    },
    cycle: () => setZoom(cycleZoom(zoom)),
    reset: () => setZoom(0),
    get zoom() {
      return zoom;
    },
  };
}

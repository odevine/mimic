import { h } from "../dom.js";

// syncChips keeps a row of filter chips in step with specs, reusing each chip's
// element by value. Rebuilding them on every update would drop a click that
// lands between the press and the release while rows are streaming in

export function syncChips(container, specs, current, onPick) {
  const existing = new Map([...container.children].map((el) => [el.dataset.value, el]));
  const next = specs.map(({ label, value, count }) => {
    let el = existing.get(value);
    if (!el) {
      el = h(
        "button",
        { class: "chip", type: "button", dataset: { value }, onClick: () => onPick(value) },
        h("span", { class: "chip-label" }),
        h("span", { class: "count" }),
      );
    }
    el.querySelector(".chip-label").textContent = label;
    el.querySelector(".count").textContent = String(count);
    el.setAttribute("aria-pressed", String(current === value));
    return el;
  });
  const same = next.length === container.children.length && next.every((el, i) => container.children[i] === el);
  if (!same) container.replaceChildren(...next);
}

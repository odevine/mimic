import { $, h } from "../dom.js";
import { openPopover, closePopover } from "../components/popover.js";
import { FIELD_LABELS } from "../components/fieldLabels.js";
import { toast } from "../components/toast.js";

// Bulk actions on the checked rows of a list: set a field, set a quantity,
// remove rows, and tick or untick by what is shown, by section and by whether a
// row needs attention. Setting a field writes a field override on each checked
// row, the same as editing it in the inspector, and a removal can be undone until
// the next resolve. The list supplies what the actions work on and does the
// changing, so the rows keep a single owner

let host = null;

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

function menuItem(label, onClick, { disabled = false, tip = "" } = {}) {
  const b = h("button", { class: "menu-item", type: "button", disabled, onClick }, h("span", { class: "truncate" }, label));
  if (tip) b.dataset.tip = tip;
  return b;
}

function mainMenu(body) {
  const checked = host.checked();
  const n = checked.length;
  const none = n === 0 ? "No rows are checked" : "";
  const groups = host.groups();
  body.replaceChildren(
    h("div", { class: "menu-heading section-heading" }, n ? `${plural(n, "checked row", "checked rows")}` : "No checked rows"),
    menuItem("Set a field…", () => setField(body), { disabled: !n, tip: none }),
    menuItem("Set quantity…", () => setQty(body), { disabled: !n, tip: none }),
    menuItem(`Remove ${n ? plural(n, "checked row", "checked rows") : "checked rows"}`, () => remove(checked), { disabled: !n, tip: none }),
    h("div", { class: "menu-sep" }),
    h("div", { class: "menu-heading section-heading" }, "Tick or untick"),
    menuItem(`Tick the ${plural(host.shown().length, "row", "rows")} shown`, () => tick(host.shown(), true), { disabled: !host.shown().length }),
    menuItem("Untick the rows shown", () => tick(host.shown(), false), { disabled: !host.shown().length }),
    menuItem("Untick rows that need attention", () => tick(host.rows().filter((r) => host.needsAttention(r)), false), {
      disabled: !host.rows().some((r) => host.needsAttention(r)),
    }),
    ...groups.map((g) =>
      h(
        "div",
        { class: "bulk-section" },
        menuItem(`Tick ${g}`, () => tick(host.rows().filter((r) => r.group === g), true)),
        menuItem("Untick", () => tick(host.rows().filter((r) => r.group === g), false)),
      ),
    ),
    host.canUndo() ? h("div", { class: "menu-sep" }) : null,
    host.canUndo() ? menuItem("Undo the last removal", undo) : null,
  );
}

function setField(body) {
  const field = h("select", { "aria-label": "Field" }, Object.entries(FIELD_LABELS).map(([f, label]) => h("option", { value: f }, label)));
  const value = h("input", { "aria-label": "Value", placeholder: "New value (leave empty to clear it)", spellcheck: false });
  const apply = () => {
    const rows = host.checked();
    host.setField(rows, field.value, value.value);
    closePopover();
    toast(`${FIELD_LABELS[field.value]} set on ${plural(rows.length, "row", "rows")}`, "ok");
  };
  value.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      apply();
    }
  });
  body.replaceChildren(
    h(
      "div",
      { class: "bulk-panel" },
      h("div", { class: "section-heading" }, `Set a field on ${plural(host.checked().length, "checked row", "checked rows")}`),
      h("div", { class: "line" }, field, value),
      h("div", { class: "line" }, h("button", { class: "btn primary", type: "button", onClick: apply }, "Set"), h("button", { class: "btn subtle", type: "button", onClick: () => mainMenu(body) }, "Back")),
    ),
  );
  value.focus();
}

function setQty(body) {
  const qty = h("input", { type: "number", min: "1", max: "999", value: "1", "aria-label": "Quantity", inputmode: "numeric" });
  const apply = () => {
    const n = Math.max(1, Math.min(999, parseInt(qty.value, 10) || 1));
    const rows = host.checked();
    host.setQty(rows, n);
    closePopover();
    toast(`Quantity ${n} on ${plural(rows.length, "row", "rows")}`, "ok");
  };
  qty.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      apply();
    }
  });
  body.replaceChildren(
    h(
      "div",
      { class: "bulk-panel" },
      h("div", { class: "section-heading" }, `Quantity for ${plural(host.checked().length, "checked row", "checked rows")}`),
      h("div", { class: "line" }, qty),
      h("div", { class: "line" }, h("button", { class: "btn primary", type: "button", onClick: apply }, "Set"), h("button", { class: "btn subtle", type: "button", onClick: () => mainMenu(body) }, "Back")),
    ),
  );
  qty.select();
}

function remove(rows) {
  closePopover();
  host.remove(rows);
  toast(`Removed ${plural(rows.length, "row", "rows")}. Undo is in Actions.`, "ok", 5000);
}

function undo() {
  closePopover();
  const n = host.undo();
  if (n) toast(`Put back ${plural(n, "row", "rows")}`, "ok");
}

function tick(rows, on) {
  closePopover();
  host.setExcluded(rows, !on);
  toast(`${on ? "Ticked" : "Unticked"} ${plural(rows.length, "row", "rows")}`, "ok");
}

export function initBulk(h2) {
  host = h2;
  $("list-actions").addEventListener("click", () => {
    const body = h("div", { class: "menu bulk-menu" });
    mainMenu(body);
    openPopover($("list-actions"), body, { align: "end", cls: "bulk-popover" });
  });
}


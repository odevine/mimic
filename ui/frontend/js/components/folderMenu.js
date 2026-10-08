import { api } from "../api.js";
import { h, icon } from "../dom.js";
import { openPopover, closePopover } from "./popover.js";
import { toast } from "./toast.js";

// The output folder menu: recent folders first, then a Browse item that opens the
// system folder dialog. With no recent folder there is nothing to list, so it
// goes straight to the dialog. A chosen folder that does not exist yet is
// created when a run starts

export function openFolderMenu(anchor, { start, recent = [], onChoose }) {
  async function browse() {
    closePopover();
    try {
      const dir = await api.chooseFolder(start);
      if (dir) onChoose(dir);
    } catch (err) {
      toast(err.message, "err", 6000);
    }
  }
  if (!recent.length) return browse();

  const body = h(
    "div",
    { class: "menu folder-menu" },
    h("div", { class: "menu-heading section-heading" }, "Recent"),
    recent.map((p) =>
      h(
        "button",
        {
          class: "menu-item mono",
          type: "button",
          title: p,
          onClick: () => {
            closePopover();
            onChoose(p);
          },
        },
        icon("clock"),
        h("span", { class: "truncate" }, p),
      ),
    ),
    h("div", { class: "menu-sep" }),
    h("button", { class: "menu-item", type: "button", onClick: browse }, icon("folder"), "Browse…"),
  );
  openPopover(anchor, body, { align: "end", cls: "folder-popover" });
}

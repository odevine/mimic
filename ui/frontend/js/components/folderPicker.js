import { api } from "../api.js";
import { h, icon } from "../dom.js";
import { openPopover, closePopover, repositionPopover } from "./popover.js";

// The output folder picker. A browser cannot hand the server a real path, so
// this browses the filesystem through the server: recent folders first, then
// the current folder's subfolders, with a path field for typing one directly. A
// typed folder that does not exist yet is created when a run starts

export function openFolderPicker(anchor, { start, recent = [], onChoose }) {
  let current = null;

  const pathInput = h("input", {
    class: "mono",
    value: start || "",
    placeholder: "~/Documents/proxies",
    "aria-label": "Folder path",
    spellcheck: false,
    onKeydown: (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        go(pathInput.value);
      }
    },
  });
  const upBtn = h("button", { class: "btn subtle icon-only", type: "button", "aria-label": "Up one folder", "data-tip": "Up one folder", onClick: () => current?.parent && go(current.parent) }, icon("revert"));
  const homeBtn = h("button", { class: "btn subtle", type: "button", onClick: () => go("") }, "Home");
  const list = h("div", { class: "menu folder-list", role: "listbox", "aria-label": "Folders" });
  const note = h("p", { class: "folder-note dim", role: "status" });
  const useBtn = h("button", { class: "btn primary", type: "button", onClick: () => choose(pathInput.value) }, icon("check"), "Use this folder");

  const recentList = recent.length
    ? h(
        "div",
        { class: "menu" },
        h("div", { class: "menu-heading section-heading" }, "Recent"),
        recent.map((p) =>
          h("button", { class: "menu-item mono", type: "button", title: p, onClick: () => choose(p) }, icon("clock"), h("span", { class: "truncate" }, p)),
        ),
        h("div", { class: "menu-sep" }),
      )
    : null;

  const body = h(
    "div",
    { class: "folder-picker" },
    recentList,
    h("div", { class: "folder-bar" }, upBtn, pathInput, homeBtn),
    list,
    note,
    h("div", { class: "folder-foot" }, h("span", { class: "spacer" }), useBtn),
  );

  function choose(path) {
    const p = (path || "").trim();
    if (!p) {
      note.textContent = "Type or pick a folder first.";
      return;
    }
    closePopover();
    onChoose(p);
  }

  async function go(path) {
    note.textContent = "";
    list.replaceChildren(h("div", { class: "menu-item faint" }, "Loading…"));
    try {
      current = await api.listDir(path);
    } catch (err) {
      // A folder that does not exist yet is still a valid choice
      list.replaceChildren();
      note.textContent = `${err.message}. Use it anyway and the run creates it.`;
      pathInput.value = path;
      return;
    }
    pathInput.value = current.path;
    upBtn.disabled = !current.parent;
    list.replaceChildren(
      ...(current.dirs.length
        ? current.dirs.map((name) =>
            h(
              "button",
              {
                class: "menu-item",
                type: "button",
                onClick: () => go(joinPath(current.path, name)),
              },
              icon("folder"),
              h("span", { class: "truncate" }, name),
            ),
          )
        : [h("div", { class: "menu-item faint" }, "No subfolders")]),
    );
    repositionPopover("end");
  }

  const el = openPopover(anchor, body, { align: "end", cls: "folder-popover", focus: false });
  if (!el) return;
  pathInput.focus();
  go(start || "");
}

// joinPath appends a folder name using whichever separator the path already has
function joinPath(dir, name) {
  const sep = dir.includes("\\") && !dir.includes("/") ? "\\" : "/";
  return dir.endsWith(sep) ? dir + name : dir + sep + name;
}

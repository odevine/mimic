import { $, h, icon } from "../dom.js";

// toast shows a transient confirmation in the corner. kind is "ok", "err", or
// empty for neutral
export function toast(text, kind = "", ms = 3200) {
  const glyph = kind === "err" ? "warn" : "check";
  const el = h("div", { class: `toast ${kind}` }, icon(glyph), h("span", {}, text));
  $("toasts").append(el);
  setTimeout(() => el.remove(), ms);
}

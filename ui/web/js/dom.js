// Small DOM helpers shared by every module

export const $ = (id) => document.getElementById(id);

// h builds an element. attrs sets properties for known keys and attributes for
// the rest, with on* keys wiring listeners. Children may be strings, nodes, or
// falsy values, which are skipped
export function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k.startsWith("on") && typeof v === "function") el.addEventListener(k.slice(2).toLowerCase(), v);
    else if (k === "class") el.className = v;
    else if (k === "dataset") Object.assign(el.dataset, v);
    else if (k in el && typeof v !== "string") el[k] = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat()) {
    if (c == null || c === false) continue;
    el.append(c instanceof Node ? c : String(c));
  }
  return el;
}

const SVG = "http://www.w3.org/2000/svg";

// icon returns a sprite glyph from icons.svg
export function icon(name, cls = "icon") {
  const svg = document.createElementNS(SVG, "svg");
  svg.setAttribute("class", cls);
  svg.setAttribute("aria-hidden", "true");
  const use = document.createElementNS(SVG, "use");
  use.setAttribute("href", `icons.svg#i-${name}`);
  svg.append(use);
  return svg;
}

// isTyping reports whether focus is in a text control, where single-key
// shortcuts must not fire
export function isTyping(el = document.activeElement) {
  if (!el) return false;
  if (el.isContentEditable) return true;
  if (el.tagName === "TEXTAREA" || el.tagName === "SELECT") return true;
  if (el.tagName === "INPUT") return !["checkbox", "radio", "button", "submit"].includes(el.type);
  return false;
}

export const isMac = /Mac|iPhone|iPad/.test(navigator.platform);

// modKey reports whether the platform's command modifier is held
export const modKey = (e) => (isMac ? e.metaKey : e.ctrlKey);

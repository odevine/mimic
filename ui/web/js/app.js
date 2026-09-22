import { api } from "./api.js";
import { $, isTyping, modKey } from "./dom.js";
import { app, effect } from "./state.js";
import { applyCapabilities, initGateGuard } from "./capabilities.js";
import { initTooltips } from "./components/tooltip.js";
import { initTemplatePicker, loadActiveTemplate } from "./components/templatePicker.js";
import { initSettings, applyTheme, loadResolution } from "./components/settings.js";
import { initPreview, setZoom } from "./components/preview.js";
import { initSplitters } from "./components/splitter.js";
import { popoverOpen } from "./components/popover.js";
import { initSingle, single } from "./flows/single.js";

// Entry point: loads what every mode shares, wires the shell, and routes
// between modes. Switching modes changes the workspace and nothing else

const MODES = ["single", "list", "art", "overrides", "run"];

async function loadCapabilities() {
  try {
    app.capabilities.value = await api.capabilities();
  } catch {
    // With no map every keyed control reads as not built, which is safe
  }
}

async function loadSettings() {
  try {
    const s = await api.settings();
    app.settings.value = { theme: "dark", expandPrintings: false, splits: {}, ...s };
  } catch {
    // defaults stand
  }
}

let saveTimer = 0;

// persistSplits saves one flow's region widths, debounced so a drag writes once
function persistSplits(flow, widths) {
  const s = app.settings.peek();
  app.settings.value = { ...s, splits: { ...(s.splits || {}), [flow]: widths.map(Math.round) } };
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => api.saveSettings(app.settings.peek()).catch(() => {}), 400);
}

function setMode(mode) {
  if (!MODES.includes(mode)) mode = "single";
  app.mode.value = mode;
  if (location.hash.slice(1) !== mode) history.replaceState(null, "", `#${mode}`);
}

function initShell() {
  for (const btn of document.querySelectorAll(".rail-item")) {
    btn.addEventListener("click", () => setMode(btn.dataset.mode));
  }
  effect(() => {
    const mode = app.mode.value;
    for (const panel of document.querySelectorAll("[data-mode-panel]")) panel.hidden = panel.dataset.modePanel !== mode;
    for (const btn of document.querySelectorAll(".rail-item")) {
      if (btn.dataset.mode === mode) btn.setAttribute("aria-current", "page");
      else btn.removeAttribute("aria-current");
    }
  });
  window.addEventListener("hashchange", () => setMode(location.hash.slice(1)));

  // Overrides scope switcher
  const scopes = { template: $("scope-template"), global: $("scope-global") };
  for (const [name, chip] of Object.entries(scopes)) {
    chip.addEventListener("click", () => {
      for (const [n, c] of Object.entries(scopes)) {
        c.setAttribute("aria-pressed", String(n === name));
        $(`scope-${n}-panel`).hidden = n !== name;
      }
    });
  }

  $("help-btn").addEventListener("click", () => $("shortcuts-dialog").showModal());
  for (const d of document.querySelectorAll("dialog")) {
    d.addEventListener("click", (e) => {
      if (e.target === d || e.target.closest("[data-close]")) d.close();
    });
  }

  // Status bar
  effect(() => {
    $("status").textContent = app.status.value;
  });
  effect(() => {
    const p = app.progress.value;
    const el = $("status-progress");
    el.hidden = p === null;
    if (p !== null) el.firstElementChild.style.width = `${p * 100}%`;
  });
  effect(() => {
    const t = app.template.value;
    if (!t) return;
    $("template-label").textContent = t.label;
    const state = $("template-state");
    state.textContent = `${t.label}${t.source === "placeholder" ? "" : ` ${t.source}`}`;
    state.classList.toggle("warn", t.source === "placeholder");
    state.dataset.tip =
      t.source === "placeholder"
        ? "No template assets are available, so frames render as placeholders"
        : t.source === "local"
          ? "Rendering from a local developer directory"
          : "Rendering from a downloaded template bundle";
  });
}

function initKeyboard() {
  document.addEventListener("keydown", (e) => {
    if (e.defaultPrevented) return;
    const typing = isTyping();
    const dialogOpen = !!document.querySelector("dialog[open]");

    if (modKey(e) && !e.shiftKey && !e.altKey) {
      if (/^[1-5]$/.test(e.key)) {
        e.preventDefault();
        setMode(MODES[Number(e.key) - 1]);
        return;
      }
      if (e.key === "Enter" && app.mode.peek() === "single" && !dialogOpen) {
        e.preventDefault();
        single.render();
        return;
      }
      if (e.key.toLowerCase() === "s" && app.mode.peek() === "single" && !dialogOpen) {
        e.preventDefault();
        single.save();
        return;
      }
      if (e.key.toLowerCase() === "k") {
        e.preventDefault();
        $("palette-btn").click();
        return;
      }
    }
    if (typing || dialogOpen || popoverOpen() || e.metaKey || e.ctrlKey || e.altKey) return;

    if (e.key === "/") {
      e.preventDefault();
      if (app.mode.peek() !== "single") setMode("single");
      single.focusSearch();
    } else if (e.key === "?") {
      e.preventDefault();
      $("shortcuts-dialog").showModal();
    } else if (app.mode.peek() === "single" && ["0", "1", "2"].includes(e.key)) {
      setZoom(Number(e.key));
    }
  });
}

async function boot() {
  await Promise.all([loadCapabilities(), loadSettings(), loadActiveTemplate(), loadResolution()]);
  applyTheme(app.settings.peek().theme);
  applyCapabilities();

  initTooltips();
  initGateGuard();
  initShell();
  initTemplatePicker();
  initSettings();
  initPreview();
  initSingle();
  initKeyboard();
  initSplitters(document.querySelector("#mode-single .regions"), (app.settings.peek().splits || {}).single, (w) =>
    persistSplits("single", w),
  );

  // Anything resolved against the template refreshes when it changes, which is
  // also when a needs-template gate could lift
  document.addEventListener("mimic:template-changed", async () => {
    await Promise.all([loadCapabilities(), loadResolution()]);
    applyCapabilities();
  });

  setMode(location.hash.slice(1) || "single");
  document.documentElement.classList.add("booted");
  if (app.mode.peek() === "single") $("search-input").focus();
}

boot().catch((err) => window.mimicBootFailed && window.mimicBootFailed(err.message));

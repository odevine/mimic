import { api } from "../api.js";
import { $ } from "../dom.js";
import { app } from "../state.js";

// The active template's name in the top bar. It opens the Templates mode,
// where the catalog, downloads, and each face's template are managed

export async function loadActiveTemplate() {
  try {
    app.template.value = await api.activeTemplate();
  } catch {
    // keep the last known template on a failure
  }
}

export function initTemplatePicker() {
  $("template-trigger").addEventListener("click", () => {
    location.hash = "templates";
  });
}

// The favicon is mimic.svg filled with the active theme's accent. The file
// carries no color of its own, and a favicon cannot read the page's CSS, so the
// tinted copy is built here and swapped in as a data URL

let shape = null;

export async function tintFavicon() {
  const link = document.querySelector('link[rel="icon"]');
  if (!link) return;
  try {
    shape ??= await fetch("mimic.svg").then((r) => (r.ok ? r.text() : Promise.reject()));
  } catch {
    return; // the untinted file stays in place
  }
  const accent = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim();
  const svg = shape.replace("<svg ", `<svg fill="${accent}" `);
  link.href = `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

// The system theme swaps tokens when the OS scheme flips, with no applyTheme call
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => tintFavicon());

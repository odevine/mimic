// The launch check. The real page is served at /smoke.html with this script
// added, so it runs in the page itself, with the real runtime carrying its
// events. It makes one pass over the bridge, the job events and the image routes,
// then runs scripted flows that drive the page the way a person would, and
// reports the verdict to System.SmokeResult. Each flow uses a part of the
// webview that differs between the three platforms: dialogs, the tooltip popover,
// input events, image loading and job events. Nothing here needs the network: the
// list holds a custom card that renders from its own fields
import { Call } from "/wails/runtime.js";
import { api } from "/js/api.js";

const report = "github.com/odevine/mimic/ui/internal/desktop.System.SmokeResult";

const CSV = [
  "name,manaCost,typeLine,oracle,power,toughness,custom",
  '"Smoke Elf",{G},Creature — Elf Druid,"{T}: Add {G}.",1,1,true',
].join("\n");

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// until polls fn until it answers something, or throws once the time is up. what
// may be a function, which is called then to say where things stand
async function until(what, fn, ms = 20000) {
  const end = Date.now() + ms;
  for (;;) {
    const v = fn();
    if (v) return v;
    if (Date.now() > end) throw new Error("timed out waiting for " + (typeof what === "function" ? what() : what));
    await sleep(50);
  }
}

// An end to end test loads this page from the test server and says so, and may
// name the folder cards are written to
const params = new URLSearchParams(location.search);
const e2e = params.has("e2e");

const notes = [];

async function bridge() {
  const v = await api.version();
  if (!v.ui || !v.engine) throw new Error("System.Version answered " + JSON.stringify(v));
  if (v.testBuild && !e2e) throw new Error("the launch check ran against a test build");
  notes.push("version " + v.ui + " engine " + v.engine);

  const caps = await api.capabilities();
  if (!caps["flow.single"]) throw new Error("capabilities lack flow.single");

  // The page this script is part of has to boot, which means every call it makes
  // on the way up (capabilities, settings, template, resolution) has answered
  await until(
    () => "the page to finish booting: " + (document.body.innerText.trim().slice(0, 200) || "no output"),
    () => document.documentElement.classList.contains("booted"),
    30000,
  );
  notes.push("the page booted");

  const script = await fetch("/js/app.js");
  if (!script.ok) throw new Error("the embedded scripts did not load");

  const { jobId } = await api.render({ name: "Launch Check", typeLine: "Creature" }, {}, "preview", 0);
  let steps = 0;
  const done = await api.renderEvents(jobId, () => steps++);
  if (done.error) throw new Error(done.error);
  notes.push("render job " + jobId + " ended after " + steps + " progress events");

  const img = await fetch(api.renderImageURL(jobId));
  const blob = await img.blob();
  if (!img.ok || blob.type !== "image/png" || blob.size < 1000) throw new Error("render image answered " + img.status + " " + blob.type + " " + blob.size);

  const pip = await fetch(api.symbolURL("W", 24));
  if (!pip.ok) throw new Error("symbol answered " + pip.status);

  if (!e2e && (await api.latestRun()) !== null) throw new Error("a run existed before any started");

  let rejected = "";
  try {
    await api.stopRun("run-nope");
  } catch (e) {
    rejected = e && e.message ? e.message : String(e);
  }
  if (!rejected) throw new Error("a failing call did not reject");
  notes.push("errors reject with: " + rejected);
}

// flows runs each flow in turn and returns a note for each. outDir, when given,
// is the folder to write cards to, typed into the settings the way a person
// would. Without it the app's saved folder is used
async function flows(outDir) {
  const doc = document;
  const $ = (id) => doc.getElementById(id);
  // getClientRects is empty for an element that is not rendered, however it is positioned
  const shown = (el) => !!el && el.getClientRects().length > 0;
  const press = (el, type) => el.dispatchEvent(new Event(type, { bubbles: true }));
  const type = (el, value) => {
    el.value = value;
    press(el, "input");
    press(el, "change");
  };
  const notes = [];

  const openSettings = async () => {
    $("settings-btn").click();
    await until("the settings dialog to open", () => $("settings-dialog").open);
    // The resolution pickers fill in after the dialog opens, and a save before
    // they have is refused
    await until("the resolution pickers", () => $("preview-select").value && $("output-select").value);
  };
  const saveSettings = async () => {
    $("settings-save").click();
    try {
      await until("the settings dialog to close", () => !$("settings-dialog").open);
    } catch (err) {
      // A refused save leaves its reason in the dialog's status line
      throw new Error(`${err.message}: ${$("settings-status").textContent || "no reason given"}`);
    }
  };

  // A gated control shows its reason in a popover, which is the page's use of the
  // Popover API
  {
    $("palette-btn").click();
    const tip = $("tooltip");
    await until("the reason of a gated control", () => tip.matches(":popover-open") && /planned/i.test(tip.textContent));
    notes.push("a gated control explains itself in a popover");
    tip.hidePopover();
  }

  // The settings dialog opens, takes a choice and keeps it
  {
    if (outDir) {
      await openSettings();
      type($("output-dir-input"), outDir);
      await saveSettings();
    }
    await openSettings();
    type($("theme-select"), "light");
    await saveSettings();
    await until("the light theme", () => doc.documentElement.dataset.theme === "light");
    const saved = await api.settings();
    if (saved.theme !== "light") throw new Error("the theme was not kept: " + JSON.stringify(saved.theme));
    await openSettings();
    type($("theme-select"), "dark");
    await saveSettings();
    await until("the dark theme", () => doc.documentElement.dataset.theme === "dark");
    notes.push("the settings dialog keeps a choice");
  }

  // A list of one custom card resolves, opens its menu and renders to a folder
  {
    doc.querySelector('button[data-mode="list"]').click();
    const input = await until("the list box", () => (shown($("list-input")) ? $("list-input") : null));
    type(input, CSV);
    $("list-resolve").click();
    await until("the card to resolve", () => /1 custom/.test($("list-summary").textContent));
    const row = await until("the card's row", () => doc.querySelector("#list-table-el .row-main"));
    if (!/custom/.test(row.textContent)) throw new Error("the row reads: " + row.textContent);

    $("list-actions").click();
    const menu = await until("the actions menu", () => doc.querySelector(".bulk-menu"));
    if (menu.getBoundingClientRect().height === 0) throw new Error("the actions menu has no height");
    doc.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await until("the actions menu to close", () => !doc.querySelector(".bulk-menu"));
    notes.push("the actions menu opens and closes");

    await until("the render button", () => !$("list-render").disabled);
    // The console may still show an earlier run, so the new one is told by its id
    const earlier = ((await api.latestRun()) || {}).id || "";
    const idOf = () => $("run-meta").textContent.split(" ")[0];
    $("list-render").click();
    await until("the run to start", () => idOf() && idOf() !== earlier);
    try {
      await until(
        () => `the run to finish (the console reads "${$("run-state").textContent}", the status "${$("status").textContent}", the toasts "${$("toasts").textContent}")`,
        () => /^finished/.test($("run-state").textContent),
        60000,
      );
    } catch (err) {
      // What the app itself says the run reached is the best clue to where it stuck
      throw new Error(`${err.message}, the app reports ${JSON.stringify(await api.latestRun())}`);
    }
    const run = await api.latestRun();
    const done = run && run.cards.filter((c) => c.status === "done");
    if (!done || done.length !== 1 || !/\.(jpg|png)$/.test(done[0].file)) throw new Error("the run reports " + JSON.stringify(run && run.cards));
    notes.push("a run wrote " + done[0].file);
  }

  // The row opens in Single, which renders a preview image through the app's
  // image route
  {
    doc.querySelector('button[data-mode="list"]').click();
    await until("the list", () => shown($("list-input")));
    doc.querySelector('#list-table-el [aria-label="Edit Smoke Elf"]').click();
    await until("the inspector", () => shown($("list-inspector")));
    $("inspector-single").click();
    await until(
      () => `the Single editor for the card (the mode is ${doc.querySelector(".mode:not([hidden])")?.id}, the editor ${shown($("editor")) ? "shows" : "is hidden"} "${$("editor-title").textContent}", the toasts "${$("toasts").textContent}")`,
      () => shown($("editor")) && $("editor-title").textContent === "Smoke Elf",
    );
    await until("the preview", () => {
      const img = $("preview-img");
      return $("activity").dataset.state === "done" && img.complete && img.naturalWidth > 0;
    }, 30000);
    notes.push("Single shows a rendered preview");
  }

  // The editor makes a template from its dialog and draws a test card with it,
  // through the same image route
  {
    doc.querySelector('button[data-mode="editor"]').click();
    await until("the editor", () => shown($("ed-empty")) || shown($("ed-workspace")));
    (shown($("ed-empty-new")) ? $("ed-empty-new") : $("ed-new")).click();
    await until("the new template dialog", () => $("draft-dialog").open);
    type($("draft-name"), "Smoke frame");
    type($("draft-dpi"), "100");
    $("draft-confirm").click();
    await until("the dialog to close", () => !$("draft-dialog").open);
    await until(
      () => `the test card (the strip reads "${$("ed-activity-title").textContent}", "${$("ed-activity-step").textContent}")`,
      () => {
        const img = $("ed-img");
        return $("ed-activity").dataset.state === "done" && img.complete && img.naturalWidth > 0;
      },
      30000,
    );
    notes.push("the editor draws a test card with a new template");

    // A layer picked from the list is moved a pixel with an arrow key, which is saved
    // and read back as the new place
    const rows = await until("the layers", () => doc.querySelectorAll("#ed-layers .ed-layer").length > 3 && [...doc.querySelectorAll("#ed-layers .ed-layer")]);
    rows.find((row) => /^title/.test(row.textContent)).click();
    const readX = () => Number($("ed-sel-x").textContent);
    await until("the selection", () => shown($("ed-selection")) && $("ed-selection-name").textContent === "title");
    const x = readX();
    doc.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    await until(() => `the title to move (it is at ${readX()}, from ${x})`, () => readX() === x + 1 && !$("ed-undo").disabled);
    doc.dispatchEvent(new KeyboardEvent("keydown", { key: "z", ctrlKey: true, bubbles: true }));
    await until("the move to be undone", () => readX() === x && $("ed-undo").disabled);
    notes.push("an edit in the editor is saved, shown and undone");

    // A lever changes a field in the file, and the JSON editor shows the file with it
    const size = $("lever-fontSize");
    const was = Number(size.value);
    type(size, String(was + 3));
    const draftId = $("ed-about-id").textContent.replace(/^user\./, "");
    const sizeInFile = async () => {
      const src = await api.draftSource(draftId);
      const layout = JSON.parse(src.files.find((f) => f.name === "layouts/normal.json").content);
      return layout.textBoxes.title.text.fontSize;
    };
    const end = Date.now() + 20000;
    while ((await sizeInFile()) !== was + 3) {
      if (Date.now() > end) throw new Error("timed out waiting for the lever's change to be saved");
      await sleep(100);
    }
    $("ed-json").click();
    await until("the JSON editor", () => {
      if (!shown($("ed-json-pane"))) return false;
      try {
        return !!JSON.parse($("ed-json-text").value).contract;
      } catch {
        return false;
      }
    });
    $("ed-json-close").click();
    $("ed-undo").click();
    await until("the lever's change to be undone", () => Number($("lever-fontSize").value) === was);
    notes.push("a lever edits the file and the JSON editor shows it");

    // A text box is added from the dialog, which is saved, locking it is kept with the
    // draft, and undo takes the box away
    const boxKeys = async () => {
      const src = await api.draftSource(draftId);
      return Object.keys(JSON.parse(src.files.find((f) => f.name === "layouts/normal.json").content).textBoxes);
    };
    $("ed-add").click();
    await until("the add dialog", () => $("ed-add-dialog").open);
    $("ed-add-confirm").click();
    await until("the add dialog to close", () => !$("ed-add-dialog").open);
    const added = Date.now() + 20000;
    while (!(await boxKeys()).includes("artist_2")) {
      if (Date.now() > added) throw new Error("timed out waiting for the new text box to be saved");
      await sleep(100);
    }
    await until("the new box to be selected", () => $("ed-selection-name").textContent === "artist_2");
    doc.querySelector('#ed-layers .ed-layer.selected [data-toggle="locked"]').click();
    const locked = Date.now() + 20000;
    while (!((await api.openDraft(draftId)).meta.locked || []).includes("normal/text:artist_2")) {
      if (Date.now() > locked) throw new Error("timed out waiting for the lock to be kept");
      await sleep(100);
    }
    $("ed-undo").click();
    const gone = Date.now() + 20000;
    while ((await boxKeys()).includes("artist_2")) {
      if (Date.now() > gone) throw new Error("timed out waiting for the new text box to be undone");
      await sleep(100);
    }
    notes.push("a text box is added, locked and undone");
  }

  return notes;
}

try {
  await bridge();
  notes.push(...(await flows(params.get("out") || "")));
  window.__smoke = { ok: true, detail: notes.join("; ") };
  await Call.ByName(report, true, window.__smoke.detail);
} catch (e) {
  window.__smoke = { ok: false, detail: e && e.message ? e.message : String(e) };
  await Call.ByName(report, false, window.__smoke.detail);
}

// Every call the frontend makes into the app, so the whole surface is readable
// in one place. The window exposes the Go services as bound methods, reached by
// name through the runtime Wails serves, and sends job progress as window events.
// A test on the Go side keeps the names below in step with the services

import { Call, Events } from "/wails/runtime.js";

const NS = "github.com/odevine/mimic/ui/internal/";

// rpc calls one method of a bound service, as "Cards" and "Search"
const rpc = (service, method, ...args) => Call.ByName(`${NS}desktop.${service}.${method}`, ...args);

function message(err) {
  return (err && err.message ? err.message : String(err)).trim();
}

// abortable lets an AbortSignal cancel a call that is already running. The
// promise then rejects the way an aborted fetch does, so a caller that ignores
// superseded requests by checking for AbortError keeps working
function abortable(call, signal) {
  if (!signal) return call;
  return new Promise((resolve, reject) => {
    const abort = () => {
      call.cancel();
      reject(new DOMException("The call was aborted", "AbortError"));
    };
    if (signal.aborted) return abort();
    signal.addEventListener("abort", abort, { once: true });
    call.then(resolve, (err) => reject(new Error(message(err))));
  });
}

// --- jobs ---
//
// A job reports progress as "job" window events. They are collected from the
// moment this module loads, so a job that starts reporting before its caller
// begins watching loses nothing, and each event carries a sequence number so
// the same one is never applied twice. A page that reloaded mid-job asks the app
// for what it missed

const jobs = new Map(); // id -> { seq, events, watchers }

function jobState(id) {
  let j = jobs.get(id);
  if (!j) {
    j = { seq: 0, events: [], watchers: new Set() };
    jobs.set(id, j);
  }
  return j;
}

function deliver(id, e) {
  const j = jobState(id);
  if (e.seq <= j.seq) return;
  j.seq = e.seq;
  j.events.push(e);
  for (const w of [...j.watchers]) w(e);
}

Events.On("job", (ev) => {
  const d = Array.isArray(ev.data) ? ev.data[0] : ev.data;
  if (d && d.id) for (const e of d.events || []) deliver(d.id, e);
});

// watchJob follows a job's events, calling onStep with each progress event's
// step, fraction and the whole event, and resolving with the terminal event. It
// rejects when the job reports an error. The returned promise carries close() so
// a caller can abandon a job it superseded
export function watchJob(jobId, onStep) {
  const j = jobState(jobId);
  let watcher;
  const p = new Promise((resolve, reject) => {
    watcher = (e) => {
      if (e.done) {
        j.watchers.delete(watcher);
        jobs.delete(jobId);
        if (e.error) reject(new Error(e.error));
        else resolve(e);
        return;
      }
      if (onStep) onStep(e.step, e.frac, e);
    };
    j.watchers.add(watcher);
    // Events already collected, then anything the app holds beyond them
    for (const e of [...j.events]) watcher(e);
    Call.ByName(`${NS}jobs.Service.Backlog`, jobId, j.seq).then(
      (b) => b.events.forEach((e) => deliver(jobId, e)),
      (err) => {
        if (!j.events.some((e) => e.done)) reject(new Error(message(err)));
      },
    );
  });
  p.close = () => {
    j.watchers.delete(watcher);
    if (!j.watchers.size) jobs.delete(jobId);
  };
  return p;
}

// --- native dialogs and links ---

const TEXT_LISTS = [{ name: "Card lists", pattern: "*.txt;*.csv;*.dek;*.mtga" }];
const IMAGES = [{ name: "Images", pattern: "*.png;*.jpg;*.jpeg" }];
const FONTS = [{ name: "Fonts", pattern: "*.ttf;*.otf" }];

const SYSTEM = "System";

// pickFile and pickFolder resolve to a path, or an empty string when the user
// cancelled the dialog
const pickFile = (req) => rpc(SYSTEM, "PickFile", req);
const pickFolder = (req) => rpc(SYSTEM, "PickFolder", req);

export const api = {
  version: () => rpc(SYSTEM, "Version"),
  openURL: (url) => rpc(SYSTEM, "OpenURL", url),
  // confirm asks a yes or no question in a native dialog, since the webview has
  // no confirm() of its own
  confirm: (title, message, accept) => rpc(SYSTEM, "Confirm", { title, message, accept }),

  capabilities: () => rpc("Settings", "Capabilities"),
  settings: () => rpc("Settings", "Get"),
  saveSettings: (s) => rpc("Settings", "Put", s),
  // resources describes this computer's memory and what one output render costs,
  // for the concurrency setting
  // cache, when given, names a layer cache size to plan for in place of the saved one
  resources: (cache) => rpc("Settings", "Resources", cache || ""),

  search: (q, signal) => abortable(rpc("Cards", "Search", q), signal),
  recents: () => rpc("Cards", "Recents"),
  printings: (name, signal) => abortable(rpc("Cards", "Printings", name), signal),
  symbolURL: (code, px = 36) => `/img/symbol?code=${encodeURIComponent(code)}&px=${px}`,

  // render starts a job for one target, "preview" or "output", and resolves to
  // { jobId, dpi }
  render: (base, edits, target, face) => rpc("Render", "Start", { base, edits, target, face }),
  cancelRender: (jobId) => rpc("Render", "Cancel", jobId).catch(() => {}),
  renderEvents: (jobId, onStep) => watchJob(jobId, onStep),
  renderImageURL: (jobId) => `/img/render/${jobId}?ts=${Date.now()}`,
  // saveRender asks where to put a finished render and writes it there. It
  // resolves to the path, or an empty string when the user cancelled. dir is
  // the folder the dialog opens in
  saveRender: async (jobId, dir) => {
    const name = await rpc("Render", "SuggestedFilename", jobId);
    const path = await pickFile({ title: "Save the card", name, dir: dir || "", save: true, filters: [{ name: "PNG image", pattern: "*.png" }] });
    if (!path) return "";
    await rpc("Render", "Save", jobId, path);
    return path;
  },

  resolution: () => rpc("Settings", "Resolution"),
  saveResolution: (previewDpi, outputDpi) => rpc("Settings", "SetResolution", { previewDpi, outputDpi }),

  templates: () => rpc("Templates", "List"),
  activeTemplate: () => rpc("Templates", "Active"),
  selectTemplate: async (name, version, install = false) => ({ jobId: await rpc("Templates", "Select", { name, version, install }) }),
  selectEvents: (jobId, onStep) => watchJob(jobId, onStep),
  faceTemplates: () => rpc("Templates", "Faces"),
  setFaceTemplate: (key, name, version) => rpc("Templates", "SetFace", { key, name, version }),

  // resolve parses a list and resolves to { jobId, format, rows }, with one
  // resolved row per event on the stream
  resolve: (text, format) => rpc("List", "Resolve", { text, format }),
  resolveEvents: (jobId, onStep) => watchJob(jobId, onStep),
  // chooseListFile asks for a list file and resolves to its path, or an empty
  // string when the user cancelled, and readListFile returns a file's text
  chooseListFile: () => pickFile({ title: "Open a card list", filters: TEXT_LISTS }),
  readListFile: (path) => rpc("List", "ReadFile", path),
  // onFilesDropped calls back with the paths of files dropped on the window
  onFilesDropped: (cb) =>
    Events.On("files-dropped", (ev) => {
      const files = Array.isArray(ev.data) && Array.isArray(ev.data[0]) ? ev.data[0] : ev.data;
      if (Array.isArray(files) && files.length) cb(files);
    }),

  // run starts a batch. mpc, { stock, foil }, makes it an MPC Autofill project
  run: (rows, outDir, label, mpc) => rpc("Run", "Start", { rows, outDir, label, mpc }),
  latestRun: () => rpc("Run", "Latest"),
  runEvents: (id, onStep) => watchJob(id, onStep),
  runImageURL: (id, n) => `/img/run/${id}/${n}`,
  stopRun: (id) => rpc("Run", "Stop", id),
  retryRun: (id) => rpc("Run", "Retry", id),
  openRunFolder: (id) => rpc("Run", "OpenFolder", id),

  // chooseFolder asks for a folder, opening in start, and resolves to its path or
  // an empty string
  chooseFolder: (start) => pickFolder({ title: "Choose the output folder", dir: start || "" }),

  mpc: () => rpc("Run", "MPCInfo"),
  // chooseCardback asks for an image and stores it as the cardback, resolving to
  // the new options, or null when the user cancelled
  chooseCardback: async () => {
    const path = await pickFile({ title: "Choose a cardback", filters: IMAGES });
    return path ? rpc("Run", "SetCardback", path) : null;
  },
  cardbackURL: () => `/img/cardback?ts=${Date.now()}`,
  // mpcFolder resolves to { existing }, the files an earlier project left in path
  mpcFolder: (path) => rpc("Run", "MPCFolder", path),

  // cardData reports the local copy of Scryfall bulk data and what a download
  // would fetch, and downloadCardData starts a download job
  cardData: () => rpc("Data", "CardData"),
  downloadCardData: async () => ({ jobId: await rpc("Data", "DownloadCardData") }),
  cardDataEvents: (jobId, onStep) => watchJob(jobId, onStep),
  removeCardData: () => rpc("Data", "RemoveCardData"),

  // fonts reports which font each role draws with, and addFont and removeFont
  // change the app's own fonts folder, each answering with the new status.
  // addFont asks for the font file, and resolves to null when the user cancelled
  fonts: () => rpc("Data", "Fonts"),
  addFont: async (folder) => {
    const path = await pickFile({ title: "Choose a font", filters: FONTS });
    return path ? rpc("Data", "AddFont", folder, path) : null;
  },
  removeFont: (folder) => rpc("Data", "RemoveFont", folder),
  openFonts: () => rpc("Data", "OpenFonts"),
};

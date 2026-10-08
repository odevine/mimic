import { api } from "../api.js";
import { $ } from "../dom.js";
import { app } from "../state.js";
import { toast } from "./toast.js";

// Updates. The app checks GitHub for a newer release at launch, at most once a
// day, and keeps a status the page reads: nothing to do, a release on offer, a
// download under way, or a release staged and waiting for a restart. Two places
// show it. A strip in the top bar offers the release and can be dismissed for
// that version, and the Updates group in Settings always shows where things
// stand and carries the buttons. Nothing downloads or restarts until the user
// asks

let status = null;

const mb = (n) => `${(n / 1e6).toFixed(n < 1e7 ? 1 : 0)} MB`;

// dismissed is whether the user closed the strip for the release on offer
const dismissed = () => !!status?.release && app.settings.peek().dismissedUpdate === status.release.version;

// summary is the one line the Settings row shows for each state
function summary(s) {
  const v = s.release?.version;
  switch (s.state) {
    case "unavailable":
      return s.reason;
    case "checking":
      return "Checking for updates…";
    case "up-to-date":
      return "Mimic is up to date.";
    case "available":
      return `Mimic ${v} is available${s.release.size ? ` (${mb(s.release.size)})` : ""}.`;
    case "downloading":
      return s.total ? `Downloading Mimic ${v}, ${Math.round((s.written / s.total) * 100)}%` : `Downloading Mimic ${v}…`;
    case "ready":
      return `Mimic ${v} is ready. Restart to use it.`;
    case "error":
      return `The update failed: ${s.error}`;
    default:
      return "";
  }
}

// action is the button each state offers: install, restart, or a link to the
// release page for an install the app cannot replace itself in
function action(s) {
  if (s.state === "ready") return { label: "Restart to update", run: restart };
  if (s.state === "available" && s.canInstall) return { label: "Install update", run: install };
  if (s.state === "available") return { label: "Download", run: openPage };
  return null;
}

function render() {
  if (!status) return;
  const s = status;
  const a = action(s);

  // The top bar strip
  const strip = $("update-strip");
  const showStrip = !!a && !dismissed() && s.state !== "downloading";
  strip.hidden = !showStrip && s.state !== "downloading";
  if (!strip.hidden) {
    $("update-strip-text").textContent = s.state === "downloading" ? summary(s) : s.state === "ready" ? `Mimic ${s.release.version} is ready` : `Mimic ${s.release.version} is available`;
    $("update-strip-action").hidden = !a;
    if (a) $("update-strip-action").textContent = a.label;
    $("update-strip-dismiss").hidden = s.state === "downloading" || s.state === "ready";
  }

  // The Settings row
  $("update-version").textContent = `Mimic ${s.current}`;
  $("update-summary").textContent = summary(s);
  $("update-summary").classList.toggle("err", s.state === "error");
  const busy = s.state === "checking" || s.state === "downloading";
  $("update-check").disabled = busy || s.state === "unavailable";
  $("update-action").hidden = !a;
  if (a) $("update-action").textContent = a.label;
  $("update-action").disabled = busy;
  const notes = $("update-notes");
  notes.hidden = !s.release?.notes;
  notes.textContent = s.release?.notes || "";
}

async function guarded(fn) {
  try {
    return await fn();
  } catch (err) {
    toast(err.message, "err", 6000);
  }
}

const install = () => guarded(() => api.installUpdate());
const restart = () => guarded(() => api.restartForUpdate());
const openPage = () => guarded(() => api.openURL(status.release.pageUrl || "https://github.com/odevine/mimic/releases"));

async function check() {
  const next = await guarded(() => api.checkUpdates());
  if (next) status = next;
  render();
}

// dismiss closes the strip for the release on offer, and saves that so it stays
// closed until a newer release comes out
async function dismiss() {
  if (!status?.release) return;
  const next = { ...app.settings.peek(), dismissedUpdate: status.release.version };
  app.settings.value = next;
  render();
  api.saveSettings(next).catch(() => {});
}

// openUpdates refreshes the Settings row when the panel opens
export function openUpdates() {
  render();
}

export async function initUpdates() {
  try {
    status = await api.updateStatus();
  } catch {
    return;
  }
  api.onUpdate((s) => {
    status = s;
    render();
  });
  $("update-check").addEventListener("click", check);
  $("update-action").addEventListener("click", () => action(status)?.run());
  $("update-strip-action").addEventListener("click", () => action(status)?.run());
  $("update-strip-dismiss").addEventListener("click", dismiss);
  render();
}

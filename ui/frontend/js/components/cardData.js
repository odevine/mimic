import { api } from "../api.js";
import { $ } from "../dom.js";
import { toast } from "./toast.js";

// The Card data setting: whether lookups go to Scryfall's API or to a local copy
// of its bulk data, and the download that installs that copy. Picking the local
// copy with none installed prompts the download. The choice itself saves with
// the rest of the settings, and a download runs on its own

// A copy older than this gets a nudge to update, since Scryfall refreshes daily
const STALE_DAYS = 7;

let status = null; // the last GET /api/carddata
let job = null; // the download being watched
let progress = null; // { step, frac } while a download runs
let watched = ""; // the last job watched, so a refresh never replays it

const mb = (bytes) => `${Math.round(bytes / 1e6)} MB`;
const day = (iso) => new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
const ageDays = (iso) => Math.floor((Date.now() - new Date(iso).getTime()) / 86400000);

function draw() {
  const text = $("card-data-text");
  const dl = $("card-data-download");
  const rm = $("card-data-remove");
  const bar = $("card-data-progress");
  const chosen = $("card-data-select").value;
  const box = $("card-data");
  box.classList.remove("warn", "err");
  bar.hidden = !progress;
  dl.hidden = rm.hidden = true;
  dl.disabled = !!progress;
  if (!status) {
    text.textContent = "Checking the card data…";
    return;
  }
  const { local, remote } = status;
  const size = remote ? ` About ${mb(remote.size)} to download.` : "";

  if (progress) {
    text.textContent = progress.step || "Starting the download…";
    bar.firstElementChild.style.width = `${progress.frac * 100}%`;
    return;
  }
  switch (local.state) {
    case "loading":
      text.textContent = "Loading the local card data…";
      return;
    case "error":
      box.classList.add("err");
      text.textContent = local.error;
      dl.hidden = false;
      $("card-data-download-label").textContent = "Download again";
      return;
    case "ready": {
      const age = ageDays(local.updatedAt);
      const newer = remote && new Date(remote.updatedAt) > new Date(local.updatedAt);
      let line = `${local.printings.toLocaleString()} printings from ${day(local.updatedAt)}, ${mb(local.bytes)} on disk.`;
      if (chosen !== "local") line += " Installed but not in use.";
      if (age >= STALE_DAYS) {
        box.classList.add("warn");
        line += ` It is ${age} days old, and Scryfall updates daily.`;
      } else if (newer) {
        line += ` A newer copy from ${day(remote.updatedAt)} is available.`;
      }
      text.textContent = line;
      dl.hidden = !(age >= STALE_DAYS || newer);
      $("card-data-download-label").textContent = "Update";
      rm.hidden = false;
      return;
    }
    default:
      if (chosen === "local") {
        text.textContent = `Download Scryfall's card data to look cards up on this computer, so a list resolves at once instead of two cards a second.${size}`;
        dl.hidden = false;
        $("card-data-download-label").textContent = "Download";
      } else {
        text.textContent = "Lookups go to Scryfall, two cards a second. A local copy resolves a list at once.";
      }
  }
}

async function refresh() {
  try {
    status = await api.cardData();
  } catch {
    status = { local: { state: "error", error: "Could not read the card data status." } };
  }
  draw();
  if (status.local.jobId && !job && status.local.jobId !== watched) watch(status.local.jobId);
}

function watch(jobId) {
  watched = jobId;
  progress = { step: "", frac: 0 };
  draw();
  job = api.cardDataEvents(jobId, (step, frac) => {
    progress = { step, frac };
    draw();
  });
  job
    .then(
      (e) => toast(e.step || "Card data installed", "ok"),
      (err) => toast(`Card data download failed: ${err.message}`, "err", 6000),
    )
    .finally(() => {
      job = null;
      progress = null;
      refresh();
    });
}

async function download() {
  try {
    const { jobId } = await api.downloadCardData();
    watch(jobId);
  } catch (err) {
    toast(err.message, "err", 6000);
  }
}

async function remove() {
  try {
    await api.removeCardData();
  } catch (err) {
    toast(err.message, "err");
  }
  refresh();
}

// openCardData fills the setting from the saved choice when settings open
export function openCardData(source) {
  $("card-data-select").value = source === "local" ? "local" : "api";
  refresh();
}

// cardDataChoice is the value to save, "" for the API
export function cardDataChoice() {
  return $("card-data-select").value === "local" ? "local" : "";
}

export function initCardData() {
  $("card-data-select").addEventListener("change", draw);
  $("card-data-download").addEventListener("click", download);
  $("card-data-remove").addEventListener("click", remove);
}

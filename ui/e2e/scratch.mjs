// The scratch folder the test server's app runs in, and what the app wrote to it.
// The app answers file dialogs from the dialogs folder, so a save lands in
// dialogs/saved and an open reads from dialogs/open
import { mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";

export const home = path.resolve(import.meta.dirname, ".scratch/home");
const dialogs = path.join(home, "dialogs");

// resetDialogs empties the dialogs folder, so a test starts with nothing saved,
// nothing to open and every question answered yes
export async function resetDialogs() {
  await rm(dialogs, { recursive: true, force: true });
  await mkdir(path.join(dialogs, "open"), { recursive: true });
}

// offerFile puts a file where an open dialog will find it
export async function offerFile(name, contents) {
  await mkdir(path.join(dialogs, "open"), { recursive: true });
  await writeFile(path.join(dialogs, "open", name), contents);
}

// saved lists the files saved so far, as names
export async function saved() {
  try {
    return (await readdir(path.join(dialogs, "saved"))).sort();
  } catch {
    return [];
  }
}

export const savedPath = (name) => path.join(dialogs, "saved", name);

// events lists what the app did with dialogs and links, as [kind, detail] pairs
export async function events() {
  try {
    const raw = await readFile(path.join(dialogs, "events.log"), "utf8");
    return raw.trim().split("\n").filter(Boolean).map((line) => line.split("\t"));
  } catch {
    return [];
  }
}

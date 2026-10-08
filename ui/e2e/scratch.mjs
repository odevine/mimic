// The scratch folder the test server's app runs in, and what the app wrote to it.
// The app answers file dialogs from the dialogs folder, so a save lands in
// dialogs/saved, a folder choice is dialogs/folder and an open reads from
// dialogs/open. Each server has a scratch folder of its own, named by its project
import { copyFile, mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { servers } from "./servers.mjs";

// Scratch is the folder for one server
export class Scratch {
  constructor(server) {
    this.home = path.resolve(import.meta.dirname, servers[server].home);
    this.dialogs = path.join(this.home, "dialogs");
  }

  // reset empties the dialogs folder, so a test starts with nothing saved,
  // nothing to open and every question answered yes
  async reset() {
    await rm(this.dialogs, { recursive: true, force: true });
    await mkdir(path.join(this.dialogs, "open"), { recursive: true });
  }

  // offer puts a file where an open dialog will find it, from text or a path
  async offer(name, contents) {
    await mkdir(path.join(this.dialogs, "open"), { recursive: true });
    const dest = path.join(this.dialogs, "open", name);
    if (typeof contents === "string") await writeFile(dest, contents);
    else await copyFile(contents.path, dest);
  }

  // decline makes every question the app asks answer no
  async decline() {
    await writeFile(path.join(this.dialogs, "confirm-no"), "");
  }

  saved = () => this.#list("saved");
  chosenFolder = () => this.#list("folder");
  savedPath = (name) => path.join(this.dialogs, "saved", name);
  folderPath = (name = "") => path.join(this.dialogs, "folder", name);

  async #list(folder) {
    try {
      return (await readdir(path.join(this.dialogs, folder))).sort();
    } catch {
      return [];
    }
  }

  // events lists what the app did with dialogs and links, as [kind, detail] pairs
  async events() {
    try {
      const raw = await readFile(path.join(this.dialogs, "events.log"), "utf8");
      return raw.trim().split("\n").filter(Boolean).map((line) => line.split("\t"));
    } catch {
      return [];
    }
  }
}

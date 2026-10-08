import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { test, expect } from "../../bridge-fixtures.mjs";
import { scenes } from "../../bridge/scenes.mjs";

// Every scene is scanned with axe-core in both themes for the violations that
// matter most, serious and critical ones. A known violation sits in
// a11y-allowlist.json with the reason it is still there, and the list is expected
// to shrink: an entry that no longer matches fails the test until it is removed,
// unless it is marked optional, which means the platform's own drawing decides
// whether it appears. An entry names the rule, the target selector exactly as axe
// reports it, and the scenes it holds in, or * for all of them. Setting
// A11Y_DUMP writes what a run found to .scratch/a11y-found.json, for building the
// list
// The row of a list is named by its position, which is not part of what a rule
// says about it
const generic = (target) => target.replace(/tbody\[data-id="\d+"\]/g, "tbody");

// What axe finds depends on how the browser draws the page, so the scan is taken
// in Chromium
test.skip(({ browserName }) => browserName !== "chromium", "the scan is taken in Chromium");

const allowlist = JSON.parse(await readFile(path.join(import.meta.dirname, "../../a11y-allowlist.json"), "utf8"));
const used = new Set();
const dump = new Map();

for (const theme of ["dark", "light"]) {
  for (const [name, scene] of Object.entries(scenes)) {
    test(`${name} in the ${theme} theme has no serious accessibility violations`, async ({ bridge, page }) => {
      bridge.reply("Settings.Get", { theme });
      await scene.run({ bridge, page });
      await page.evaluate(() => document.fonts.ready);
      const { violations } = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "best-practice"]).analyze();

      const found = [];
      for (const v of violations.filter((v) => ["serious", "critical"].includes(v.impact))) {
        for (const node of v.nodes) {
          const target = generic(node.target.join(" "));
          dump.set(`${v.id}\t${target}`, { rule: v.id, target, impact: v.impact, help: v.help, scene: name, theme });
          const known = allowlist.findIndex((a) => a.rule === v.id && target === a.target && (a.scenes === "*" || a.scenes.includes(name)));
          if (known >= 0) used.add(known);
          else found.push(`${v.id} (${v.impact}) at ${target}: ${v.help}`);
        }
      }
      if (process.env.A11Y_DUMP) {
        await mkdir(".scratch", { recursive: true });
        await writeFile(".scratch/a11y-found.json", JSON.stringify([...dump.values()], null, 1));
        return;
      }
      expect(found, "violations not on the allowlist").toEqual([]);
    });
  }
}

test("every allowlisted violation is still found", async () => {
  test.skip(!!process.env.A11Y_DUMP);
  // The scenes run in their own tests before this one, in one worker
  const stale = allowlist.filter((a, i) => !a.optional && !used.has(i)).map((a) => `${a.rule} at ${a.target}`);
  expect(stale, "allowlist entries that match nothing: remove them").toEqual([]);
});

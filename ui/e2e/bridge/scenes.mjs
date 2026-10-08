// The states of the page that the visual and accessibility tests look at. Each
// scene sets the replies it needs on a bridge, boots the page and leaves it in
// its state, with nothing in it that changes from one run to the next except
// what its mask names
import { expect } from "@playwright/test";
import { ListPage } from "../pages/ListPage.mjs";
import { OverridesPage } from "../pages/OverridesPage.mjs";
import { SettingsDialog } from "../pages/SettingsDialog.mjs";
import { SinglePage } from "../pages/SinglePage.mjs";
import { card, done, result } from "./cards.mjs";

const ELF = {
  ManaCost: "{G}",
  TypeLine: "Creature — Elf Druid",
  OracleText: "{T}: Add {G}.",
  FlavorText: "Roots first, then everything else.",
  Power: "1",
  Toughness: "1",
  Colors: ["G"],
  Artist: "Anson Maddocks",
  SetCode: "dom",
  CollectorNumber: "168",
};

const row = (line, text, qty = 1, extra) => ({ line, text, qty, name: text, ...extra });

// A list of rows in every state a row can be in, resolved and shown in full
async function resolvedList(bridge, page) {
  const rows = [row(1, "Lightning Bolt", 4), row(2, "Sol Ring"), row(3, "Llanowar Elves", 2), row(4, "Delver of Secrets"), row(5, "Nonexistent Card", 3, { group: "Sideboard" })];
  const bolt = card("Lightning Bolt", { ManaCost: "{R}", TypeLine: "Instant", SetCode: "2x2", CollectorNumber: "117", Colors: ["R"] });
  const ring = card("Sol Ring", { SetCode: "c21", CollectorNumber: "263" });
  const ringAlt = card("Sol Ring", { SetCode: "cmr", CollectorNumber: "472" });
  const delver = card("Delver of Secrets // Insectile Aberration", { SetCode: "isd", CollectorNumber: "51", Layout: "transform", shapes: [{ role: "transform_front", kind: "standard" }, { role: "transform_back", kind: "standard" }] });
  bridge.reply("List.Resolve", { jobId: "r1", format: "quantity", rows });
  await bridge.boot();
  const list = new ListPage(page);
  await list.show();
  await list.paste(rows.map((r) => `${r.qty} ${r.text}`).join("\n"));
  await page.locator("#list-resolve").click();
  await expect.poll(() => bridge.calledWith("List.Resolve").length).toBe(1);
  await bridge.job("r1", [
    { seq: 1, frac: 0.2, row: { index: 0, status: "matched", card: bolt } },
    { seq: 2, frac: 0.4, row: { index: 1, status: "ambiguous", candidates: [ring, ringAlt] } },
    { seq: 3, frac: 0.6, row: { index: 2, status: "error", note: "the lookup timed out" } },
    { seq: 4, frac: 0.8, row: { index: 3, status: "matched", card: delver } },
    { seq: 5, frac: 1, row: { index: 4, status: "notFound" } },
    done(6),
  ]);
  await expect(page.locator("#list-progress")).toBeHidden();
  await list.chip("All").click();
  return list;
}

const runView = {
  id: "run-1",
  label: "Pasted list",
  outDir: "/home/user/proxies",
  template: "normal",
  dpi: 274,
  format: "jpeg",
  concurrency: 4,
  started: "2026-10-08T12:00:00Z",
  finished: "2026-10-08T12:00:07Z",
  report: "mimic-run-20261008-120000.json",
  warmupMs: 210,
  warmupCards: 4,
  cards: [
    { index: 0, name: "Lightning Bolt", status: "done", frac: 1, file: "Lightning Bolt [2X2-117].jpg", ms: 420 },
    { index: 1, name: "Sol Ring", status: "done", frac: 1, file: "Sol Ring [C21-263].jpg", ms: 390 },
    { index: 2, name: "Llanowar Elves", status: "failed", stage: "art", error: "fetching art: scryfall answered 503" },
    { index: 3, name: "Delver of Secrets", status: "unsupported", error: "No installed template renders transform front faces" },
  ],
};

// Each scene is { mask, run } where run gets the bridge and the page, and mask
// lists what a screenshot leaves out: the card image, which is a stand-in, and
// anything that shows a time
export const scenes = {
  "single-empty": {
    run: async ({ bridge }) => bridge.boot(),
  },
  "single-card": {
    mask: ["#stage", "#activity"],
    run: async ({ bridge, page }) => {
      bridge.reply("Cards.Search", [result("Llanowar Elves", ELF)]);
      bridge.reply("Cards.Printings", []);
      await bridge.boot();
      const single = new SinglePage(page);
      await single.search("llanowar elves");
      await single.select("Llanowar Elves");
      await expect.poll(() => bridge.calledWith("Render.Start").length).toBe(1);
      await bridge.job("job-1", [done(1)]);
      await expect(single.activity).toHaveAttribute("data-state", "done");
    },
  },
  "list-review": {
    run: async ({ bridge, page }) => void (await resolvedList(bridge, page)),
  },
  "list-inspector": {
    run: async ({ bridge, page }) => {
      const list = await resolvedList(bridge, page);
      await list.edit("Lightning Bolt");
    },
  },
  "run-console": {
    run: async ({ bridge, page }) => {
      bridge.reply("Run.Latest", runView);
      await bridge.boot();
      await page.locator('button[data-mode="run"]').click();
      await expect(page.locator("#run-state")).toHaveText("finished with failures");
      await page.locator("#run-rows > li", { hasText: "Llanowar Elves" }).click();
    },
  },
  "templates-library": {
    run: async ({ bridge, page }) => {
      const version = (v, extra) => ({ version: v, label: v, cached: true, selectable: true, active: false, action: "select", ...extra });
      bridge.reply("Templates.List", [
        { name: "normal", description: "The standard, modern Magic card frame", renderable: true, standard: true, versions: [version("1.1.0", { cached: false, action: "download" }), version("1.0.0", { active: true, action: "" })] },
        { name: "transform", description: "The modern frame for both faces of transform cards", renderable: true, standard: false, versions: [version("1.0.0", { cached: false, action: "download" })] },
      ]);
      await bridge.boot();
      await page.locator("#template-trigger").click();
      await expect(page.locator("#tm-library .tp-template")).toHaveCount(2);
    },
  },
  "overrides": {
    run: async ({ bridge, page }) => {
      bridge.reply("Overrides.Rules", [
        { id: "r1", name: "Big creatures", when: [{ field: "typeLine", op: "contains", value: "Creature" }], then: [{ op: "set", field: "power", value: "9" }] },
        { id: "r2", name: "", disabled: true, when: [], then: [{ op: "set", field: "artist", value: "Anonymous" }] },
      ]);
      await bridge.boot();
      await new OverridesPage(page).show();
      await expect(page.locator("#rules-list .rule-card")).toHaveCount(2);
    },
  },
  "settings": {
    run: async ({ bridge, page }) => {
      await bridge.boot();
      await new SettingsDialog(page).show();
      await expect(page.locator("#fonts-roles > li").first()).toBeVisible();
    },
  },
  "update-offer": {
    run: async ({ bridge }) => {
      bridge.reply("Updates.Status", {
        state: "available",
        current: "1.0.0",
        canInstall: true,
        release: { version: "1.1.0", notes: "Fixes", pageUrl: "https://github.com/odevine/mimic/releases/tag/ui/v1.1.0", size: 12_000_000 },
      });
      await bridge.boot();
    },
  },
};

import { test as base, expect } from "@playwright/test";
import { Fake } from "./fake.mjs";
import { Scratch } from "./scratch.mjs";
import { servers } from "./servers.mjs";
import { ListPage } from "./pages/ListPage.mjs";
import { OverridesPage } from "./pages/OverridesPage.mjs";
import { RunPage } from "./pages/RunPage.mjs";
import { SettingsDialog } from "./pages/SettingsDialog.mjs";
import { SinglePage } from "./pages/SinglePage.mjs";
import { TemplatesPage } from "./pages/TemplatesPage.mjs";

// A one pixel PNG, standing in for the card art the page asks Scryfall's image
// host for. The app fetches the real art itself through the fake upstream
const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==", "base64");

const isLocal = (url) => ["127.0.0.1", "localhost"].includes(url.hostname);

// What the app keeps for good between tests, put back after each one so tests on
// the main server do not leak into each other
async function restoreDefaults(page, defaults) {
  await page.evaluate(async (defaults) => {
    const { api } = await import("/js/api.js");
    await api.saveRules([]);
    for (const p of await api.presets()) await api.deletePreset(p.name);
    await api.saveSettings(defaults);
    for (const r of (await api.fonts()).roles) if (r.source === "user") await api.removeFont(r.folder);
  }, defaults);
}

const defaultsByServer = new Map();

// Every test gets: its server's fake upstream, reset before and after; an empty
// dialogs folder; and a guard that fails the test if the page reached for any
// host outside the machine. Tests on the main server also get its settings,
// rules, presets and fonts put back afterward
export const test = base.extend({
  server: async ({}, use, testInfo) => use(testInfo.project.metadata.server),
  fake: async ({ server }, use) => {
    const fake = new Fake(servers[server].fake);
    await fake.reset();
    await use(fake);
    await fake.reset();
  },
  scratch: async ({ server }, use) => {
    const scratch = new Scratch(server);
    await scratch.reset();
    await use(scratch);
  },
  page: async ({ page, context, server, fake, scratch }, use) => {
    const external = [];
    await context.route(
      (url) => !isLocal(url),
      (route) => {
        const url = new URL(route.request().url());
        if (url.hostname === "cards.scryfall.io") return route.fulfill({ contentType: "image/png", body: PIXEL });
        external.push(url.href);
        return route.abort();
      },
    );
    // The first test on a server finds the defaults the others are put back to
    if (server === "main" && !defaultsByServer.has(server)) {
      await page.goto("/");
      await page.locator("html.booted").waitFor();
      defaultsByServer.set(server, await page.evaluate(async () => (await import("/js/api.js")).api.settings()));
    }
    await use(page);
    expect(external, "the page reached outside the machine").toEqual([]);
    if (server === "main" && page.url().startsWith("http")) await restoreDefaults(page, defaultsByServer.get(server));
  },
  // app is the page loaded and booted, which every page object below starts from
  app: async ({ page }, use) => {
    await page.goto("/");
    await page.locator("html.booted").waitFor();
    await use(page);
  },
  // Each page object wraps the loaded page. The ones for a mode of their own,
  // list and overrides, also switch to it, and the others are shown by the test
  single: async ({ app }, use) => use(new SinglePage(app)),
  list: async ({ app }, use) => {
    const list = new ListPage(app);
    await list.show();
    await use(list);
  },
  run: async ({ app }, use) => use(new RunPage(app)),
  overrides: async ({ app }, use) => {
    const overrides = new OverridesPage(app);
    await overrides.show();
    await use(overrides);
  },
  settings: async ({ app }, use) => use(new SettingsDialog(app)),
  templates: async ({ app }, use) => use(new TemplatesPage(app)),
});

export { expect };

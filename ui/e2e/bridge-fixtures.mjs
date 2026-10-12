import { test as base, expect } from "@playwright/test";
import { Bridge, loadReplies } from "./bridge.mjs";
import { installedTemplate } from "./bridge/cards.mjs";

// A page test gets a bridge on a fresh page. It sets replies, then boots the
// page, and fails if the page made a call no reply covers. The app is answered as
// one with a template installed, since a fresh app has nothing to render with and the
// tests that cover that say so with noTemplate
export const test = base.extend({
  bridge: async ({ page }, use) => {
    const bridge = new Bridge(page, await loadReplies());
    await bridge.install();
    bridge.reply("Templates.Active", installedTemplate);
    const external = [];
    await page.route((url) => !["app.test", "127.0.0.1", "localhost"].includes(url.hostname), (route) => {
      external.push(route.request().url());
      return route.abort();
    });
    await use(bridge);
    expect([...new Set(bridge.unhandled)], "the page made calls the bridge has no reply for").toEqual([]);
    expect(external, "the page reached outside the machine").toEqual([]);
  },
});

// noTemplate answers as a fresh app does, with nothing installed to render with
export const noTemplate = (bridge) => bridge.reply("Templates.Active", bridge.defaults["Templates.Active"]);

export { expect };

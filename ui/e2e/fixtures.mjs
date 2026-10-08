import { test as base, expect } from "@playwright/test";
import { SinglePage } from "./pages/SinglePage.mjs";
import { resetDialogs } from "./scratch.mjs";

// Each test gets the Single page, opened on a fresh load, and an empty dialogs
// folder. The app and its settings are shared across the run
export const test = base.extend({
  single: async ({ page }, use) => {
    await resetDialogs();
    const single = new SinglePage(page);
    await single.open();
    await use(single);
  },
});

export { expect };

import { expect } from "@playwright/test";

// RunPage is the run console: progress of a batch, its rows, and its controls
export class RunPage {
  constructor(page) {
    this.page = page;
    this.body = page.locator("#run-body");
    this.empty = page.locator("#run-empty");
    this.title = page.locator("#run-title");
    this.state = page.locator("#run-state");
    this.meta = page.locator("#run-meta");
    this.summary = page.locator("#run-summary");
    this.count = page.locator("#run-count");
    this.stopButton = page.locator("#run-stop");
    this.retryButton = page.locator("#run-retry");
    this.openFolder = page.locator("#run-open");
    this.rows = page.locator("#run-rows > li");
    this.filterBox = page.locator("#run-filter");
    this.logToggle = page.locator("#run-log-toggle");
    this.log = page.locator("#run-log");
  }

  // show opens the console, which a started run does by itself
  async show() {
    await this.page.locator('button[data-mode="run"]').click();
  }

  // chip is one of the All, Done and Failed chips
  chip(label) {
    return this.page.locator("#run-chips button", { hasText: label });
  }

  row(name) {
    return this.rows.filter({ hasText: name });
  }

  // waitForState waits for the state pill, such as running, finished or stopped
  async waitForState(state, timeout = 60_000) {
    await expect(this.state).toHaveText(state, { timeout });
  }
}

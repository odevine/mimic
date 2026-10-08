import { expect } from "@playwright/test";

// OverridesPage is the Overrides tab of Templates: global rules that change
// fields on every card, in order, and presets that bundle them with settings
export class OverridesPage {
  constructor(page) {
    this.page = page;
    this.chip = page.locator("#overrides-chip");
    this.rules = page.locator("#rules-list .rule-card");
    this.status = page.locator("#rules-status");
    this.error = page.locator("#rules-error");
    this.add = page.locator("#rule-add");
    this.disableAll = page.locator("#rules-disable-all");
    this.presetTrigger = page.locator("#preset-trigger");
    this.presetLabel = page.locator("#preset-label");
  }

  async show() {
    await this.page.locator('button[data-mode="templates"]').click();
    await this.page.locator("#tm-tab-overrides").click();
    await expect(this.add).toBeVisible();
  }

  // waitUntilKept waits until the app has stored n rules. Edits are stored after
  // a short pause, so a test that reloads the page waits for this first
  async waitUntilKept(n) {
    await expect
      .poll(() => this.page.evaluate(async () => (await (await import("/js/api.js")).api.rules()).length))
      .toBe(n);
  }

  // addRule adds a rule and returns it
  async addRule() {
    const before = await this.rules.count();
    await this.add.click();
    await expect(this.rules).toHaveCount(before + 1);
    return this.rules.nth(before);
  }

  // when and then are a rule's condition and action rows
  conditions(rule) {
    return rule.locator(".rule-block", { has: this.page.locator(".kw", { hasText: "When" }) }).locator(".rule-line");
  }

  actions(rule) {
    return rule.locator(".rule-block", { has: this.page.locator(".kw", { hasText: "Then" }) }).locator(".rule-line");
  }
}

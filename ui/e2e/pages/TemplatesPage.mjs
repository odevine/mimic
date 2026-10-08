import { expect } from "@playwright/test";

// TemplatesPage is the Templates mode: the catalog library, the template each
// kind of face uses, and the overrides tab
export class TemplatesPage {
  constructor(page) {
    this.page = page;
    this.trigger = page.locator("#template-trigger");
    this.label = page.locator("#template-label");
    this.library = page.locator("#tm-library");
    this.libraryList = page.locator("#tm-library-list");
    this.defaults = page.locator("#face-templates");
  }

  async show() {
    await this.trigger.click();
    await expect(this.library).toBeVisible();
  }

  async showDefaults() {
    await this.page.locator("#tm-tab-defaults").click();
    await expect(this.defaults).toBeVisible();
  }

  // entry is one template of the library, by name
  entry(name) {
    return this.libraryList.locator(":scope > *", { has: this.page.locator("text=" + name) }).first();
  }
}

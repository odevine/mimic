import { expect } from "@playwright/test";

// EditorPage is the Editor mode: the templates being built, the card they are
// tested with, and what is drawn. Methods wait for what they start
export class EditorPage {
  constructor(page) {
    this.page = page;
    this.empty = page.locator("#ed-empty");
    this.workspace = page.locator("#ed-workspace");
    this.picker = page.locator("#ed-draft-select");
    this.cardLine = page.locator("#ed-card-line");
    this.presets = page.locator("#ed-presets .chip");
    this.searchInput = page.locator("#ed-search-input");
    this.results = page.locator("#ed-results .result");
    this.preview = page.locator("#ed-img");
    this.activity = page.locator("#ed-activity");
    this.aboutName = page.locator("#ed-about-name");
    this.aboutId = page.locator("#ed-about-id");
    this.aboutFork = page.locator("#ed-about-fork");
    this.layouts = page.locator("#ed-layouts li");
    this.problems = page.locator("#ed-problems li");
    this.dialog = page.locator("#draft-dialog");
    this.dialogName = page.locator("#draft-name");
    this.dialogDpi = page.locator("#draft-dpi");
    this.dialogError = page.locator("#draft-error");
    this.dialogConfirm = page.locator("#draft-confirm");
  }

  // show switches to the Editor
  async show() {
    await this.page.locator('button[data-mode="editor"]').click();
    await expect(this.page.locator("#mode-editor")).toBeVisible();
  }

  // waitForPreview waits until the strip says the card was drawn and the image has
  // decoded
  async waitForPreview() {
    await expect(this.activity).toHaveAttribute("data-state", "done", { timeout: 60_000 });
    await expect.poll(() => this.preview.evaluate((img) => img.complete && img.naturalWidth > 0)).toBe(true);
  }

  // openNewDialog opens the dialog for a new template from wherever the editor
  // offers it
  async openNewDialog() {
    const button = this.page.locator("#ed-empty-new, #ed-new").filter({ visible: true }).first();
    await button.click();
    await expect(this.dialog).toBeVisible();
  }

  // create makes a blank template and waits for it to be drawn
  async create(name, dpi) {
    await this.openNewDialog();
    await this.dialogName.fill(name);
    if (dpi !== undefined) await this.dialogDpi.fill(String(dpi));
    await this.dialogConfirm.click();
    await expect(this.dialog).toBeHidden();
    await this.waitForPreview();
  }

  preset(label) {
    return this.page.locator("#ed-presets .chip", { hasText: label });
  }

  async search(query) {
    await this.searchInput.fill(query);
    await this.searchInput.press("Enter");
    await expect(this.results.first()).toBeVisible();
  }
}

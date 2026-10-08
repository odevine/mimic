import { expect } from "@playwright/test";

// SinglePage is the Single mode: search for a card, edit its fields, render a
// preview and save the PNG. Methods wait for what they start, so a test reads as
// the steps a person takes
export class SinglePage {
  constructor(page) {
    this.page = page;
    this.searchInput = page.locator("#search-input");
    this.results = page.locator("#results .result:not(.recent)");
    this.resultsCount = page.locator("#results-count");
    this.editor = page.locator("#editor");
    this.title = page.locator("#editor-title");
    this.preview = page.locator("#preview-img");
    this.activity = page.locator("#activity");
    this.renderButton = page.locator("#render-btn");
    this.saveButton = page.locator("#save-btn");
    this.printing = page.locator("#printing-trigger");
  }

  // show switches to Single mode, which is where the page starts
  async show() {
    await this.page.locator('button[data-mode="single"]').click();
    await expect(this.searchInput).toBeVisible();
  }

  // field is the input for one editor field, by its key such as name or flavor.
  // The list inspector builds the same fields with the same ids, so the lookup
  // starts from the editor's own form
  field(key) {
    return this.page.locator(`#editor-form #f-${key}`);
  }

  // fieldRow is the whole row of a field, which carries the dirty mark
  fieldRow(key) {
    return this.page.locator(`#editor-form .field[data-field="${key}"]`);
  }

  async search(query) {
    await this.searchInput.fill(query);
    await this.searchInput.press("Enter");
    // Either the cards or the note that none matched replaces the recents
    await expect(this.page.locator("#results .result:not(.recent), #results .results-note")).not.toHaveCount(0);
    await expect(this.resultsCount).not.toHaveText("Searching…");
  }

  // select opens a result by the card name it shows
  async select(name) {
    await this.results.filter({ has: this.page.locator(".r-name", { hasText: name }) }).first().click();
    await expect(this.editor).toBeVisible();
  }

  // waitForPreview waits until the preview shows a decoded image and the strip
  // reports the render finished
  async waitForPreview() {
    await expect(this.activity).toHaveAttribute("data-state", "done", { timeout: 60_000 });
    await expect.poll(() => this.preview.evaluate((img) => img.complete && img.naturalWidth > 0)).toBe(true);
  }

  async render() {
    await this.renderButton.click();
    await this.waitForPreview();
  }

  async save() {
    await this.saveButton.click();
  }
}

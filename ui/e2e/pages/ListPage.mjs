import { expect } from "@playwright/test";

// ListPage is the From a list mode: paste a list, resolve it against Scryfall,
// review and edit the rows, choose where the cards go and start a run
export class ListPage {
  constructor(page) {
    this.page = page;
    this.input = page.locator("#list-input");
    this.format = page.locator("#list-format");
    this.detected = page.locator("#list-detected");
    this.summary = page.locator("#list-summary");
    this.filterBox = page.locator("#list-filter");
    this.none = page.locator("#list-none");
    this.rows = page.locator("#list-table-el tbody.row-group");
    this.actions = page.locator("#list-actions");
    this.outputLabel = page.locator("#list-output-label");
    this.renderButton = page.locator("#list-render");
    this.renderLabel = page.locator("#list-render-label");
    this.skipNote = page.locator("#list-skip-note");
    this.inspector = page.locator("#list-inspector");
  }

  async show() {
    await this.page.locator('button[data-mode="list"]').click();
    await expect(this.input).toBeVisible();
  }

  // paste fills the list box
  async paste(text) {
    await this.input.fill(text);
  }

  // resolve looks every line up and waits until none is still resolving
  async resolve() {
    await this.page.locator("#list-resolve").click();
    await expect(this.page.locator("#list-review-body")).toBeVisible();
    await expect(this.page.locator("#list-progress")).toBeHidden({ timeout: 30_000 });
    await expect(this.rows.locator(".pill", { hasText: "resolving" })).toHaveCount(0);
  }

  // row is one row of the review table, by the card name it shows
  row(name) {
    return this.rows.filter({ has: this.page.locator(".row-name, .row-text", { hasText: name }) }).first();
  }

  status(name) {
    return this.row(name).locator("td").last().locator(".pill, .status-pill, [class*=pill]").first();
  }

  // chip is one of the filter chips above the table, such as "Unsupported"
  chip(label) {
    return this.page.locator("#list-chips button", { hasText: label });
  }

  async filter(text) {
    await this.filterBox.fill(text);
  }

  // check ticks or unticks a row for rendering
  checkbox(name) {
    return this.row(name).locator('input[type="checkbox"]');
  }

  // edit opens the inspector on a row
  async edit(name) {
    await this.row(name).getByRole("button", { name: `Edit ${name}` }).click();
    await expect(this.inspector).toBeVisible();
  }

  inspectorField(key) {
    return this.page.locator(`#inspector-form #f-${key}`);
  }

  // bulk runs an item of the Actions menu, such as "Set a field…", which acts on
  // the checked rows
  async bulk(item) {
    await this.actions.click();
    await this.page.locator(".bulk-menu .menu-item", { hasText: item }).click();
  }

  // bulkPanel is the small form some items open, with a field, a value and Set
  get bulkPanel() {
    return this.page.locator(".bulk-panel");
  }

  qty(name) {
    return this.row(name).locator("td.num").first();
  }

  // chooseOutput picks the output folder, which the test server answers
  async chooseOutput() {
    await this.page.locator("#list-output").click();
    await expect(this.outputLabel).not.toHaveText(/Choose a folder/);
  }

  async render() {
    await this.renderButton.click();
  }
}

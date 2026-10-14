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
    this.canvas = page.locator("#ed-canvas");
    this.overlay = page.locator("#ed-overlay");
    this.selectionBox = page.locator("#ed-overlay .ed-box.sel");
    this.handles = page.locator("#ed-overlay .ed-handle");
    this.guides = page.locator("#ed-overlay .ed-guide");
    this.layers = page.locator("#ed-layers .ed-layer");
    this.selectedRow = page.locator("#ed-layers .ed-layer.selected");
    this.selection = page.locator("#ed-selection");
    this.selectionName = page.locator("#ed-selection-name");
    this.undoButton = page.locator("#ed-undo");
    this.redoButton = page.locator("#ed-redo");
    this.levers = page.locator("#ed-levers");
    this.jsonButton = page.locator("#ed-json");
    this.jsonPane = page.locator("#ed-json-pane");
    this.jsonText = page.locator("#ed-json-text");
    this.jsonTabs = page.locator("#ed-json-tabs [role=tab]");
    this.jsonStatus = page.locator("#ed-json-status");
    this.jsonProblems = page.locator("#ed-json-problems li");
    this.images = page.locator("#ed-images");
    this.cells = page.locator("#ed-images .ed-cell");
    this.addButton = page.locator("#ed-add");
    this.addDialog = page.locator("#ed-add-dialog");
    this.addKind = page.locator("#ed-add-kind");
    this.addDraws = page.locator("#ed-add-draws");
    this.addAfter = page.locator("#ed-add-after");
    this.addPart = page.locator("#ed-add-part");
    this.addPartName = page.locator("#ed-add-partname");
    this.addSlot = page.locator("#ed-add-slot");
    this.addError = page.locator("#ed-add-error");
    this.addConfirm = page.locator("#ed-add-confirm");
    this.fonts = page.locator("#ed-fonts .ed-font");
    this.fontAdd = page.locator("#ed-font-add");
    this.roles = page.locator("#ed-roles select");
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

  // sectionToggle is the heading of one of the right column's sections, which opens
  // and collapses it, by its name: layers, selection, layout, about or problems
  sectionToggle(name) {
    return this.page.locator(`#ed-sec-${name}-toggle`);
  }

  sectionBody(name) {
    return this.page.locator(`#ed-sec-${name}-body`);
  }

  // openSection shows a section that is collapsed, and leaves one that is open
  async openSection(name) {
    const toggle = this.sectionToggle(name);
    if ((await toggle.getAttribute("aria-expanded")) !== "true") await toggle.click();
    await expect(this.sectionBody(name)).toBeVisible();
  }

  // collapseSection folds an open section down to its heading
  async collapseSection(name) {
    const toggle = this.sectionToggle(name);
    if ((await toggle.getAttribute("aria-expanded")) === "true") await toggle.click();
    await expect(this.sectionBody(name)).toBeHidden();
  }

  // The canvas section is in the left column, under the template, and opens and
  // collapses the same way
  get canvasToggle() {
    return this.page.locator("#ed-canvas-toggle");
  }

  get canvasBody() {
    return this.page.locator("#ed-canvas-body");
  }

  async openCanvas() {
    if ((await this.canvasToggle.getAttribute("aria-expanded")) !== "true") await this.canvasToggle.click();
    await expect(this.canvasBody).toBeVisible();
  }

  async collapseCanvas() {
    if ((await this.canvasToggle.getAttribute("aria-expanded")) === "true") await this.canvasToggle.click();
    await expect(this.canvasBody).toBeHidden();
  }

  // row is the row of the layers list for an item, by the name it shows. The toggles,
  // the grip and the menu are in it
  row(name) {
    return this.layers.filter({ has: this.page.locator(".ed-layer-name", { hasText: new RegExp(`^${name}$`) }) });
  }

  lockToggle(name) {
    return this.row(name).locator('[data-toggle="locked"]');
  }

  snapToggle(name) {
    return this.row(name).locator('[data-toggle="snap"]');
  }

  grip(name) {
    return this.row(name).locator(".ed-layer-grip");
  }

  moreButton(name) {
    return this.row(name).locator('[data-menu="more"]');
  }

  menuItem(label) {
    return this.page.locator('.popover [role="menuitem"]').filter({ hasText: new RegExp(`^${label}$`) });
  }

  // choose opens an item's menu and picks an entry of it
  async choose(name, label) {
    await this.moreButton(name).click();
    await this.menuItem(label).click();
  }

  // lever is the control of a field of the selection, by its id, such as lever-fontSize for the selection, canvas-width for the layout or template-name for the template
  lever(id) {
    return this.page.locator(`#${id}`);
  }

  leverError(id) {
    return this.page.locator(`#${id}`).locator("xpath=following-sibling::p[contains(@class,'ed-lever-error')]");
  }

  // sizeCanvas gives the card image the size of a real preview, since a page test
  // answers with a one pixel stand-in, and returns where the overlay is on screen
  async sizeCanvas(width = 400) {
    await this.preview.evaluate((img, w) => {
      img.style.width = `${w}px`;
      img.style.height = `${(w * 1400) / 1000}px`;
      img.style.maxWidth = "none";
      img.style.maxHeight = "none";
    }, width);
    return this.overlay.boundingBox();
  }

  // at is the place on screen of a point on the finished image, given its pixels and
  // the overlay's box
  static at(box, x, y, canvas = { width: 1000, height: 1400 }) {
    return { x: box.x + (x / canvas.width) * box.width, y: box.y + (y / canvas.height) * box.height };
  }
}

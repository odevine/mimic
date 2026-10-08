import { expect } from "@playwright/test";

// SettingsDialog is the settings panel. A change is kept when Save is pressed,
// and the update controls act at once
export class SettingsDialog {
  constructor(page) {
    this.page = page;
    this.dialog = page.locator("#settings-dialog");
    this.theme = page.locator("#theme-select");
    this.imageFormat = page.locator("#image-format-select");
    this.livePreview = page.locator("#live-preview");
    this.outputDir = page.locator("#output-dir-input");
    this.fontsSource = page.locator("#fonts-source");
    this.fontRoles = page.locator("#fonts-roles > li");
    this.resetFonts = page.locator("#fonts-reset");
    this.updateVersion = page.locator("#update-version");
    this.updateSummary = page.locator("#update-summary");
    this.updateNotes = page.locator("#update-notes");
    this.updateCheck = page.locator("#update-check");
    this.updateAction = page.locator("#update-action");
    this.checkUpdates = page.locator("#check-updates");
    this.strip = page.locator("#update-strip");
    this.stripText = page.locator("#update-strip-text");
    this.stripAction = page.locator("#update-strip-action");
    this.stripDismiss = page.locator("#update-strip-dismiss");
  }

  async show() {
    await this.page.locator("#settings-btn").click();
    await expect(this.dialog).toBeVisible();
  }

  async save() {
    await this.page.locator("#settings-save").click();
    await expect(this.dialog).toBeHidden();
  }

  async cancel() {
    await this.dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(this.dialog).toBeHidden();
  }

  // fontRole is the row of one role, such as Title
  fontRole(label) {
    return this.fontRoles.filter({ has: this.page.locator(".fonts-role-name", { hasText: new RegExp(`^${label}$`) }) });
  }
}

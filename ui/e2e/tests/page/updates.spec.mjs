import { test, expect } from "../../bridge-fixtures.mjs";
import { SettingsDialog } from "../../pages/SettingsDialog.mjs";

const release = { version: "1.1.0", notes: "Fixes", pageUrl: "https://github.com/odevine/mimic/releases/tag/ui/v1.1.0", size: 12_000_000 };
const available = { state: "available", current: "1.0.0", canInstall: true, release };

test("an update that can install itself goes from offered to downloading to ready, after a refusal, and a failure is said in the settings", async ({ bridge, page }) => {
  bridge.reply("Updates.Status", available);
  bridge.fail("Updates.Install", "this install cannot update itself");
  bridge.reply("Updates.Restart", null);
  await bridge.boot();
  const settings = new SettingsDialog(page);

  await expect(settings.stripText).toHaveText("mimic 1.1.0 is available");
  await expect(settings.stripAction).toHaveText("Install update");
  await settings.stripAction.click();
  await expect(page.locator(".toast.err")).toContainText("this install cannot update itself");
  await expect(settings.stripText).toHaveText("mimic 1.1.0 is available");

  bridge.reply("Updates.Install", null);
  await settings.stripAction.click();
  await expect.poll(() => bridge.calledWith("Updates.Install").length).toBe(2);

  await bridge.emit("update", { ...available, state: "downloading", written: 3_000_000, total: 12_000_000 });
  await expect(settings.stripText).toHaveText("Downloading mimic 1.1.0, 25%");
  await expect(settings.stripDismiss).toBeHidden();

  await bridge.emit("update", { ...available, state: "ready" });
  await expect(settings.stripText).toHaveText("mimic 1.1.0 is ready");
  await expect(settings.stripAction).toHaveText("Restart to update");
  await settings.stripAction.click();
  await expect.poll(() => bridge.calledWith("Updates.Restart").length).toBe(1);

  await bridge.emit("update", { ...available, state: "error", error: "checksum mismatch" });
  await settings.show();
  await expect(settings.updateSummary).toHaveText("The update failed: checksum mismatch");
});

test("a release the user dismissed stays dismissed until a newer one comes out", async ({ bridge, page }) => {
  bridge.reply("Updates.Status", available);
  bridge.reply("Settings.Get", { dismissedUpdate: "1.1.0" });
  await bridge.boot();
  const settings = new SettingsDialog(page);
  await expect(settings.strip).toBeHidden();

  await bridge.emit("update", { ...available, release: { ...release, version: "1.2.0" } });
  await expect(settings.strip).toBeVisible();
  await expect(settings.stripText).toHaveText("mimic 1.2.0 is available");
});

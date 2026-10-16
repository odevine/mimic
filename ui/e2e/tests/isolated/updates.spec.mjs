import { test, expect } from "../../fixtures.mjs";

// The app runs as version 1.0.0 here, so it checks for updates like a release.
// A test build is not a packaged app and cannot replace itself, so an update is
// offered as a link to its release page

test("with no newer release the app says it is up to date, and one that cannot be reached is reported", async ({ settings, fake }) => {
  await settings.show();
  await expect(settings.updateVersion).toHaveText("mimic 1.0.0");
  await settings.updateCheck.click();
  await expect(settings.updateSummary).toContainText(/up to date/i);
  await expect(settings.updateAction).toBeHidden();
  await expect(settings.strip).toBeHidden();

  await fake.fail("api.github.com", 503);
  await settings.updateCheck.click();
  await expect(settings.updateSummary).toContainText(/503|GitHub/);
  await expect(settings.updateSummary).toHaveClass(/err/);
});

test("a newer release is offered with its notes, opens its page, and its notice can be dismissed", async ({ settings, fake, scratch }) => {
  // A dismissed version stays dismissed, so each run offers one of its own
  const version = `1.${Date.now() % 100000}.0`;
  await fake.release(version, "Adds the thing\nFixes the other thing");
  await settings.show();
  await settings.updateCheck.click();

  await expect(settings.updateSummary).toContainText(version);
  await expect(settings.updateNotes).toContainText("Fixes the other thing");
  await expect(settings.stripText).toHaveText(`mimic ${version} is available`);
  await settings.updateAction.click();
  await expect.poll(async () => (await scratch.events()).filter(([kind]) => kind === "url").map(([, link]) => link)).toContain(
    `https://github.com/odevine/mimic/releases/tag/ui/v${version}`,
  );

  await settings.cancel();
  await settings.stripDismiss.click();
  await expect(settings.strip).toBeHidden();
});

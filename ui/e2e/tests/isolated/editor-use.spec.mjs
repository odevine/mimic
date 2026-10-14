import { test, expect } from "../../fixtures.mjs";

// These tests put a template made in the editor among the installed ones, which the app
// keeps for good, so they run on the isolated server. Each takes it out again by
// deleting the template, which is also a test of that

test("a template used from the editor is in Templates, draws a card in Single, and is taken back when it is deleted", async ({ editor, templates, single }) => {
  await editor.create("Mine", 100);
  await editor.useButton.click();
  await expect(editor.toasts.last()).toContainText("Mine is ready in Single, List and Run as user.mine");

  await templates.show();
  const entry = templates.entry("Mine");
  await expect(entry).toContainText("user.mine");
  await expect(entry).toContainText("draft (from the editor)");
  await entry.getByRole("button", { name: "Select" }).click();
  await expect(templates.label).toHaveText("Mine · draft");

  // The template is a real one, so a card renders with it
  await single.show();
  await single.search("sol ring");
  await single.select("Sol Ring");
  await single.render();
  await expect(single.preview).toBeVisible();

  // Using it again after more edits brings the template in use up to date
  await editor.show();
  await editor.useButton.click();
  await expect(editor.toasts.last()).toContainText("Mine is up to date in Single, List and Run as user.mine. It is the template in use");

  // Deleting the template takes it back out of the app, whatever was drawing with it
  await editor.page.locator("#ed-delete").click();
  await expect(editor.empty).toBeVisible();
  await templates.show();
  await expect(templates.libraryList.locator(".tp-template")).toHaveCount(0);
  await expect(templates.label).toHaveText("No template installed");
});

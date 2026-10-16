import { test, expect } from "../../bridge-fixtures.mjs";

const row = (name, title) => ({
  name,
  title,
  standard: true,
  shapes: [{ role: "single", kind: "standard" }],
  versions: [{ version: "1.2.0", label: "1.2.0", cached: true, selectable: true, active: false, action: "select" }],
});

const imported = (extra = {}) => ({ template: "jane.pinlines", version: "1.2.0", title: "Jane's pinlines", same: false, replaced: false, exists: false, active: false, ...extra });

async function ready(bridge, page) {
  bridge.reply("Templates.List", [row("mimic.m15", "Modern frame (M15)")]);
  await bridge.boot();
  await page.locator("#template-trigger").click();
  await expect(page.locator("#tm-library .tp-template")).toHaveCount(1);
}

const button = (page) => page.locator("#tm-import-template");
const toast = (page) => page.locator("#toasts .toast").last();
const imports = (bridge) => bridge.calledWith("Templates.Import").map(([req]) => req);

test("a chosen .mimic file is imported and the library read again, after a cancel, a file it cannot read and one installed already", async ({ page, bridge }) => {
  await ready(bridge, page);
  bridge.reply("System.PickFile", "");
  await button(page).click();
  await expect.poll(() => bridge.calledWith("System.PickFile").length).toBe(1);
  expect(imports(bridge)).toEqual([]);

  bridge.reply("System.PickFile", "/downloads/notes.mimic");
  bridge.fail("Templates.Import", "That file is not a template this version of mimic can read: template: reading as a bundle");
  await button(page).click();
  await expect(page.locator(".toast.err")).toContainText("not a template this version of mimic can read");
  await expect(button(page)).toBeEnabled();

  bridge.reply("Templates.Import", imported({ same: true }));
  await button(page).click();
  await expect(toast(page)).toHaveText("Jane's pinlines 1.2.0 is installed already");

  bridge.reply("System.PickFile", "/downloads/jane.pinlines-1.2.0.mimic");
  bridge.reply("Templates.Import", imported());
  const lists = bridge.calledWith("Templates.List").length;
  bridge.reply("Templates.List", [row("mimic.m15", "Modern frame (M15)"), row("jane.pinlines", "Jane's pinlines")]);
  await button(page).click();
  await expect(toast(page)).toHaveText("Imported Jane's pinlines 1.2.0");
  expect(bridge.calledWith("System.PickFile").at(-1)[0]).toMatchObject({ filters: [{ name: "mimic templates", pattern: "*.mimic" }] });
  expect(imports(bridge).at(-1)).toEqual({ path: "/downloads/jane.pinlines-1.2.0.mimic", replace: false });
  await expect(page.locator("#tm-library .tp-template")).toHaveCount(2);
  expect(bridge.calledWith("Templates.List").length).toBeGreaterThan(lists);
  // The template in use is not changed by an import
  expect(bridge.calledWith("Templates.Active")).toHaveLength(1);
});

test("a different copy of an installed version is replaced only when the question is answered yes", async ({ page, bridge }) => {
  bridge.reply("System.PickFile", "/downloads/b.mimic");
  bridge.reply("System.Confirm", false);
  bridge.reply("Templates.Import", (req) => (req.replace ? imported({ replaced: true, active: true }) : imported({ exists: true })));
  await ready(bridge, page);
  await button(page).click();
  await expect.poll(() => bridge.calledWith("System.Confirm").length).toBe(1);
  const [ask] = bridge.calledWith("System.Confirm")[0];
  expect(ask).toMatchObject({ title: "Replace the installed template?", accept: "Replace" });
  expect(ask.message).toContain("Jane's pinlines 1.2.0 is installed already");
  // A no leaves what is installed alone
  expect(imports(bridge).map((r) => r.replace)).toEqual([false]);
  await expect(page.locator("#toasts .toast")).toHaveCount(0);

  bridge.reply("System.Confirm", true);
  await button(page).click();
  await expect(toast(page)).toHaveText("Replaced Jane's pinlines 1.2.0");
  expect(imports(bridge).map((r) => r.replace)).toEqual([false, false, true]);
  // The template in use was opened again, so the page asks which it is
  await expect.poll(() => bridge.calledWith("Templates.Active").length).toBeGreaterThan(1);
});


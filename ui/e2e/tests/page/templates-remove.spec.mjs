import { test, expect } from "../../bridge-fixtures.mjs";

const version = (v, extra = {}) => ({ version: v, label: v, cached: true, selectable: true, active: false, action: "select", removable: true, restorable: true, ...extra });
const row = (name, title, versions) => ({ name, title, standard: true, shapes: [{ role: "single", kind: "standard" }], versions });

async function ready(bridge, page, rows) {
  bridge.reply("Templates.List", rows);
  await bridge.boot();
  await page.locator("#template-trigger").click();
  await expect(page.locator("#tm-library .tp-template")).toHaveCount(rows.length);
}

const removeButton = (page, name) => page.getByRole("button", { name: `Remove ${name}` });
const removals = (bridge) => bridge.calledWith("Templates.Remove").map(([req]) => req);
const toast = (page) => page.locator("#toasts .toast").last();

test("removing asks first and says whether the version can come back, and a yes removes it or says why it could not", async ({ page, bridge }) => {
  await ready(bridge, page, [
    row("mimic.m15", "Modern frame (M15)", [version("1.2.0")]),
    row("jane.pinlines", "Jane's pinlines", [version("1.0.0", { restorable: false })]),
    row("user.mine", "Mine", [version("draft", { label: "draft (from the editor)", restorable: false })]),
  ]);
  bridge.reply("System.Confirm", false);
  const asked = async (name) => {
    const before = bridge.calledWith("System.Confirm").length;
    await removeButton(page, name).click();
    await expect.poll(() => bridge.calledWith("System.Confirm").length).toBe(before + 1);
    return bridge.calledWith("System.Confirm")[before][0];
  };
  const catalog = await asked("Modern frame (M15) 1.2.0");
  expect(catalog).toMatchObject({ title: "Remove Modern frame (M15) 1.2.0?", accept: "Remove" });
  expect(catalog.message).toContain("can be downloaded again");
  const imported = await asked("Jane's pinlines 1.0.0");
  expect(imported.message).toContain("only from the file it was imported from");
  const draft = await asked("Mine draft (from the editor)");
  expect(draft.message).toContain("The draft stays in the Editor");
  // A no deletes nothing
  expect(removals(bridge)).toEqual([]);
  await expect(page.locator("#toasts .toast")).toHaveCount(0);

  // A yes removes the version, reads the library again and says so
  bridge.reply("System.Confirm", true);
  bridge.reply("Templates.Remove", { template: "jane.pinlines", version: "1.0.0", active: false });
  bridge.reply("Templates.List", [row("mimic.m15", "Modern frame (M15)", [version("1.2.0")])]);
  const actives = bridge.calledWith("Templates.Active").length;
  await removeButton(page, "Jane's pinlines 1.0.0").click();
  await expect(toast(page)).toHaveText("Removed Jane's pinlines 1.0.0");
  expect(removals(bridge)).toEqual([{ template: "jane.pinlines", version: "1.0.0" }]);
  await expect(page.locator("#tm-library .tp-template")).toHaveCount(1);
  // Nothing in use changed, so the page did not ask what is
  expect(bridge.calledWith("Templates.Active")).toHaveLength(actives);

  // A removal that fails says why and the library is read again
  bridge.fail("Templates.Remove", "Templates cannot change while a run is in progress");
  const lists = bridge.calledWith("Templates.List").length;
  await removeButton(page, "Modern frame (M15) 1.2.0").click();
  await expect(page.locator(".toast.err")).toContainText("while a run is in progress");
  await expect.poll(() => bridge.calledWith("Templates.List").length).toBeGreaterThan(lists);
});

test("removing the template in use says it switches, and the page asks what is in use now", async ({ page, bridge }) => {
  bridge.reply("System.Confirm", true);
  bridge.reply("Templates.Remove", { template: "mimic.m15", version: "1.2.0", active: true, now: "mimic.placeholder" });
  await ready(bridge, page, [row("mimic.m15", "Modern frame (M15)", [version("1.2.0", { active: true, action: "" })])]);
  const actives = bridge.calledWith("Templates.Active").length;
  await removeButton(page, "Modern frame (M15) 1.2.0").click();
  const [ask] = bridge.calledWith("System.Confirm")[0];
  expect(ask.message).toContain("It is the template in use");
  await expect(toast(page)).toHaveText("Removed Modern frame (M15) 1.2.0");
  await expect.poll(() => bridge.calledWith("Templates.Active").length).toBeGreaterThan(actives);
});


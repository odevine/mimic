import { test, expect } from "../fixtures.mjs";

// A rule that gives every creature a power of 9
async function creaturesGetPowerNine(overrides) {
  const rule = await overrides.addRule();
  await rule.getByLabel("Rule name").fill("Big creatures");
  const when = overrides.conditions(rule).first();
  await when.getByLabel("Field").selectOption("typeLine");
  await when.getByPlaceholder("text").fill("Creature");
  const then = overrides.actions(rule).first();
  await then.getByLabel("Field").selectOption("power");
  await then.getByPlaceholder("new value").fill("9");
  return rule;
}

test("a global rule changes the fields of the cards it matches and leaves the others", async ({ overrides, single, page }) => {
  await creaturesGetPowerNine(overrides);
  await expect(overrides.status).toHaveText("1 rule, 1 on");
  await expect(overrides.chip).toContainText("1 rule on");

  await single.show();
  await page.locator("#search-input").fill("llanowar elves");
  await page.locator("#search-input").press("Enter");
  await page.locator("#results .result:not(.recent)").first().click();
  await expect(page.locator('#f-single-power')).toHaveValue("9");
  await expect(page.locator('#editor-form .field[data-field="power"]')).toHaveClass(/ruled/);

  await page.locator("#search-input").fill("sol ring");
  await page.locator("#search-input").press("Enter");
  await page.locator("#results .result:not(.recent)").first().click();
  await expect(page.locator('#f-single-power')).toHaveValue("");
});

test("rules are kept across a reload and can all be switched off", async ({ overrides, page }) => {
  await creaturesGetPowerNine(overrides);
  await expect(overrides.status).toHaveText("1 rule, 1 on");
  await overrides.waitUntilKept(1);

  await page.reload();
  await page.locator("html.booted").waitFor();
  await expect(overrides.chip).toContainText("1 rule on");

  await page.locator('button[data-mode="templates"]').click();
  await page.locator("#tm-tab-overrides").click();
  await expect(overrides.rules).toHaveCount(1);
  await overrides.disableAll.click();
  await expect(overrides.status).toHaveText("1 rule, 0 on");
  await expect(overrides.chip).toBeHidden();
});

test("a preset keeps the rules and brings them back", async ({ overrides, page }) => {
  await creaturesGetPowerNine(overrides);
  await overrides.waitUntilKept(1);

  await overrides.presetTrigger.click();
  await page.getByLabel("Preset name").fill("Big creatures");
  await page.locator(".preset-save").getByRole("button", { name: "Save" }).click();
  await expect(overrides.presetLabel).toHaveText("Big creatures");

  // Deleting the rule leaves the preset, which puts it back
  await overrides.rules.first().getByLabel("Delete this rule").click();
  await expect(overrides.rules).toHaveCount(0);
  await overrides.waitUntilKept(0);

  await overrides.presetTrigger.click();
  await page.locator(".preset-menu").getByText("Big creatures").click();
  await expect(overrides.rules).toHaveCount(1);
  await expect(overrides.rules.first().getByLabel("Rule name")).toHaveValue("Big creatures");
  await expect(overrides.chip).toContainText("1 rule on");
});

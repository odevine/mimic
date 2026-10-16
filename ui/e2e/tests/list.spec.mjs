import { test, expect } from "../fixtures.mjs";

const DECK = ["4 Lightning Bolt", "1 Sol Ring", "2 Llanowar Elves", "Sideboard", "1 Delver of Secrets", "3 Nonexistent Card"].join("\n");

test("a list is detected, resolved, sorted into matched, unsupported and not found, and narrowed by chips and the filter", async ({ list }) => {
  await list.paste(DECK);
  await list.resolve();

  await expect(list.detected).toHaveText("Detected Quantity prefix.");
  await expect(list.summary).toHaveText("5 rows · 3 matched · 1 not found · 1 unsupported");
  await expect(list.row("Lightning Bolt")).toContainText("2X2 117");
  // The placeholder template draws no transform faces, so the card is flagged
  await expect(list.row("Delver of Secrets")).toContainText("unsupported");
  await expect(list.row("Nonexistent Card")).toContainText("not found");
  await expect(list.row("Delver of Secrets")).toContainText("Sideboard");
  // Rows that cannot render are skipped and the button counts what will
  await expect(list.renderLabel).toHaveText("Render 3 cards");
  await expect(list.skipNote).toContainText("1 row needs attention and 1 card unsupported");

  await list.chip("Sideboard").click();
  await expect(list.row("Delver of Secrets")).toBeVisible();
  await expect(list.row("Sol Ring")).toBeHidden();
  await list.chip("All").click();
  await list.filter("elves");
  await expect(list.row("Llanowar Elves")).toBeVisible();
  await expect(list.row("Sol Ring")).toBeHidden();
  await list.filter("zzz");
  await expect(list.none).toBeVisible();
});

test("the checked rows are rendered, given a field or a quantity, and removed", async ({ list }) => {
  await list.paste("4 Lightning Bolt\n1 Sol Ring\n2 Llanowar Elves");
  await list.resolve();
  await expect(list.renderLabel).toHaveText("Render 3 cards");
  await list.checkbox("Sol Ring").uncheck();
  await expect(list.renderLabel).toHaveText("Render 2 cards");

  await list.bulk("Set a field");
  await list.bulkPanel.getByLabel("Field").selectOption("artist");
  await list.bulkPanel.getByLabel("Value").fill("Test Artist");
  await list.bulkPanel.getByRole("button", { name: "Set" }).click();
  await expect(list.row("Lightning Bolt")).toContainText("artist set on this row");
  await expect(list.row("Llanowar Elves")).toContainText("artist set on this row");
  await expect(list.row("Sol Ring")).not.toContainText("set on this row");

  await list.checkbox("Sol Ring").check();
  await list.checkbox("Llanowar Elves").uncheck();
  await list.bulk("Set quantity");
  await list.bulkPanel.getByRole("spinbutton").fill("7");
  await list.bulkPanel.getByRole("button", { name: "Set" }).click();
  await expect(list.qty("Lightning Bolt")).toHaveText("7");
  await expect(list.qty("Sol Ring")).toHaveText("7");
  await expect(list.qty("Llanowar Elves")).toHaveText("2");

  await list.bulk("Remove 2 checked rows");
  await expect(list.rows).toHaveCount(1);
  await expect(list.summary).toHaveText("1 row · 1 matched");
});

test("rows can be ticked and unticked by what they are", async ({ list }) => {
  await list.paste(["1 Lightning Bolt", "Sideboard", "1 Sol Ring", "1 Nonexistent Card"].join("\n"));
  await list.resolve();
  await expect(list.renderLabel).toHaveText("Render 2 cards");

  // A list with a row that needs attention opens on that filter, and the bulk
  // items act on the rows shown, so every row is shown first
  await expect(list.chip("Needs attention")).toHaveAttribute("aria-pressed", "true");
  await list.chip("All").click();
  await list.bulk("Untick the rows shown");
  await expect(list.renderButton).toBeDisabled();
  await list.bulk("Tick Sideboard");
  await expect(list.renderLabel).toHaveText("Render 1 card");
  await expect(list.checkbox("Sol Ring")).toBeChecked();
  await expect(list.checkbox("Lightning Bolt")).not.toBeChecked();
});

test("a list file chosen in the dialog fills the box, and a row edited in the inspector changes its card", async ({ list, scratch }) => {
  await scratch.offer("deck.txt", "2 Llanowar Elves\n");
  await list.page.locator("#list-open").click();
  await expect(list.input).toHaveValue("2 Llanowar Elves\n");
  await list.resolve();
  await list.edit("Llanowar Elves");

  await expect(list.inspectorField("name")).toHaveValue("Llanowar Elves");
  await list.inspectorField("power").fill("5");
  await expect(list.inspector.locator('.field[data-field="power"]')).toHaveClass(/dirty/);
  await expect(list.row("Llanowar Elves")).toContainText("power set on this row");
});

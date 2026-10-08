import { readdir } from "node:fs/promises";
import path from "node:path";
import { test, expect } from "../fixtures.mjs";

// The launch check is a page of the app with scripted flows, which runs in each
// real webview on CI. Loading it here, in headless browsers against the test
// server, keeps its flows from going stale between those runs
test("the launch check's flows pass against the test server", async ({ page, scratch }) => {
  test.setTimeout(120_000);
  const out = path.join(scratch.home, "smoke-out");
  await page.goto(`/smoke.html?e2e&out=${encodeURIComponent(out)}`);
  await page.waitForFunction(() => window.__smoke, null, { timeout: 90_000 });
  const verdict = await page.evaluate(() => window.__smoke);

  expect(verdict.detail).toContain("a run wrote Smoke Elf.jpg");
  expect(verdict.ok, verdict.detail).toBe(true);
  expect((await readdir(out)).filter((f) => f.endsWith(".jpg"))).toEqual(["Smoke Elf.jpg"]);
});

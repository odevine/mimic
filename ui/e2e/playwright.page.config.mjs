import { defineConfig } from "@playwright/test";
import { pageProjects, visualProjects } from "./playwright.config.mjs";
import { snapshotDir } from "./visual.mjs";

// The page, accessibility and visual tests alone, with no server to build or start
export default defineConfig({
  testDir: "tests",
  outputDir: "test-results",
  workers: 4,
  reporter: [["list"], ["html", { outputFolder: "report", open: "never" }]],
  use: { trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [...pageProjects, ...visualProjects],
  snapshotPathTemplate: `${snapshotDir}/{arg}{ext}`,
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.002 } },
});

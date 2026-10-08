import { defineConfig } from "@playwright/test";
import { pageProjects } from "./playwright.config.mjs";

// The page tests alone, with no server to build or start
export default defineConfig({
  testDir: "tests",
  outputDir: "test-results",
  workers: 4,
  reporter: [["list"], ["html", { outputFolder: "report", open: "never" }]],
  use: { trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: pageProjects,
});

import { defineConfig, devices } from "@playwright/test";

const port = 4173;
const baseURL = `http://127.0.0.1:${port}`;

// One server and one scratch folder serve the whole run, so the tests share the
// app's state and run one at a time. The server builds the app with the server
// tag, starts a fake Scryfall, and prints its address once the page answers
export default defineConfig({
  testDir: "tests",
  outputDir: "test-results",
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: [["list"], ["html", { outputFolder: "report", open: "never" }]],
  use: {
    baseURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 820 } } },
    { name: "webkit", use: { ...devices["Desktop Safari"], viewport: { width: 1280, height: 820 } } },
  ],
  webServer: {
    command: `go run ../cmd/testserver -port ${port} -home .scratch/home -clean -log .scratch/server.log`,
    url: baseURL,
    reuseExistingServer: false,
    timeout: 240_000,
    stdout: "ignore",
    stderr: "pipe",
  },
});

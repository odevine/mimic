import { defineConfig, devices } from "@playwright/test";

import { servers } from "./servers.mjs";
import { snapshotDir, visualEnabled } from "./visual.mjs";

// The test servers run for the whole run, described in servers.mjs. Each project
// names the server its tests use in its metadata
const webServer = ({ port, fake, home }, name) => ({
  command: `go run ../cmd/testserver -port ${port} -fake-port ${fake} -version 1.0.0 -home ${home} -clean -log .scratch/${name}.log`,
  url: `http://127.0.0.1:${port}`,
  reuseExistingServer: false,
  timeout: 240_000,
  stdout: "ignore",
  stderr: "pipe",
});

export const browsers = {
  chromium: devices["Desktop Chrome"],
  webkit: devices["Desktop Safari"],
};

// The page tests run the frontend against a bridge in the test and need no
// server, so they have projects of their own that a config without servers shares
export const pageProjects = Object.entries(browsers).map(([browser, device]) => ({
  name: `${browser}-page`,
  testMatch: /tests\/page\/.*\.spec\.mjs$/,
  use: { ...device, viewport: { width: 1280, height: 820 } },
}));

const projects = Object.entries(browsers).flatMap(([browser, device]) =>
  [
    { name: browser, server: "main", testMatch: /tests\/[^/]+\.spec\.mjs$/, retries: process.env.CI ? 1 : 0 },
    // The isolated tests change what the app keeps for good, so a second try would
    // meet the first one's leftovers
    { name: `${browser}-isolated`, server: `${browser}-isolated`, testMatch: /tests\/isolated\/.*\.spec\.mjs$/, retries: 0 },
  ].map(({ name, server, ...rest }) => ({
    name,
    ...rest,
    metadata: { server },
    use: { ...device, viewport: { width: 1280, height: 820 }, baseURL: `http://127.0.0.1:${servers[server].port}` },
  })),
);

// The visual tests also run from a bridge and need no server. They run only where
// the baselines are made, so a screenshot is never compared across platforms
export const visualProjects = visualEnabled
  ? [
      {
        name: "chromium-visual",
        testMatch: /tests\/visual\/.*\.spec\.mjs$/,
        use: { ...browsers.chromium, viewport: { width: 1280, height: 820 }, deviceScaleFactor: 1 },
      },
    ]
  : [];

// One server and one scratch folder serve each group of tests, so the tests in a
// group run one at a time
export default defineConfig({
  testDir: "tests",
  outputDir: "test-results",
  workers: 1,
  reporter: [["list"], ["html", { outputFolder: "report", open: "never" }]],
  use: {
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [...projects, ...pageProjects, ...visualProjects],
  snapshotPathTemplate: `${snapshotDir}/{arg}{ext}`,
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.002 } },
  webServer: Object.entries(servers).map(([name, server]) => webServer(server, name)),
});

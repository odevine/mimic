// Regenerates bridge/replies.json from the running test server, so the replies a
// page test starts from are what the real app answers. Run it through
// `task test:e2e:capture` after a change to what the bridge returns
import { writeFile } from "node:fs/promises";
import path from "node:path";
import { spawn } from "node:child_process";
import { chromium } from "@playwright/test";
import { CALLS } from "./calls.mjs";

const server = spawn("go", ["run", "../cmd/testserver", "-clean", "-home", ".scratch/capture-home", "-log", ".scratch/capture-server.log", "-exit-on-stdin-close", "-version", "1.0.0"], {
  cwd: path.resolve(import.meta.dirname, ".."),
  stdio: ["pipe", "pipe", "inherit"],
});
const url = await new Promise((resolve, reject) => {
  let out = "";
  server.stdout.on("data", (chunk) => {
    out += chunk;
    const m = out.match(/listening at (http:\/\/\S+)/);
    if (m) resolve(m[1]);
  });
  server.on("exit", (code) => reject(new Error(`the test server exited with ${code}`)));
});

const browser = await chromium.launch();
try {
  const page = await browser.newPage();
  await page.goto(url);
  await page.locator("html.booted").waitFor();
  const replies = {};
  for (const [name, ...args] of CALLS) {
    replies[name] = await page.evaluate(async ([name, args]) => {
      const [service, method] = name.split(".");
      const { Call } = await import("/wails/runtime.js");
      const prefix = service === "Jobs" ? "github.com/odevine/mimic/ui/internal/jobs.Service." : `github.com/odevine/mimic/ui/internal/desktop.${service}.`;
      return Call.ByName(prefix + method, ...args);
    }, [name, args]);
  }
  const out = path.join(import.meta.dirname, "replies.json");
  await writeFile(out, JSON.stringify(replies, null, 2) + "\n");
  console.log(`wrote ${Object.keys(replies).length} replies to ${out}`);
} finally {
  await browser.close();
  server.stdin.end();
  await new Promise((resolve) => server.on("exit", resolve));
}

// Takes a screenshot of the page in headless Chromium against the test server
// and prints where the PNG landed, so an agent can look at the page without
// controlling a window. Usage:
//
//   node shot.mjs [--search "lightning bolt"] [--select "Lightning Bolt"] [--render]
//                 [--theme dark|light] [--size 1280x820] [--out shots/name.png]
//
// --search runs a search, --select opens a result, and --render renders the
// preview and waits for it. The run uses the same fake Scryfall as the tests
import { spawn } from "node:child_process";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { parseArgs } from "node:util";
import { chromium } from "@playwright/test";
import { SinglePage } from "./pages/SinglePage.mjs";

const { values: o } = parseArgs({
  options: {
    search: { type: "string" },
    select: { type: "string" },
    render: { type: "boolean", default: false },
    theme: { type: "string", default: "dark" },
    size: { type: "string", default: "1280x820" },
    out: { type: "string", default: "shots/page.png" },
  },
});

// go run does not pass a signal on to the program it runs, so the server quits
// when its standard input closes instead
const server = spawn("go", ["run", "../cmd/testserver", "-clean", "-home", ".scratch/shot-home", "-log", ".scratch/shot-server.log", "-exit-on-stdin-close"], {
  cwd: import.meta.dirname,
  stdio: ["pipe", "pipe", "inherit"],
});
const stop = () => server.stdin.end();

const url = await new Promise((resolve, reject) => {
  let out = "";
  server.stdout.on("data", (chunk) => {
    out += chunk;
    const m = out.match(/listening at (http:\/\/\S+)/);
    if (m) resolve(m[1]);
  });
  server.on("exit", (code) => reject(new Error(`the test server exited with ${code} before it was ready`)));
});

const [width, height] = o.size.split("x").map(Number);
const browser = await chromium.launch();
try {
  const context = await browser.newContext({ viewport: { width, height }, colorScheme: o.theme === "light" ? "light" : "dark" });
  const page = await context.newPage();
  await page.addInitScript((theme) => localStorage.setItem("mimic.theme", theme), o.theme);
  const single = new SinglePage(page);
  await page.goto(url);
  await page.locator("html.booted").waitFor();
  if (o.search) await single.search(o.search);
  if (o.select) await single.select(o.select);
  if (o.render) await single.render();
  const out = path.resolve(import.meta.dirname, o.out);
  await mkdir(path.dirname(out), { recursive: true });
  await page.screenshot({ path: out });
  console.log(out);
} finally {
  await browser.close();
  stop();
  await new Promise((resolve) => server.on("exit", resolve));
}

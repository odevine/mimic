// A bridge for page tests: the real frontend in a browser with no Go process
// behind it. The page's calls into the app are answered from replies a test
// sets, and its events are sent by the test. This is for states the real app
// makes hard to reach, such as a failed render or an update that errors, and
// the replies start from what the real app answers (bridge/replies.json).
import { readFile } from "node:fs/promises";
import path from "node:path";

const FRONTEND = path.resolve(import.meta.dirname, "../frontend");
export const ORIGIN = "http://app.test";
const NS = "github.com/odevine/mimic/ui/internal/";

const TYPES = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png", ".json": "application/json" };

// The Wails runtime the page imports, reduced to the two things it uses. A call
// goes to the test through the function the bridge exposes, and an event is sent
// by the test through window.__emit
const RUNTIME = `
const handlers = new Map();
window.__emit = (name, data) => { for (const cb of handlers.get(name) || []) cb({ name, data }); };
export const Events = {
  On(name, cb) {
    if (!handlers.has(name)) handlers.set(name, new Set());
    handlers.get(name).add(cb);
    return () => handlers.get(name).delete(cb);
  },
};
export const Call = {
  ByName(name, ...args) {
    const call = window.__bridgeCall(name, args).then((r) => {
      if (!r.ok) throw new Error(r.message);
      return r.value;
    });
    call.cancel = () => {};
    return call;
  },
};
`;

// A one pixel PNG for every image the page asks the app for
const PIXEL = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==", "base64");

export class Bridge {
  // calls is every call the page made, as { name, args }, and unhandled the names
  // it made that no reply covers
  calls = [];
  unhandled = [];
  #replies = new Map();
  // jobEvents is what each job has reported, which the page asks for again when
  // it joins a job late, as the app's backlog does
  #jobEvents = new Map();

  constructor(page, replies) {
    this.page = page;
    // defaults is what the real app answered, for a reply that changes one part
    this.defaults = replies;
    for (const [name, value] of Object.entries(replies)) this.#replies.set(name, () => value);
    // No rule changes a card unless a test says so
    this.reply("Overrides.Apply", () => ({ fields: {}, applied: [] }));
    // Saving settings answers with what was saved, as the app does
    this.reply("Settings.Put", (settings) => settings);
    // Each render starts a job of its own, numbered from one
    let jobs = 0;
    this.reply("Render.Start", () => ({ jobId: `job-${++jobs}`, dpi: 274 }));
    this.reply("jobs.Service.Backlog", (id, seq) => {
      const events = this.#jobEvents.get(id) || [];
      return { events: events.filter((e) => e.seq > seq), finished: events.some((e) => e.done) };
    });
  }

  // reply answers a call, such as "Settings.Get", with a value, or with the result
  // of a function given the call's arguments. A function may throw to reject
  reply(name, valueOrFn) {
    this.#replies.set(name, typeof valueOrFn === "function" ? valueOrFn : () => valueOrFn);
    return this;
  }

  // fail makes a call reject with a message, as the app does with an error
  fail(name, message) {
    return this.reply(name, () => {
      throw new Error(message);
    });
  }

  // calledWith lists the arguments of every call to name
  calledWith(name) {
    return this.calls.filter((c) => c.name === name).map((c) => c.args);
  }

  // emit sends the page a window event
  emit(name, data) {
    return this.page.evaluate(([n, d]) => window.__emit(n, d), [name, data]);
  }

  // job sends a job's events, as the app batches them
  async job(id, events) {
    this.#jobEvents.set(id, [...(this.#jobEvents.get(id) || []), ...events]);
    await this.emit("job", { id, events });
  }

  async install() {
    await this.page.exposeFunction("__bridgeCall", async (full, args) => {
      const name = full.startsWith(NS) ? full.slice(NS.length).replace(/^desktop\./, "") : full;
      this.calls.push({ name, args });
      const reply = this.#replies.get(name);
      if (!reply) {
        this.unhandled.push(name);
        return { ok: false, message: `the bridge has no reply for ${name}` };
      }
      try {
        return { ok: true, value: await reply(...args) };
      } catch (err) {
        return { ok: false, message: err.message };
      }
    });
    await this.page.route(`${ORIGIN}/**`, async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: RUNTIME });
      if (url.pathname.startsWith("/img/")) return route.fulfill({ contentType: "image/png", body: PIXEL });
      const file = path.join(FRONTEND, url.pathname === "/" ? "index.html" : url.pathname);
      if (!file.startsWith(FRONTEND)) return route.fulfill({ status: 403 });
      try {
        return route.fulfill({ contentType: TYPES[path.extname(file)] || "application/octet-stream", body: await readFile(file) });
      } catch {
        return route.fulfill({ status: 404, body: "not found" });
      }
    });
  }

  // boot loads the page and waits for it to finish booting
  async boot(hash = "") {
    await this.page.goto(`${ORIGIN}/${hash}`);
    await this.page.locator("html.booted").waitFor();
  }
}

// replies is what the real app answered when the fixtures were captured
export async function loadReplies() {
  return JSON.parse(await readFile(path.join(import.meta.dirname, "bridge/replies.json"), "utf8"));
}

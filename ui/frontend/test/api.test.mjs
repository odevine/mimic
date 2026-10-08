import { register } from "node:module";
import test from "node:test";
import assert from "node:assert/strict";

register("./loader.mjs", import.meta.url);
const { handlers } = await import("./runtime-stub.mjs");
const { watchJob } = await import("../js/api.js");

const emit = (id, events) => handlers.job({ data: { id, events } });

test("a watcher is never called before watchJob returns", async () => {
  // Events that arrive before anyone watches, as with a fast render
  emit("1", [{ seq: 1, step: "Fetching", frac: 0.1 }, { seq: 2, step: "Drawing", frac: 0.9 }, { seq: 3, done: true }]);

  // The caller's callback names the promise watchJob returns, as the single
  // card render does, which fails if the callback runs first
  let renderJob = null;
  const steps = [];
  const job = (renderJob = watchJob("1", (step) => {
    if (renderJob === job) steps.push(step);
  }));
  const done = await job;
  assert.deepEqual(steps, ["Fetching", "Drawing"]);
  assert.equal(done.done, true);
});

test("an event is applied once however it arrives", async () => {
  const heard = [];
  const p = watchJob("2", (step) => heard.push(step));
  emit("2", [{ seq: 1, step: "a" }]);
  await Promise.resolve();
  emit("2", [{ seq: 1, step: "a" }, { seq: 2, step: "b" }, { seq: 3, done: true }]);
  await p;
  assert.deepEqual(heard, ["a", "b"]);
});

test("a job that ends with an error rejects", async () => {
  emit("3", [{ seq: 1, done: true, error: "no template" }]);
  await assert.rejects(watchJob("3"), /no template/);
});

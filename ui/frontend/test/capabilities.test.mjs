import { register } from "node:module";
import test, { beforeEach } from "node:test";
import assert from "node:assert/strict";

register("./loader.mjs", import.meta.url);
const { app } = await import("../js/state.js");
const { gateFor, isLive } = await import("../js/capabilities.js");

beforeEach(() => {
  app.capabilities.value = {};
});

test("a key the app lists answers with its own gate", () => {
  app.capabilities.value = {
    "flow.single": { state: "live" },
    "flow.mpc": { state: "needs-template", reason: "Install a template that draws MPC bleed" },
  };
  assert.equal(isLive("flow.single"), true);
  assert.equal(isLive("flow.mpc"), false);
  assert.equal(gateFor("flow.mpc").reason, "Install a template that draws MPC bleed");
});

test("a key the app does not list reads as planned, so a typo shows up as a gate", () => {
  const g = gateFor("flow.typo");
  assert.equal(g.state, "planned");
  assert.ok(g.reason);
  assert.equal(isLive("flow.typo"), false);
});

test("gates follow the map when it changes", () => {
  assert.equal(isLive("flow.list"), false);
  app.capabilities.value = { "flow.list": { state: "live" } };
  assert.equal(isLive("flow.list"), true);
});

import test from "node:test";
import assert from "node:assert/strict";

// The helpers read navigator.platform once, at load, so each case sets it first
// and imports a fresh copy of the module
async function load(platform) {
  Object.defineProperty(globalThis, "navigator", { value: { platform }, configurable: true });
  return import(`../js/dom.js?platform=${encodeURIComponent(platform)}`);
}

test("the command modifier is meta on a Mac and control elsewhere", async () => {
  const mac = await load("MacIntel");
  assert.equal(mac.isMac, true);
  assert.equal(mac.modKey({ metaKey: true, ctrlKey: false }), true);
  assert.equal(mac.modKey({ metaKey: false, ctrlKey: true }), false);

  const win = await load("Win32");
  assert.equal(win.isMac, false);
  assert.equal(win.modKey({ metaKey: false, ctrlKey: true }), true);
  assert.equal(win.modKey({ metaKey: true, ctrlKey: false }), false);
});

test("single-key shortcuts stay out of text controls", async () => {
  const { isTyping } = await load("Win32");
  assert.equal(isTyping({ tagName: "TEXTAREA" }), true);
  assert.equal(isTyping({ tagName: "SELECT" }), true);
  assert.equal(isTyping({ tagName: "DIV", isContentEditable: true }), true);
  assert.equal(isTyping({ tagName: "INPUT", type: "text" }), true);
  assert.equal(isTyping({ tagName: "INPUT", type: "search" }), true);
  assert.equal(isTyping({ tagName: "INPUT", type: "checkbox" }), false);
  assert.equal(isTyping({ tagName: "INPUT", type: "button" }), false);
  assert.equal(isTyping({ tagName: "BUTTON" }), false);
  assert.equal(isTyping(null), false);
});

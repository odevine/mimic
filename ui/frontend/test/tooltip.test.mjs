import test, { beforeEach, afterEach, mock } from "node:test";
import assert from "node:assert/strict";

// A just-enough document for the tooltip: listeners to fire, the tooltip box
// and elements that answer closest() the way the page's do
const listeners = {};
let popoverOpen = false;
const box = {
  textContent: "",
  replaceChildren() { this.textContent = ""; },
  append(t) { this.textContent += typeof t === "string" ? t : t.textContent; },
  matches: (sel) => sel === ":popover-open" && popoverOpen,
  showPopover() { popoverOpen = true; },
  hidePopover() { popoverOpen = false; },
  getBoundingClientRect: () => ({ width: 100, height: 20 }),
  style: {},
};
globalThis.document = {
  getElementById: (id) => (id === "tooltip" ? box : null),
  createElement: () => ({ className: "", textContent: "" }),
  addEventListener: (type, fn) => { listeners[type] = fn; },
  documentElement: { addEventListener: (type, fn) => { listeners["root:" + type] = fn; } },
};
globalThis.window = { innerHeight: 800, innerWidth: 1200, addEventListener() {} };

const { initTooltips, hideTooltip } = await import("../js/components/tooltip.js");

// tipped is an element carrying a tip, plain one carries none
const tipped = (text) => {
  const el = { dataset: { tip: text }, isConnected: true, getBoundingClientRect: () => ({ bottom: 10, top: 0, left: 0, width: 50 }), matches: () => true };
  el.closest = () => el;
  return el;
};
const plain = () => ({ closest: () => null });
const over = (el) => listeners.mouseover({ target: el });
const shownText = () => (popoverOpen ? box.textContent : null);

let initialised = false;

beforeEach(() => {
  mock.timers.enable({ apis: ["setTimeout"] });
  popoverOpen = false;
  if (!initialised) initTooltips();
  initialised = true;
  hideTooltip();
});

afterEach(() => mock.timers.reset());

test("a tip shows after the delay and goes when the pointer moves off", () => {
  const a = tipped("A tip");
  over(a);
  assert.equal(shownText(), null, "before the delay");
  mock.timers.tick(400);
  assert.equal(shownText(), "A tip");
  over(plain());
  assert.equal(shownText(), null);
});

test("leaving before the delay for somewhere with no tip shows nothing", () => {
  over(tipped("A tip"));
  mock.timers.tick(100);
  over(plain());
  mock.timers.tick(1000);
  assert.equal(shownText(), null, "the tip of the element the pointer left appeared");
});

test("leaving before the delay for another tipped element shows only that one", () => {
  over(tipped("A tip"));
  mock.timers.tick(100);
  over(tipped("B tip"));
  mock.timers.tick(300);
  assert.equal(shownText(), null, "B's delay has not ended, and A's must not fire");
  mock.timers.tick(100);
  assert.equal(shownText(), "B tip");
});

test("staying on an element does not restart the delay", () => {
  const a = tipped("A tip");
  over(a);
  mock.timers.tick(200);
  over(a);
  mock.timers.tick(200);
  assert.equal(shownText(), "A tip");
});

test("the pointer leaving the window cancels a pending tip", () => {
  over(tipped("A tip"));
  mock.timers.tick(100);
  listeners["root:mouseleave"]();
  mock.timers.tick(1000);
  assert.equal(shownText(), null);
});

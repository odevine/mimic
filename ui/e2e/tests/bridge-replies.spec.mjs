import { test, expect } from "../fixtures.mjs";
import { CALLS } from "../bridge/calls.mjs";
import { loadReplies } from "../bridge.mjs";

// The page tests start from replies captured from the real app. This test asks
// the real app the same questions and checks every field a captured reply holds
// is still there with the same kind of value, so a change to what the app sends
// does not leave the page tests running on a story the app no longer tells.
// Run test:e2e:capture to refresh the replies after such a change

const kind = (v) => (v === null ? "null" : Array.isArray(v) ? "array" : typeof v);

// problems lists where a captured reply holds something the real one does not
function problems(captured, real, at) {
  if (captured === null || real === null) return [];
  if (kind(captured) !== kind(real)) return [`${at}: captured a ${kind(captured)}, the app sent a ${kind(real)}`];
  if (Array.isArray(captured)) {
    // An empty list says nothing about its items
    return captured.length && real.length ? problems(captured[0], real[0], `${at}[]`) : [];
  }
  if (kind(captured) === "object") {
    return Object.keys(captured).flatMap((key) => (key in real ? problems(captured[key], real[key], `${at}.${key}`) : [`${at}.${key}: the app no longer sends it`]));
  }
  return [];
}

test("the replies the page tests start from still match what the app sends", async ({ app }) => {
  const captured = await loadReplies();
  const found = [];
  for (const [name, ...args] of CALLS) {
    const real = await app.evaluate(async ([name, args]) => {
      const [service, method] = name.split(".");
      const { Call } = await import("/wails/runtime.js");
      return Call.ByName(`github.com/odevine/mimic/ui/internal/desktop.${service}.${method}`, ...args);
    }, [name, args]);
    found.push(...problems(captured[name], real, name));
  }
  expect(found, "the captured replies are stale: run test:e2e:capture").toEqual([]);
});

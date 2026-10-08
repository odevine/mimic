import test from "node:test";
import assert from "node:assert/strict";
import { signal, effect, computed, batch } from "../js/state.js";

test("an effect runs at once and again when a signal it read changes", () => {
  const n = signal(1);
  const seen = [];
  effect(() => seen.push(n.value));
  n.value = 2;
  n.value = 3;
  assert.deepEqual(seen, [1, 2, 3]);
});

test("setting a signal to the value it holds runs nothing", () => {
  const n = signal(1);
  let runs = 0;
  effect(() => {
    n.value;
    runs++;
  });
  n.value = 1;
  assert.equal(runs, 1);
});

test("peek reads without subscribing", () => {
  const n = signal(1);
  let runs = 0;
  effect(() => {
    n.peek();
    runs++;
  });
  n.value = 2;
  assert.equal(runs, 1);
});

test("an effect follows the signals its latest run read", () => {
  const useA = signal(true);
  const a = signal("a");
  const b = signal("b");
  const seen = [];
  effect(() => seen.push(useA.value ? a.value : b.value));
  b.value = "b2";
  assert.deepEqual(seen, ["a"], "b was not read yet");
  useA.value = false;
  a.value = "a2";
  assert.deepEqual(seen, ["a", "b2"], "a is no longer read");
});

test("a disposed effect stops running", () => {
  const n = signal(0);
  let runs = 0;
  const stop = effect(() => {
    n.value;
    runs++;
  });
  stop();
  n.value = 1;
  assert.equal(runs, 1);
});

test("a computed follows what it derives from", () => {
  const n = signal(2);
  const double = computed(() => n.value * 2);
  assert.equal(double.value, 4);
  n.value = 5;
  assert.equal(double.value, 10);
  assert.equal(double.peek(), 10);
});

test("an effect reading a computed runs when its source changes", () => {
  const n = signal(1);
  const label = computed(() => `n=${n.value}`);
  const seen = [];
  effect(() => seen.push(label.value));
  n.value = 2;
  assert.deepEqual(seen, ["n=1", "n=2"]);
});

test("batch runs each dependent effect once after all the sets", () => {
  const a = signal(1);
  const b = signal(1);
  const seen = [];
  effect(() => seen.push(a.value + b.value));
  batch(() => {
    a.value = 2;
    b.value = 3;
  });
  assert.deepEqual(seen, [2, 5]);
});

test("nested batches flush when the outermost one ends", () => {
  const n = signal(0);
  const seen = [];
  effect(() => seen.push(n.value));
  batch(() => {
    batch(() => {
      n.value = 1;
    });
    assert.deepEqual(seen, [0], "the inner batch has not flushed");
    n.value = 2;
  });
  assert.deepEqual(seen, [0, 2]);
});

test("a batch that throws still flushes and rethrows", () => {
  const n = signal(0);
  const seen = [];
  effect(() => seen.push(n.value));
  assert.throws(() =>
    batch(() => {
      n.value = 1;
      throw new Error("boom");
    }), /boom/);
  assert.deepEqual(seen, [0, 1]);
});

test("an effect that throws on its first run does not leak its tracking into later reads", () => {
  const n = signal(0);
  assert.throws(() =>
    effect(() => {
      throw new Error("first run");
    }), /first run/);
  // Had the throwing effect stayed the running one, this read would subscribe it
  n.value;
  const seen = [];
  effect(() => seen.push(n.value));
  n.value = 1;
  assert.deepEqual(seen, [0, 1]);
});

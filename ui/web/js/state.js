// A small reactive store. A signal holds one value, an effect re-runs whenever a
// signal it read changes, and a computed is a read-only signal derived from
// others. DOM updates stay explicit in the effects, so a change touches only the
// elements that read it and never re-renders a whole region

let running = null;
let batchDepth = 0;
const pending = new Set();

function schedule(effect) {
  if (batchDepth > 0) pending.add(effect);
  else effect.run();
}

export function signal(initial) {
  let value = initial;
  const subs = new Set();
  return {
    get value() {
      if (running) {
        subs.add(running);
        running.deps.add(subs);
      }
      return value;
    },
    set value(next) {
      if (Object.is(next, value)) return;
      value = next;
      for (const e of [...subs]) schedule(e);
    },
    // peek reads without subscribing, for handlers that act on the current value
    peek() {
      return value;
    },
  };
}

export function effect(fn) {
  const e = {
    deps: new Set(),
    run() {
      for (const subs of e.deps) subs.delete(e);
      e.deps.clear();
      const prev = running;
      running = e;
      try {
        fn();
      } finally {
        running = prev;
      }
    },
  };
  e.run();
  return () => {
    for (const subs of e.deps) subs.delete(e);
    e.deps.clear();
  };
}

export function computed(fn) {
  const s = signal(undefined);
  effect(() => {
    s.value = fn();
  });
  return {
    get value() {
      return s.value;
    },
    peek: () => s.peek(),
  };
}

// batch defers effects until fn returns, so setting several signals together
// runs each dependent effect once
export function batch(fn) {
  batchDepth++;
  try {
    fn();
  } finally {
    batchDepth--;
    if (batchDepth === 0) {
      const effects = [...pending];
      pending.clear();
      for (const e of effects) e.run();
    }
  }
}

// app is the global part of the store: what every mode shares. Each flow keeps
// its own store beside its module
export const app = {
  mode: signal("single"),
  capabilities: signal({}),
  settings: signal({ theme: "dark", expandPrintings: false, splits: {} }),
  template: signal(null), // { name, version, label, source }
  resolution: signal(null),
  status: signal("Ready."),
  // progress drives the status bar's compact bar, null when idle
  progress: signal(null),
};

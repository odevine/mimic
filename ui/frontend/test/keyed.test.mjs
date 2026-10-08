import test from "node:test";
import assert from "node:assert/strict";
import { keyedRows } from "../js/components/keyed.js";

// An element reduced to what keyedRows touches: it can leave its container
// and replace itself with others
function container() {
  const kids = [];
  const c = {
    kids,
    append: (...els) => {
      for (const e of els) {
        e.parent = c;
        kids.push(e);
      }
    },
  };
  return c;
}
const element = (id) => {
  const e = {
    id,
    hidden: false,
    parent: null,
    remove() {
      const i = e.parent.kids.indexOf(e);
      if (i >= 0) e.parent.kids.splice(i, 1);
    },
    replaceWith(...els) {
      const i = e.parent.kids.indexOf(e);
      for (const x of els) x.parent = e.parent;
      e.parent.kids.splice(i, 1, ...els);
    },
  };
  return e;
};

function setup() {
  const c = container();
  const disposed = [];
  const rows = keyedRows(c, (row) => {
    const el = element(row.id);
    return { el, dispose: () => disposed.push(row.id) };
  });
  return { c, rows, disposed };
}

const ids = (c) => c.kids.map((e) => e.id);

test("reset builds one element per row, in order", () => {
  const { c, rows } = setup();
  rows.reset([{ id: "a" }, { id: "b" }, { id: "c" }]);
  assert.deepEqual(ids(c), ["a", "b", "c"]);
  assert.equal(rows.el("b").id, "b");
  assert.equal(rows.el("zzz"), null);
});

test("reset drops the old rows and disposes them", () => {
  const { c, rows, disposed } = setup();
  rows.reset([{ id: "a" }, { id: "b" }]);
  rows.reset([{ id: "c" }]);
  assert.deepEqual(ids(c), ["c"]);
  assert.deepEqual(disposed.sort(), ["a", "b"]);
  assert.equal(rows.el("a"), null);
});

test("replace swaps one row for several in the same place", () => {
  const { c, rows, disposed } = setup();
  rows.reset([{ id: "a" }, { id: "q" }, { id: "z" }]);
  rows.replace("q", [{ id: "q1" }, { id: "q2" }]);
  assert.deepEqual(ids(c), ["a", "q1", "q2", "z"]);
  assert.deepEqual(disposed, ["q"]);
  assert.equal(rows.el("q"), null);
});

test("replacing a row that is not there changes nothing", () => {
  const { c, rows } = setup();
  rows.reset([{ id: "a" }]);
  rows.replace("nope", [{ id: "x" }]);
  assert.deepEqual(ids(c), ["a"]);
});

test("show hides rejected rows in place and counts the rest", () => {
  const { c, rows } = setup();
  const list = [{ id: "a", n: 1 }, { id: "b", n: 2 }, { id: "c", n: 3 }];
  rows.reset(list);
  assert.equal(rows.show(list, (r) => r.n !== 2), 2);
  assert.deepEqual(c.kids.map((e) => e.hidden), [false, true, false]);
  assert.deepEqual(ids(c), ["a", "b", "c"], "nothing is removed");
  assert.equal(rows.show(list, () => true), 3);
  assert.ok(c.kids.every((e) => !e.hidden));
});

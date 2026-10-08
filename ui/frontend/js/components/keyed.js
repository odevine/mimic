// keyedRows keeps one element per row in a container, keyed by row id. Each row
// builds its element once and wires its own effects, so a change to one row
// updates that row's element and never rebuilds the list. Filtering hides
// elements in place rather than removing them

export function keyedRows(container, build) {
  const items = new Map(); // id -> { el, dispose }

  function make(row) {
    const built = build(row);
    items.set(row.id, built);
    return built.el;
  }

  function drop(id) {
    const item = items.get(id);
    if (!item) return;
    if (item.dispose) item.dispose();
    item.el.remove();
    items.delete(id);
  }

  return {
    // reset replaces every row, leaving anything else in the container alone
    reset(rows) {
      for (const id of [...items.keys()]) drop(id);
      container.append(...rows.map(make));
    },
    // replace swaps one row for several in the same place, which is how a query
    // line becomes the cards it matched
    replace(id, rows) {
      const old = items.get(id);
      if (!old) return;
      const els = rows.map(make);
      old.el.replaceWith(...els);
      if (old.dispose) old.dispose();
      items.delete(id);
    },
    el(id) {
      const item = items.get(id);
      return item ? item.el : null;
    },
    // show hides every row the predicate rejects and returns how many remain
    show(rows, keep) {
      let n = 0;
      for (const row of rows) {
        const el = items.get(row.id)?.el;
        if (!el) continue;
        const visible = keep(row);
        el.hidden = !visible;
        if (visible) n++;
      }
      return n;
    },
  };
}

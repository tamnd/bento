// A Map's keys(), values(), and entries() iterators, and its default [Symbol.iterator],
// all walk the entries in insertion order. A key updated after it was first inserted
// keeps that first position, and a deleted-then-reinserted key moves to the end, so the
// order a for...of observes is the order the entries were first added, minus deletions.
function run(): void {
  const m = new Map<string, number>();
  m.set("a", 1);
  m.set("b", 2);
  m.set("c", 3);
  m.set("a", 10);

  let keys = "";
  for (const k of m.keys()) {
    keys += k;
  }
  console.log(keys);

  let total = 0;
  for (const v of m.values()) {
    total += v;
  }
  console.log(total);

  // The default iterator yields [key, value] pairs, the same as entries().
  for (const [k, v] of m) {
    console.log(k, v);
  }

  for (const [k, v] of m.entries()) {
    console.log(k + "=" + v);
  }

  // Deleting then reinserting a key moves it to the end of the iteration order.
  m.delete("b");
  m.set("b", 20);
  let order = "";
  for (const k of m.keys()) {
    order += k;
  }
  console.log(order);
}

run();

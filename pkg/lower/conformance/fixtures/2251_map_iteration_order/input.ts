// A Map's iteration order is the order keys were first inserted, and it survives every
// mutation the spec defines. Updating an existing key leaves it in place, deleting a key
// removes it without disturbing the rest, and deleting then reinserting a key gives it a
// new position at the end. The order a for...of observes after a run of such mutations is
// exactly the surviving keys in first-insertion order, with each reinsertion at the tail.
function run(): void {
  const m = new Map<string, number>();
  m.set("a", 1);
  m.set("b", 2);
  m.set("c", 3);
  m.set("d", 4);
  m.set("e", 5);

  m.delete("b");
  m.delete("d");
  m.set("a", 100); // update keeps a's original position
  m.set("b", 6); // reinsertion appends b at the end

  let keys = "";
  for (const k of m.keys()) {
    keys += k;
  }
  console.log(keys);

  for (const [k, v] of m) {
    console.log(k, v);
  }
  console.log(m.size);
}

run();

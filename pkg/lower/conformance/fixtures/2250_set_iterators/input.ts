// A Set is its own iterable, so a for...of yields each member in insertion order, and
// its values() and keys() iterators both yield the members (a Set's key is its value).
// Its entries() yields a [value, value] pair, so a destructuring loop binds the same
// member to both names. A member re-added after it was deleted takes a new position at
// the end, matching the insertion-order guarantee.
function run(): void {
  const s = new Set<number>();
  s.add(1);
  s.add(2);
  s.add(3);
  s.add(2);

  let sum = 0;
  for (const v of s) {
    sum += v;
  }
  console.log(sum);

  let a = "";
  for (const v of s.values()) {
    a += v;
  }
  console.log(a);

  let b = "";
  for (const k of s.keys()) {
    b += k;
  }
  console.log(b);

  for (const [x, y] of s.entries()) {
    console.log(x, y);
  }

  s.delete(1);
  s.add(1);
  let order = "";
  for (const v of s) {
    order += v;
  }
  console.log(order);
}

run();

// A stored Map or Set iterator driven by hand, `const it = m.values()` followed by an
// `it.next()` loop, cannot see through to the receiver the way a for...of or spread
// does, because the iterator object is read many times and stepped one call at a time.
// It lowers to a real runtime iterator minted over the receiver's insertion-ordered
// snapshot: values() and a Set's members walk the values, keys() walks the keys, and
// each next() packs the same { value, done } result a generator's next() hands back, so
// `.value` and `.done` read off it the same way. The snapshot is taken when the iterator
// is minted, matching the moment the accessor is called.

function mapValues(m: Map<string, number>): number {
  const it = m.values();
  let total = 0;
  let r = it.next();
  while (!r.done) {
    total = total + r.value;
    r = it.next();
  }
  return total;
}

function mapKeys(m: Map<string, number>): string {
  const it = m.keys();
  let out = "";
  let r = it.next();
  while (!r.done) {
    out = out + r.value + ".";
    r = it.next();
  }
  return out;
}

function setMembers(s: Set<number>): number {
  const it = s.values();
  let total = 0;
  let r = it.next();
  while (!r.done) {
    total = total + r.value;
    r = it.next();
  }
  return total;
}

// The drive stops the moment done turns true, so a partial drive reads only the steps it
// pulls and leaves the rest of the snapshot unread. Each step is guarded on done before
// its value is read, the narrowing that makes the value present.
function firstTwo(s: Set<number>): number {
  const it = s.keys();
  let total = 0;
  const a = it.next();
  if (!a.done) {
    total = total + a.value;
  }
  const b = it.next();
  if (!b.done) {
    total = total + b.value;
  }
  return total;
}

const m = new Map<string, number>();
m.set("a", 1);
m.set("b", 2);
m.set("c", 3);
console.log(mapValues(m));
console.log(mapKeys(m));

const s = new Set<number>();
s.add(10);
s.add(20);
s.add(30);
console.log(setMembers(s));
console.log(firstTwo(s));

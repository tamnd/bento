// The ES2025 set-algebra methods accept any set-like argument, not only another Set. A
// Map is a set-like whose members are its keys, so passing a Map runs the algebra over
// its keys: the union adds the map's keys the set lacks, the intersection keeps the
// members the map also keys, and the difference drops them. The result order follows the
// same rule the two-set form uses, the receiver's members first.
function run(): void {
  const s = new Set<number>();
  s.add(1);
  s.add(2);
  s.add(3);

  const m = new Map<number, string>();
  m.set(2, "b");
  m.set(3, "c");
  m.set(4, "d");

  let u = "";
  for (const v of s.union(m)) {
    u += v;
  }
  console.log(u);

  let i = "";
  for (const v of s.intersection(m)) {
    i += v;
  }
  console.log(i);

  let d = "";
  for (const v of s.difference(m)) {
    d += v;
  }
  console.log(d);

  console.log(s.isSubsetOf(m));
  console.log(s.isDisjointFrom(m));
}

run();

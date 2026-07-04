// The ES2025 set-algebra methods combine two sets or compare them. union,
// intersection, difference, and symmetricDifference return a new set;
// isSubsetOf, isSupersetOf, and isDisjointFrom return a boolean. The results are
// probed by size and membership here, since spread over a set still waits on
// iterator lowering.
function run(): void {
  const a = new Set<number>();
  a.add(1);
  a.add(2);
  a.add(3);
  const b = new Set<number>();
  b.add(3);
  b.add(4);
  b.add(5);

  const u = a.union(b);
  console.log(u.size, u.has(1), u.has(5), u.has(6));

  const i = a.intersection(b);
  console.log(i.size, i.has(3), i.has(1));

  const d = a.difference(b);
  console.log(d.size, d.has(1), d.has(3));

  const s = a.symmetricDifference(b);
  console.log(s.size, s.has(1), s.has(3), s.has(4));

  console.log(a.isSubsetOf(b), a.isSupersetOf(b), a.isDisjointFrom(b));

  const sub = new Set<number>();
  sub.add(1);
  sub.add(2);
  console.log(sub.isSubsetOf(a), a.isSupersetOf(sub), sub.isDisjointFrom(b));
}

run();

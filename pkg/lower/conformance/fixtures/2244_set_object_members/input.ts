// A Set of objects compares members by reference identity, the SameValueZero an
// object matches under. Objects lower to Go struct pointers, so adding the same
// reference twice leaves one member, while c, a distinct object of the same shape
// as a, is not a member at all. Delete drops the member the reference names.
interface Box {
  id: number;
}

function run(): void {
  const s = new Set<Box>();
  const a: Box = { id: 1 };
  const b: Box = { id: 2 };
  const c: Box = { id: 1 };
  s.add(a);
  s.add(a);
  s.add(b);
  console.log(s.size);
  console.log(s.has(a));
  console.log(s.has(c));
  s.delete(a);
  console.log(s.has(a));
  console.log(s.size);
}

run();

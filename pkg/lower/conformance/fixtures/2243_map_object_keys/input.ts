// A Map keyed by an object compares keys by reference identity, the SameValueZero
// an object matches under. Objects lower to Go struct pointers, so two object
// literals of the same shape are distinct keys: a and c read the same but are
// different objects, so an entry stored under a leaves c absent. Storing under a a
// second time updates the entry in place rather than adding, so the size holds,
// and delete drops the one entry the reference names.
interface Box {
  id: number;
}

function run(): void {
  const m = new Map<Box, string>();
  const a: Box = { id: 1 };
  const b: Box = { id: 2 };
  const c: Box = { id: 1 };
  m.set(a, "a");
  m.set(b, "b");
  console.log(m.has(a));
  console.log(m.has(c));
  console.log(m.size);
  m.set(a, "A");
  console.log(m.size);
  m.delete(a);
  console.log(m.has(a));
  console.log(m.size);
}

run();

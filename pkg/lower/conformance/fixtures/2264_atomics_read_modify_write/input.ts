// Atomics runs read, write, and read-modify-write operations over an integer typed
// array backed by a SharedArrayBuffer. store returns the value it wrote, load reads it
// back, and add, sub, and, or, xor, and exchange each return the element from before
// the update and leave the new element in place, which the load after each confirms. In
// a single agent every operation is already indivisible, so the sequence reads exactly
// as the spec's atomic steps would.
function run(): void {
  const ta = new Int32Array(new SharedArrayBuffer(16));

  console.log(Atomics.store(ta, 0, 10));
  console.log(Atomics.load(ta, 0));

  console.log(Atomics.add(ta, 0, 5));
  console.log(Atomics.load(ta, 0));

  console.log(Atomics.sub(ta, 0, 3));
  console.log(Atomics.load(ta, 0));

  console.log(Atomics.and(ta, 0, 10));
  console.log(Atomics.load(ta, 0));

  console.log(Atomics.or(ta, 0, 1));
  console.log(Atomics.load(ta, 0));

  console.log(Atomics.xor(ta, 0, 3));
  console.log(Atomics.load(ta, 0));

  console.log(Atomics.exchange(ta, 0, 99));
  console.log(Atomics.load(ta, 0));
}

run();

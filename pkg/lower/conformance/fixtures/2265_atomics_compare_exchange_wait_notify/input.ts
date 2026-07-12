// compareExchange stores its replacement only when the element still equals the
// expected value and returns the previous element either way; isLockFree reports whether
// an atomic op on a byte size is lock-free; and wait, notify, and pause coordinate
// agents. In a single agent there is no second agent to send a notify, so a wait that
// would block reports not-equal when the element already differs and timed-out
// otherwise, notify finds no waiter and wakes zero, and pause is a no-op.
function run(): void {
  const ta = new Int32Array(new SharedArrayBuffer(16));
  Atomics.store(ta, 0, 5);

  console.log(Atomics.compareExchange(ta, 0, 5, 20));
  console.log(Atomics.load(ta, 0));
  console.log(Atomics.compareExchange(ta, 0, 5, 99));
  console.log(Atomics.load(ta, 0));

  console.log(Atomics.isLockFree(4));
  console.log(Atomics.isLockFree(3));

  const w1: string = Atomics.wait(ta, 0, 7);
  console.log(w1);
  const w2: string = Atomics.wait(ta, 0, 20, 0);
  console.log(w2);

  console.log(Atomics.notify(ta, 0));

  Atomics.pause();
  console.log("done");
}

run();

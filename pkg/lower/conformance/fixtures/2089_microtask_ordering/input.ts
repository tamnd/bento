// An async body's await continuations and a promise's then callbacks share one microtask
// queue, so they interleave in the order JavaScript fixes rather than one draining ahead
// of the other. The async body runs synchronously to its first await, printing co-start,
// then parks. Each then callback runs a turn after the synchronous run, and each await
// continuation a turn after its await, so the coroutine and the two thens step through the
// queue together: then-A, then co-1, then then-B, then co-2. This is the ordering the
// event loop from group 3 gives, checked against Node.
async function counter(): Promise<void> {
  console.log("co-start");
  await Promise.resolve(0);
  console.log("co-1");
  await Promise.resolve(0);
  console.log("co-2");
}
console.log("sync-start");
Promise.resolve(0).then((v) => console.log("then-A"));
counter();
Promise.resolve(0).then((v) => console.log("then-B"));
console.log("sync-end");

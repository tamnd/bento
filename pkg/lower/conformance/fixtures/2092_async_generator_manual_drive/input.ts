// An async generator (async function* g()) is both a generator and an async body: it
// yields values a consumer pulls and awaits promises between yields. It lowers to a Go
// coroutine the runtime drives over the event loop, where each pull returns a promise the
// consumer awaits. Driven by hand through await g.next(), each pull settles on the
// microtask queue, so the synchronous tail runs before the first value and the loop reads
// each yielded value in order, the ordering Node fixes for the awaited pulls.
async function* ag(): AsyncGenerator<number> {
  yield 1;
  await Promise.resolve(0);
  yield 2;
  yield 3;
}
async function run(): Promise<void> {
  console.log("start");
  const g = ag();
  let r = await g.next();
  while (!r.done) {
    console.log("v:" + r.value);
    r = await g.next();
  }
  console.log("end");
}
run();
console.log("sync");

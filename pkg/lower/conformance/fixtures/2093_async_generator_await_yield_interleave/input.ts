// An async generator interleaves its two kinds of suspend point: it awaits a promise
// inside the loop and yields the running accumulator each pass, then returns it. The two
// stay ordered, each await settles before the value it feeds is yielded, so the consumer
// reads the accumulator after every step and the final return value once the loop ends.
// The await between yields keeps its pull pending until the promise settles, so the yields
// still arrive one at a time and the synchronous tail runs before the first value.
async function* running(n: number): AsyncGenerator<number> {
  let acc = 0;
  for (let i = 0; i < n; i++) {
    const step = await Promise.resolve(i + 1);
    acc = acc + step;
    yield acc;
  }
  return acc;
}
async function run(): Promise<void> {
  const g = running(4);
  let r = await g.next();
  while (!r.done) {
    console.log("acc:" + r.value);
    r = await g.next();
  }
  console.log("final:" + r.value);
}
run();
console.log("sync");

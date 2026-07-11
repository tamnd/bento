// A for await...of over a user async iterable drives the async iterator protocol: it
// obtains the iterator from the class's [Symbol.asyncIterator], awaits the promise its
// next() returns each turn, stops when the settled result is done, and binds the value.
// The loop runs inside an async body, so it parks and resumes on the microtask queue and
// the synchronous code after the call runs first, the ordering Node fixes for the loop.
class Counter {
  i = 0;
  next(): Promise<{ value: number; done: boolean }> {
    const cur = this.i;
    this.i = cur + 1;
    return Promise.resolve({ value: cur, done: cur >= 3 });
  }
  [Symbol.asyncIterator]() { return this; }
}
async function run(): Promise<void> {
  console.log("start");
  for await (const x of new Counter()) {
    console.log("x:" + x);
  }
  console.log("end");
}
run();
console.log("sync");

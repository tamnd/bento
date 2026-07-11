// Breaking out of a for await...of over a user async iterable that defines return() closes
// the iterator: the loop calls return() once on the early exit, awaiting the promise it
// hands back, so the close runs after the last body pass and before the after-loop
// statement. A loop that runs to completion would never call return(), so the close is
// reached only through the break, the same broke-flag gate the sync iterator close uses.
class Counter {
  n: number;
  i: number;
  constructor(n: number) { this.n = n; this.i = 0; }
  [Symbol.asyncIterator](): Counter { return this; }
  async next(): Promise<{ value: number; done: boolean }> {
    if (this.i < this.n) { const v = this.i; this.i++; return { value: v, done: false }; }
    return { value: 0, done: true };
  }
  async return(): Promise<{ value: number; done: boolean }> {
    console.log("closed at " + this.i);
    return { value: 0, done: true };
  }
}
async function run(): Promise<void> {
  for await (const x of new Counter(5)) {
    console.log("x:" + x);
    if (x === 2) break;
  }
  console.log("after");
}
run();
console.log("sync");

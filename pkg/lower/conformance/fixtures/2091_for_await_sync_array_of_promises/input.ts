// A for await...of over an array takes the spec's fallback for a sync iterable with no
// [Symbol.asyncIterator]: it drives the array's sync iterator and awaits each value it
// yields. An array of promises settles each one and binds the fulfilled value; an array
// of plain values awaits each after the one-turn delay await imposes and binds it back.
// The loop parks inside the async body, so the synchronous tail runs before the first
// element, the ordering Node fixes for the awaited loop.
async function run(): Promise<void> {
  console.log("start");
  for await (const x of [Promise.resolve(1), Promise.resolve(2), Promise.resolve(3)]) {
    console.log("x:" + x);
  }
  for await (const n of [10, 20, 30]) {
    console.log("n:" + n);
  }
  console.log("end");
}
run();
console.log("sync");

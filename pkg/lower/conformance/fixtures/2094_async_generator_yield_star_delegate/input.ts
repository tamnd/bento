// An async generator delegates part of its sequence to another async generator with
// yield*. The delegate's values flow out through the outer generator's pulls in order, and
// the delegate's completion value becomes the value of the yield* expression. The delegate
// awaits between its own yields, so the outer drive stays parked while the delegate runs
// and the yields still arrive one at a time; the outer generator resumes with its own yield
// once the delegate completes.
async function* inner(): AsyncGenerator<number, number> {
  yield await Promise.resolve(1);
  yield await Promise.resolve(2);
  return 99;
}
async function* outer(): AsyncGenerator<number> {
  yield 0;
  const r = yield* inner();
  console.log("delegate returned:" + r);
  yield 3;
}
async function run(): Promise<void> {
  const g = outer();
  let res = await g.next();
  while (!res.done) {
    console.log("v:" + res.value);
    res = await g.next();
  }
}
run();
console.log("sync");

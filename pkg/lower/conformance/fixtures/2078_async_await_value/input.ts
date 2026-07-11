// Awaiting a plain, non-promise value is legal: JavaScript wraps it in a resolved
// promise, so the await still suspends for one microtask turn before yielding the value
// back. The body therefore prints its first line during the synchronous run, parks at
// the await, and resumes with the awaited number only after the trailing synchronous
// line, at the microtask checkpoint.
async function run(): Promise<number> {
  console.log("body-start");
  const a = await 40;
  return a + 2;
}

console.log("before");
run().then((v) => console.log(v));
console.log("after-call");

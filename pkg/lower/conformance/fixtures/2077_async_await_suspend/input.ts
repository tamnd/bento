// An await suspends the async body: it runs synchronously up to the first await, parks
// there, and the code after the await runs in a later microtask turn, once the awaited
// promise has settled. So the body's first line prints during the synchronous run, but
// the line after the await, and the value the returned promise resolves to, print only
// after the trailing synchronous line, at the microtask checkpoint.
async function base(): Promise<number> {
  return 20;
}

async function load(): Promise<number> {
  console.log("body-start");
  const a = await base();
  console.log("after-await");
  return a + 22;
}

console.log("before");
load().then((v) => console.log(v));
console.log("after-call");

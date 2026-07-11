// An async function declaration returns a promise even when its body never awaits:
// the body runs to completion on the calling stack and the returned promise settles
// with the value the body returns. A .then callback registered on that promise fires
// after the synchronous run, at the microtask checkpoint, so the resolved value
// prints after the trailing line rather than inline where the call sits.
async function double(n: number): Promise<number> {
  return n * 2;
}

console.log("start");
double(21).then((v) => console.log(v));
console.log("end");

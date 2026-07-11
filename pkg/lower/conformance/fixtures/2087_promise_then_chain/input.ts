// A then whose callback returns a value chains that value to the next then, so a run of
// thens threads each stage's result to the following one. A callback that returns a
// promise flattens: the returned promise adopts the inner promise's state, so the next
// then reads the inner value rather than a promise of a promise. A rejection with no
// rejection handler on a then passes straight through to a catch further down the chain.
Promise.resolve(1)
  .then((v) => v + 1)
  .then((v) => "n:" + v)
  .then((s) => console.log(s));

Promise.resolve(10)
  .then((v) => Promise.resolve(v * 2))
  .then((v) => console.log("flat:" + v));

const failing: Promise<number> = Promise.reject("boom");
failing
  .then((v) => v + 100)
  .catch((e) => console.log("caught:" + e));

console.log("sync");

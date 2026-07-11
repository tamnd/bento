// Promise.resolve mints a settled promise carrying its argument, and Promise.reject
// one that is already rejected with its reason, both observed after the synchronous
// code through then and catch. Promise.resolve of a promise applies the same-promise
// rule: it hands that promise straight back, so its fulfilled value flows through
// unchanged rather than being wrapped in a second promise.
Promise.resolve(41).then((v) => console.log("resolve:" + (v + 1)));
Promise.reject("boom").catch((e) => console.log("reject:" + e));

const inner: Promise<number> = Promise.resolve(7);
Promise.resolve(inner).then((v) => console.log("adopt:" + v));

console.log("sync");

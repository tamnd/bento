// Promise.all combines an array of promises into one promise of their values in input
// order, observed through then after the synchronous code once every input has
// fulfilled. A single rejection among the inputs rejects the combined promise with
// that reason, the first rejection winning, so the catch runs rather than the then. An
// empty input fulfills straight away with an empty array.
const ps: Promise<number>[] = [Promise.resolve(1), Promise.resolve(2), Promise.resolve(3)];
Promise.all(ps).then((vs) => console.log("all:" + vs.join(",")));

const mixed: Promise<number>[] = [Promise.resolve(9), Promise.reject("bad")];
Promise.all(mixed).catch((e) => console.log("caught:" + e));

const empty: Promise<number>[] = [];
Promise.all(empty).then((vs) => console.log("empty:" + vs.length));

console.log("sync");

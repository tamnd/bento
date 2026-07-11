// A finally callback runs when the promise settles, whichever way it ends, and takes
// no argument. It is scheduled as a microtask like then and catch, so it runs after
// the synchronous code and in settle order: the fulfilled promise's then runs, then
// the finally on it, and the rejected promise's catch runs, then the finally on it.
new Promise<number>((resolve, reject) => { resolve(1); })
  .then((v) => console.log("then:" + v))
  .finally(() => console.log("finally-a"));

new Promise<number>((resolve, reject) => { reject("boom"); })
  .catch((e) => console.log("catch:" + e))
  .finally(() => console.log("finally-b"));

console.log("sync");

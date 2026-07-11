// A finally callback runs when the promise settles, whichever way it ends, and takes
// no argument. It is scheduled as a microtask like then and catch, and a finally chained
// onto a then or catch only runs once that reaction has settled its own promise, a turn
// later. So the two heads run first, the then and the catch, and only then the two
// finallys, in the order the chains were written, not each finally right after its head.
new Promise<number>((resolve, reject) => { resolve(1); })
  .then((v) => console.log("then:" + v))
  .finally(() => console.log("finally-a"));

new Promise<number>((resolve, reject) => { reject("boom"); })
  .catch((e) => console.log("catch:" + e))
  .finally(() => console.log("finally-b"));

console.log("sync");

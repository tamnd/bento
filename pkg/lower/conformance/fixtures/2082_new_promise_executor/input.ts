// new Promise runs its executor now and hands it a resolve and a reject callback. The
// executor here resolves synchronously, but the then callback still fires after the
// synchronous code that follows the construction, the microtask ordering a promise
// fixes. A second promise rejects with a plain string, not an Error, and its catch
// reads that value straight back, since a promise may be rejected with any value.
new Promise<number>((resolve, reject) => {
  resolve(41);
}).then((v) => console.log("then:" + (v + 1)));

new Promise<number>((resolve, reject) => {
  reject("boom");
}).catch((e) => console.log("catch:" + e));

console.log("sync");

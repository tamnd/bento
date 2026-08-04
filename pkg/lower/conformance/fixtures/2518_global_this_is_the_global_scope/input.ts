const p = globalThis.process;
console.log(typeof p, p === process);
console.log(p.platform === process.platform);
const g = globalThis;
console.log(typeof g, g === globalThis);

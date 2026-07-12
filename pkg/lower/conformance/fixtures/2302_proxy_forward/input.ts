// A Proxy whose handler carries no trap forwards every operation to its target, so
// it reads, writes, probes, and deletes exactly as the target would. This is the
// base a Proxy with an empty handler behaves as, and the floor every trap builds on.
const target: any = { x: 1 };
const p: any = new Proxy(target, {});
console.log(p.x); // 1
p.y = 2;
console.log(target.y); // 2
console.log("x" in p); // true
delete p.x;
console.log("x" in target); // false

// A Proxy over a callable target is itself callable and forwards the call to the
// target when the handler carries no apply trap.
const fn: any = (a: number, b: number): number => a + b;
const pf: any = new Proxy(fn, {});
console.log(pf(2, 3)); // 5

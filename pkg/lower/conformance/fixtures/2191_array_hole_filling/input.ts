const a: any = [1, 2, 3];
delete a[1];

// join treats a hole as undefined, contributing the empty string, so the gap shows
// as two commas with nothing between them.
const j: any = Array.prototype.join.call(a);
console.log(j);

// fill overwrites every index in range, so it fills the hole: all three indices
// become present own properties.
const f: any = [1, 2, 3];
delete f[1];
Array.prototype.fill.call(f, 0);
console.log(Object.keys(f).length);
console.log(f[1]);

// copyWithin carries a hole across by deleting the target, so a hole stays a hole
// rather than being written as undefined.
const c: any = [1, 2, 3];
delete c[1];
Array.prototype.copyWithin.call(c, 0, 1);
console.log(Object.keys(c).length);
console.log(c[1]);

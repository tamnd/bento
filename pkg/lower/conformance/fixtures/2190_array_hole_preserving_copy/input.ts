const a: any = [1, 2, 3];
delete a[1];

// slice carries the hole across: the copy still spans three indices but only two are
// present own properties, so the hole at index 1 stays a hole.
const s: any = Array.prototype.slice.call(a);
console.log(s.length);
console.log(Object.keys(s).length);

// map leaves a hole a hole: the callback never runs on index 1, so the result has a
// hole there rather than a doubled undefined.
const m: any = Array.prototype.map.call(a, (x: any) => x * 2);
console.log(m.length);
console.log(Object.keys(m).length);
console.log(m[0]);
console.log(m[2]);

// concat spreads the array and carries the hole across, then appends a non-array
// argument whole.
const c: any = Array.prototype.concat.call(a, 9);
console.log(c.length);
console.log(Object.keys(c).length);
console.log(c[3]);

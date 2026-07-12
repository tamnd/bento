// The typed-array copy and search methods run over the view and clamp to its
// length, mirroring the array methods but numeric throughout: fill and copyWithin
// write in place, slice copies a range into a fresh array, set copies a source
// list at an offset, and the search methods compare by Number.
const a = new Int32Array([10, 20, 30, 40, 50]);

// fill overwrites a half-open range in place and coerces the value.
const f = new Int32Array([1, 2, 3, 4, 5]);
f.fill(9, 1, 3);
console.log(f.join(","));

// slice copies a range into a fresh array; the source is untouched.
const s = a.slice(1, 4);
console.log(s.join(","));
console.log(a.join(","));

// copyWithin moves a block within the same array, overlap included.
const c = new Int32Array([1, 2, 3, 4, 5]);
c.copyWithin(0, 3);
console.log(c.join(","));

// the search methods use numeric equality; at counts from the end.
console.log(a.indexOf(30));
console.log(a.lastIndexOf(50));
console.log(a.includes(40));
console.log(a.includes(999));
console.log(a.at(-1) ?? 0);

// join with an explicit separator and with the default comma.
console.log(a.join(" "));
console.log(a.join());

// set copies a source list at an offset, coercing each element into the view.
const t = new Int32Array(5);
t.set([7, 8, 9], 1);
console.log(t.join(","));

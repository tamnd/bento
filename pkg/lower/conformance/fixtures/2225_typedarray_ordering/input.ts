// The typed-array ordering methods reorder the view or a copy of it. reverse and
// sort mutate in place and return the receiver; toReversed, toSorted, and with
// leave the receiver untouched and return a fresh array of the same element kind.
// A typed array's default sort is numeric ascending, unlike an array's string
// order, so [10, 1, 2, 20, 3] sorts to 1, 2, 3, 10, 20 rather than by code unit.
const a = new Int32Array([10, 1, 2, 20, 3]);

// toSorted with no comparator copies in numeric order, receiver unchanged.
const s = a.toSorted();
console.log(s.join(","));
console.log(a.join(","));

// sort with no comparator reorders the view in place and returns it.
a.sort();
console.log(a.join(","));

// a comparator sorts by its sign; here descending.
const d = new Int32Array([3, 1, 2]);
d.sort((x, y) => y - x);
console.log(d.join(","));

// toSorted with a comparator copies, leaving the receiver in order.
const e = new Int32Array([3, 1, 2]);
console.log(e.toSorted((x, y) => y - x).join(","));
console.log(e.join(","));

// reverse in place and toReversed as a copy.
const r = new Int32Array([1, 2, 3]);
r.reverse();
console.log(r.join(","));
const t = new Int32Array([1, 2, 3]);
console.log(t.toReversed().join(","));
console.log(t.join(","));

// with returns a fresh array with one element replaced; a negative index counts
// from the end, and the receiver keeps its values.
const w = new Int32Array([1, 2, 3]);
console.log(w.with(1, 99).join(","));
console.log(w.with(-1, 77).join(","));
console.log(w.join(","));

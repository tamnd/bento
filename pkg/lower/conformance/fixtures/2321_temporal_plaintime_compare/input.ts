// Temporal.PlainTime.compare orders two times, returning -1, 1, or 0, and it orders on
// the least significant field when the rest match. equals reports field equality, and
// from over a PlainTime returns a distinct object that still compares equal to its
// source, the copy the specification makes.
const a = new Temporal.PlainTime(1, 0, 0);
const b = new Temporal.PlainTime(2, 0, 0);
console.log(Temporal.PlainTime.compare(a, b));
console.log(Temporal.PlainTime.compare(b, a));
console.log(Temporal.PlainTime.compare(a, a));
const lo = new Temporal.PlainTime(3, 15, 30, 0, 0, 1);
const hi = new Temporal.PlainTime(3, 15, 30, 0, 0, 2);
console.log(Temporal.PlainTime.compare(lo, hi));
console.log(a.equals(a));
console.log(a.equals(b));
const copy = Temporal.PlainTime.from(a);
console.log(a.equals(copy));

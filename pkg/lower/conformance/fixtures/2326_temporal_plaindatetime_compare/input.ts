// Temporal.PlainDateTime.compare orders two date-times, returning -1, 1, or 0. It compares
// the date first and falls to the time only when the dates match, so a pair that shares a
// date still orders on the clock. equals reports field equality, and from over a
// PlainDateTime returns a distinct object that still compares equal to its source.
const a = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);
const b = new Temporal.PlainDateTime(1976, 11, 18, 15, 23, 30, 123, 456, 789);
console.log(Temporal.PlainDateTime.compare(a, b));
console.log(Temporal.PlainDateTime.compare(b, a));
console.log(Temporal.PlainDateTime.compare(a, a));
const early = new Temporal.PlainDateTime(2020, 6, 15, 8, 0);
const late = new Temporal.PlainDateTime(2020, 6, 15, 9, 0);
console.log(Temporal.PlainDateTime.compare(early, late));
console.log(a.equals(a));
console.log(a.equals(b));
const copy = Temporal.PlainDateTime.from(a);
console.log(a.equals(copy));

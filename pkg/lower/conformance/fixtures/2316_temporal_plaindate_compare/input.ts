// Temporal.PlainDate comparison: the static compare orders two dates as -1, 1, or
// 0, and equals reports whether two dates fall on the same day under the same ISO
// calendar. Temporal.PlainDate.from over a PlainDate returns a distinct object that
// compares equal to its source, the copy the specification makes.
const a = new Temporal.PlainDate(2020, 1, 1);
const b = new Temporal.PlainDate(2020, 3, 15);
const c = Temporal.PlainDate.from(a);
console.log(Temporal.PlainDate.compare(a, b));
console.log(Temporal.PlainDate.compare(b, a));
console.log(Temporal.PlainDate.compare(a, c));
console.log(a.equals(c));
console.log(a.equals(b));

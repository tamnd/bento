// Temporal.PlainDateTime.toString renders the ISO date and time joined by "T", each half
// rendered as its own type renders it: the year expands to a signed six-digit form outside
// 0..9999, and the fractional-second part appears only when a sub-second field is set,
// trimmed to the fewest digits. toJSON produces the same string.
const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);
console.log(dt.toString());
console.log(new Temporal.PlainDateTime(1976, 11, 18, 15, 23, 30, 123, 456, 789).toString());
console.log(new Temporal.PlainDateTime(2020, 1, 1, 1, 2, 3, 250).toString());
console.log(new Temporal.PlainDateTime(2020, 2, 29, 0, 0, 0).toString());
console.log(new Temporal.PlainDateTime(-1, 1, 1, 0, 0).toString());
console.log(new Temporal.PlainDateTime(12345, 1, 1, 0, 0).toString());
console.log(dt.toJSON());

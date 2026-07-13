// Temporal.PlainDate.prototype.until returns the calendar duration from the receiver to the
// argument. With the default largestUnit of day it counts whole days between the two dates.
// A coarser largestUnit balances that span into years, months, weeks, and days, stepping
// whole years then whole months so a short target month settles into the remaining days:
// January 31 to February 29 over years is 29 days, not one month, because the intermediate
// February 31 falls past the target. A plural unit name resolves to its singular. since is
// until with the sign flipped. Two equal dates return the zero duration. The values were
// checked against @js-temporal/polyfill.
const a = new Temporal.PlainDate(2020, 1, 31);
const b = new Temporal.PlainDate(2021, 3, 30);
console.log(a.until(b).toString());
console.log(a.until(b, { largestUnit: "year" }).toString());
console.log(a.until(b, { largestUnit: "month" }).toString());
console.log(a.until(b, { largestUnit: "weeks" }).toString());
console.log(a.since(b, { largestUnit: "year" }).toString());
console.log(b.until(a, { largestUnit: "year" }).toString());
console.log(new Temporal.PlainDate(2020, 1, 31).until(new Temporal.PlainDate(2020, 2, 29), { largestUnit: "year" }).toString());
console.log(new Temporal.PlainDate(2000, 1, 1).until(new Temporal.PlainDate(2025, 6, 15), { largestUnit: "year" }).toString());
console.log(new Temporal.PlainDate(2024, 3, 31).since(new Temporal.PlainDate(2024, 1, 31), { largestUnit: "month" }).toString());
console.log(a.until(a).toString());

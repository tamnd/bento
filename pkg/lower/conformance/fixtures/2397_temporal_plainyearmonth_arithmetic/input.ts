// Temporal.PlainYearMonth.prototype.add and subtract anchor the year-month to a reference day (the
// first of the month going forward, the last going backward), add the Duration's date fields, then
// drop back to a year-month, so a day-carrying Duration never shifts the result. until and since
// measure the gap between two year-months as a years-and-months Duration, since being until negated,
// with largestUnit defaulting to "year" and "month" flattening the years into months. An add that
// pushes the year past the representable range is a RangeError. Every result was checked against
// @js-temporal/polyfill.
const a = new Temporal.PlainYearMonth(2020, 3);
const b = new Temporal.PlainYearMonth(2021, 8);
console.log(a.add({ months: 1 }).toString());
console.log(a.add({ years: 1, months: 11 }).toString());
console.log(a.add({ months: 1, days: 400 }).toString());
console.log(a.subtract({ months: 1 }).toString());
console.log(a.subtract({ years: 1, months: 4 }).toString());
console.log(a.until(b).toString());
console.log(a.until(b, { largestUnit: "month" }).toString());
console.log(a.since(b).toString());
console.log(b.since(a, { largestUnit: "month" }).toString());
console.log(a.until(a).toString());

try {
  a.add({ years: 1000000 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

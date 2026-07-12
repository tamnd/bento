// Temporal.PlainYearMonth.prototype.toString renders the ISO 8601 year-month, YYYY-MM, with
// the year expanded to a signed six-digit form outside 0..9999. The ISO calendar hides the
// reference day, so no day appears. toJSON returns the same string. The values match
// @js-temporal/polyfill.
console.log(new Temporal.PlainYearMonth(2020, 3).toString());
console.log(new Temporal.PlainYearMonth(-1, 5).toString());
console.log(new Temporal.PlainYearMonth(10000, 5).toString());
console.log(new Temporal.PlainYearMonth(2020, 3).toJSON());

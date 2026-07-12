// Temporal.PlainYearMonth.compare orders two year-months, -1 when the first precedes the
// second and 1 when it follows, and equals reports whether they are the same year-month. from
// over a PlainYearMonth returns a fresh copy that renders the same string. The values match
// @js-temporal/polyfill.
const a = new Temporal.PlainYearMonth(2020, 3);
const b = new Temporal.PlainYearMonth(2020, 4);
console.log(Temporal.PlainYearMonth.compare(a, b));
console.log(Temporal.PlainYearMonth.compare(b, a));
console.log(a.equals(new Temporal.PlainYearMonth(2020, 3)));
console.log(a.equals(b));
const c = Temporal.PlainYearMonth.from(a);
console.log(c.toString());

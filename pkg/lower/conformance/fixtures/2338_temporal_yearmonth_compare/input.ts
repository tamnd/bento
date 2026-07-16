// Temporal.PlainYearMonth.compare orders two year-months, -1 when the first precedes the
// second and 1 when it follows, and equals reports whether they are the same year-month. Besides a
// PlainYearMonth argument, each accepts an ISO string or a year-month-like bag, coercing it to a
// PlainYearMonth first through ToTemporalYearMonth, so the comparison measures against the
// year-month the argument names rather than handing back. from over a PlainYearMonth returns a
// fresh copy that renders the same string. The values match @js-temporal/polyfill.
const a = new Temporal.PlainYearMonth(2020, 3);
const b = new Temporal.PlainYearMonth(2020, 4);
console.log(Temporal.PlainYearMonth.compare(a, b));
console.log(Temporal.PlainYearMonth.compare(b, a));
console.log(Temporal.PlainYearMonth.compare("2020-03", "2020-04"));
console.log(Temporal.PlainYearMonth.compare({ year: 2021, month: 1 }, { year: 2020, month: 12 }));
console.log(a.equals(new Temporal.PlainYearMonth(2020, 3)));
console.log(a.equals(b));
console.log(a.equals("2020-03"));
const c = Temporal.PlainYearMonth.from(a);
console.log(c.toString());

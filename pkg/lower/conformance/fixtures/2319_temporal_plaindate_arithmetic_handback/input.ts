// PlainDate.toZonedDateTime pins a date to a time zone, resolving the wall-clock instant, which
// a later slice carries. It hands back here so the compiler reports the ceiling rather than emit
// a wrong value. add, subtract, until, since, with, toPlainDateTime, toPlainYearMonth, and
// toPlainMonthDay already lower.
const d = new Temporal.PlainDate(2020, 2, 29);
const z = d.toZonedDateTime("UTC");
console.log(z.epochMilliseconds);

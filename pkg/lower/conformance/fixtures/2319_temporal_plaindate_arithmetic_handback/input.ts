// PlainDate.toLocaleString formats a date for a locale, which needs the Intl machinery a later
// slice carries. It hands back here so the compiler reports the ceiling rather than emit a wrong
// value. add, subtract, until, since, with, and the four conversions toPlainDateTime,
// toPlainYearMonth, toPlainMonthDay, and toZonedDateTime already lower.
const d = new Temporal.PlainDate(2020, 2, 29);
console.log(d.toLocaleString());

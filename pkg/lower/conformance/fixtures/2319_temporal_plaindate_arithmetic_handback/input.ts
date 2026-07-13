// PlainDate.toPlainMonthDay narrows a date to its month and day under the date's calendar,
// which a later slice carries. It hands back here so the compiler reports the ceiling rather
// than emit a wrong value. add, subtract, until, since, with, toPlainDateTime, and
// toPlainYearMonth already lower.
const d = new Temporal.PlainDate(2020, 2, 29);
const md = d.toPlainMonthDay();
console.log(md.day);

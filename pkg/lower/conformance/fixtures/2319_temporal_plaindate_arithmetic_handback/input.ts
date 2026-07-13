// PlainDate.toPlainYearMonth narrows a date to its year and month under the date's calendar,
// which a later slice carries. It hands back here so the compiler reports the ceiling rather
// than emit a wrong value. add, subtract, until, since, with, and toPlainDateTime already
// lower.
const d = new Temporal.PlainDate(2020, 2, 29);
const ym = d.toPlainYearMonth();
console.log(ym.month);

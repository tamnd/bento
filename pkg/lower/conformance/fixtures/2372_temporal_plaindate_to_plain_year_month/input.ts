// Temporal.PlainDate.prototype.toPlainYearMonth narrows a date to its year and month, dropping
// the day, under the date's own calendar. An ISO date prints YYYY-MM with the day hidden. A
// non-ISO date keeps its calendar: the year getter reads in the calendar's reckoning, so a roc
// date counts from 1912, and the toString carries the reference day, the first of the month,
// with the "[u-ca=<id>]" annotation. The values were checked against @js-temporal/polyfill.
const d = new Temporal.PlainDate(2020, 5, 15);
console.log(d.toPlainYearMonth().toString());
console.log(d.toPlainYearMonth().year);
console.log(d.withCalendar("roc").toPlainYearMonth().toString());
console.log(d.withCalendar("roc").toPlainYearMonth().year);
console.log(d.withCalendar("gregory").toPlainYearMonth().toString());

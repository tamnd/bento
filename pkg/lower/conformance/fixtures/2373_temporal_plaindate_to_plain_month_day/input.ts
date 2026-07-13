// Temporal.PlainDate.prototype.toPlainMonthDay narrows a date to its month and day, dropping
// the year, under the date's own calendar. An ISO date prints MM-DD with the year hidden. A
// non-ISO date keeps its calendar: the toString carries the reference year 1972, the actual
// month and day, and the "[u-ca=<id>]" annotation. A leap day round-trips in both forms. The
// values were checked against @js-temporal/polyfill.
const d = new Temporal.PlainDate(2020, 5, 15);
console.log(d.toPlainMonthDay().toString());
console.log(d.toPlainMonthDay().day);
console.log(d.withCalendar("roc").toPlainMonthDay().toString());
console.log(d.withCalendar("gregory").toPlainMonthDay().toString());
const leap = new Temporal.PlainDate(2020, 2, 29);
console.log(leap.toPlainMonthDay().toString());
console.log(leap.withCalendar("roc").toPlainMonthDay().toString());

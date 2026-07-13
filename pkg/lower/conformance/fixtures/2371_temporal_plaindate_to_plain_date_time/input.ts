// Temporal.PlainDate.prototype.toPlainDateTime widens a date to a date-time by pairing it
// with a wall clock. With no argument the time defaults to midnight; a Temporal.PlainTime
// argument supplies the time, and its subsecond components carry through to nanoseconds. The
// result keeps the date's calendar, so a roc date stays under roc. The values were checked
// against @js-temporal/polyfill.
const d = new Temporal.PlainDate(2020, 3, 14);
console.log(d.toPlainDateTime().toString());
console.log(d.toPlainDateTime(new Temporal.PlainTime(15, 30, 45)).toString());
console.log(d.toPlainDateTime(new Temporal.PlainTime(1, 2, 3, 4, 5, 6)).toString());
console.log(d.withCalendar("roc").toPlainDateTime(new Temporal.PlainTime(9, 0)).toString());

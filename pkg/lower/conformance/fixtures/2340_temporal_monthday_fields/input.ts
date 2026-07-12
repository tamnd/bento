// Temporal.PlainMonthDay is a calendar month and day with no year, the way a birthday recurs.
// It exposes its month only through the month code, plus its day and calendar id, and renders
// as MM-DD with the reference year hidden. The leap reference year admits February 29. equals
// compares two month-days, and from over a PlainMonthDay returns a fresh copy. The values match
// @js-temporal/polyfill.
const md = new Temporal.PlainMonthDay(3, 15);
console.log(md.monthCode);
console.log(md.day);
console.log(md.calendarId);
console.log(md.toString());
console.log(md.toJSON());
console.log(new Temporal.PlainMonthDay(2, 29).toString());
console.log(md.equals(new Temporal.PlainMonthDay(3, 15)));
console.log(md.equals(new Temporal.PlainMonthDay(3, 16)));
const c = Temporal.PlainMonthDay.from(md);
console.log(c.toString());

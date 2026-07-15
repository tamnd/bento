// PlainDateTime.withPlainTime keeps the calendar date and swaps in a new wall clock. No argument
// resets the clock to midnight; a Temporal.PlainTime, a time string, or a time-like bag of numbers
// (regulated under constrain) sets it. The date carries its calendar through the reshape. Every
// value was checked against @js-temporal/polyfill.
const dt = new Temporal.PlainDateTime(2020, 5, 15, 13, 30, 45, 500, 250, 125);
console.log(dt.withPlainTime().toString());
console.log(dt.withPlainTime(new Temporal.PlainTime(9, 15)).toString());
console.log(dt.withPlainTime("22:45:10").toString());
console.log(dt.withPlainTime({ hour: 6, minute: 5 }).toString());
console.log(dt.withPlainTime({ hour: 25 }).toString());

const g = dt.withCalendar("gregory");
console.log(g.withPlainTime().toString());
console.log(g.withPlainTime(new Temporal.PlainTime(9, 15, 30)).toString());

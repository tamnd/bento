// PlainDateTime splits into its two halves. toPlainDate keeps the calendar and drops the clock;
// toPlainTime keeps the clock and drops the date. Each returns a fresh plain type whose getters
// and toString then route on. When the date-time carries a hosted calendar, the date half carries
// it too. Every value was checked against @js-temporal/polyfill.
const dt = new Temporal.PlainDateTime(2020, 5, 15, 13, 30, 45, 500, 250, 125);
console.log(dt.toPlainDate().toString());
console.log(dt.toPlainDate().day);
console.log(dt.toPlainDate().dayOfWeek);
console.log(dt.toPlainTime().toString());
console.log(dt.toPlainTime().hour);
console.log(dt.toPlainTime().second);

const g = new Temporal.PlainDateTime(2020, 5, 15, 13, 30).withCalendar("gregory");
console.log(g.toPlainDate().calendarId);
console.log(g.toPlainDate().toString());

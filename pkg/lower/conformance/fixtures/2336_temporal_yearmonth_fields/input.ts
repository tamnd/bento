// Temporal.PlainYearMonth is a calendar year and month with no day. Its clean ISO getters,
// the year and month, the month code, the calendar id, and the derived day counts, months in
// year, and leap flag, read straight off the pair. The leap year-month 2020-02 exercises the
// leap-sensitive fields. The values match @js-temporal/polyfill.
const ym = new Temporal.PlainYearMonth(2020, 2);
console.log(ym.year);
console.log(ym.month);
console.log(ym.monthCode);
console.log(ym.calendarId);
console.log(ym.daysInMonth);
console.log(ym.daysInYear);
console.log(ym.monthsInYear);
console.log(ym.inLeapYear);

// Temporal.PlainDate over the ISO calendar: construction from the three number
// components and the clean field getters. The leap date 2020-02-29 exercises the
// derived getters (the weekday, the day of the year, the leap flag, the month and
// year lengths) alongside the stored year, month, and day. The calendar-dependent
// getters the checker types number | undefined (era, weekOfYear) are a later slice.
const d = new Temporal.PlainDate(2020, 2, 29);
console.log(d.year);
console.log(d.month);
console.log(d.day);
console.log(d.calendarId);
console.log(d.monthCode);
console.log(d.dayOfWeek);
console.log(d.dayOfYear);
console.log(d.daysInWeek);
console.log(d.daysInMonth);
console.log(d.daysInYear);
console.log(d.monthsInYear);
console.log(d.inLeapYear);

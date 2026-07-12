// Temporal.PlainDateTime: construction from three date components and up to six time
// components, then the field getters. A date-time with every field set exercises the ISO
// date getters (year, month, day, and the derived weekday, days-in-month, leap flag,
// month code, calendar id) alongside the six clean time getters. The values match
// @js-temporal/polyfill.
const dt = new Temporal.PlainDateTime(1976, 11, 18, 15, 23, 30, 123, 456, 789);
console.log(dt.year);
console.log(dt.month);
console.log(dt.day);
console.log(dt.hour);
console.log(dt.minute);
console.log(dt.second);
console.log(dt.millisecond);
console.log(dt.microsecond);
console.log(dt.nanosecond);
console.log(dt.dayOfWeek);
console.log(dt.daysInMonth);
console.log(dt.inLeapYear);
console.log(dt.monthCode);
console.log(dt.calendarId);

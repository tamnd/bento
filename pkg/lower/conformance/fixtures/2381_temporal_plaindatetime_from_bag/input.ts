// PlainDateTime.from over a property bag builds a date-time from the required year, month, and day
// plus the optional time fields, defaulting an omitted time field to the midnight the record
// carries. Each half regulates under the overflow option: the date reads the year in the calendar's
// own reckoning and clamps the day to the resulting month under constrain, the time clamps to its
// ISO maxima, and reject throws on an out-of-range field. The calendar carries through. Every value
// was checked against @js-temporal/polyfill.
console.log(Temporal.PlainDateTime.from({ year: 2020, month: 1, day: 31 }).toString());
console.log(Temporal.PlainDateTime.from({ year: 2020, month: 1, day: 31, hour: 13, minute: 30, second: 45 }).toString());
console.log(Temporal.PlainDateTime.from({ year: 2020, month: 2, day: 31 }).toString());
console.log(Temporal.PlainDateTime.from({ year: 2020, month: 13, day: 5 }).toString());
console.log(Temporal.PlainDateTime.from({ year: 2020, month: 1, day: 31, hour: 25 }).toString());
console.log(Temporal.PlainDateTime.from({ year: 2020, month: 1, day: 31, hour: 5, millisecond: 250 }).toString());
console.log(Temporal.PlainDateTime.from({ year: 109, month: 5, day: 15, hour: 12, calendar: "roc" }).toString());
console.log(Temporal.PlainDateTime.from({ year: 2020, month: 5, day: 15, calendar: "gregory" }).toString());

try {
  Temporal.PlainDateTime.from({ year: 2020, month: 2, day: 31 }, { overflow: "reject" });
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

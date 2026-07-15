// Temporal.PlainYearMonth.from and Temporal.PlainMonthDay.from over a property bag build from the
// fields the bag supplies, ISO-gated. A year-month reads a required year and month, a month-day a
// required month and day, and a monthCode of the form "MNN" resolves to its month. A month-day may
// also carry a year, which sets the year the day is validated against, so February 29 with a common
// year constrains to the 28th while the leap reference year admits it. Under the default constrain
// an out-of-range field clamps; under reject it is a RangeError. Every result was checked against
// @js-temporal/polyfill.
console.log(Temporal.PlainYearMonth.from({ year: 2020, month: 3 }).toString());
console.log(Temporal.PlainYearMonth.from({ year: 2020, monthCode: "M07" }).toString());
console.log(Temporal.PlainYearMonth.from({ year: 2020, month: 13 }).toString());
console.log(Temporal.PlainMonthDay.from({ month: 3, day: 15 }).toString());
console.log(Temporal.PlainMonthDay.from({ monthCode: "M02", day: 29 }).toString());
console.log(Temporal.PlainMonthDay.from({ year: 2021, month: 2, day: 29 }).toString());
console.log(Temporal.PlainMonthDay.from({ month: 4, day: 31 }).toString());

try {
  Temporal.PlainYearMonth.from({ year: 2020, month: 13 }, { overflow: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}
try {
  Temporal.PlainMonthDay.from({ month: 4, day: 31 }, { overflow: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

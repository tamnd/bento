// Temporal.PlainMonthDay.prototype.with overlays the present month and day of a partial bag onto the
// receiver, each absent field keeping its value, and a monthCode of the form "MNN" resolves to its
// month. Under the default constrain an out-of-range day clamps to the month's length, so month 2
// day 30 lands on the 29th of the leap reference year; under reject it is a RangeError. toPlainDate
// supplies the missing year and always constrains the day, so February 29 lands on the 28th in a
// common year. Every result was checked against @js-temporal/polyfill.
const a = new Temporal.PlainMonthDay(3, 15);
console.log(a.with({ day: 20 }).toString());
console.log(a.with({ month: 12 }).toString());
console.log(a.with({ monthCode: "M07", day: 4 }).toString());
console.log(a.with({ month: 2, day: 30 }).toString());
console.log(a.toPlainDate({ year: 2020 }).toString());
const feb29 = new Temporal.PlainMonthDay(2, 29);
console.log(feb29.toPlainDate({ year: 2020 }).toString());
console.log(feb29.toPlainDate({ year: 2021 }).toString());

try {
  a.with({ month: 4, day: 31 }, { overflow: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

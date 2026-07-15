// Temporal.PlainYearMonth.prototype.with overlays the present year and month of a partial bag onto
// the receiver, each absent field keeping its value, and a monthCode of the form "MNN" resolves to
// its month. Under the default constrain an out-of-range month clamps to December; under reject it
// is a RangeError. toPlainDate combines the year-month with a day and always constrains the day to
// the month's length, so day 31 in February lands on the 29th in a leap year. Every result was
// checked against @js-temporal/polyfill.
const a = new Temporal.PlainYearMonth(2020, 3);
console.log(a.with({ month: 11 }).toString());
console.log(a.with({ year: 1999 }).toString());
console.log(a.with({ year: 2021, month: 2 }).toString());
console.log(a.with({ month: 13 }).toString());
console.log(a.with({ monthCode: "M07" }).toString());
console.log(a.toPlainDate({ day: 15 }).toString());
console.log(a.toPlainDate({ day: 31 }).toString());
const feb = new Temporal.PlainYearMonth(2020, 2);
console.log(feb.toPlainDate({ day: 31 }).toString());

try {
  a.with({ month: 13 }, { overflow: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

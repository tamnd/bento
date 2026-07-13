// Temporal.PlainDate.prototype.with lays a bag's present fields over the receiver and
// regulates the result, so an omitted field keeps its current value. Setting the month to
// February over January 31 regulates the day to the last day of February under the constrain
// default, a month past twelve clamps to December, and a day past the month length clamps to
// the month end. Setting the year of a leap day to a common year lands on February 28. The
// overflow option defaults to constrain, which clamps; overflow reject throws a RangeError
// when a field does not fit. The values were checked against @js-temporal/polyfill.
const d = new Temporal.PlainDate(2020, 1, 31);
console.log(d.with({ month: 2 }).toString());
console.log(d.with({ day: 15 }).toString());
console.log(d.with({ year: 2021, month: 6, day: 10 }).toString());
console.log(d.with({ month: 13 }).toString());
console.log(d.with({ day: 40 }).toString());
console.log(new Temporal.PlainDate(2020, 2, 29).with({ year: 2021 }).toString());

// overflow reject rejects a day that does not fit the resulting month, so reading the
// caught error's constructor name proves the runtime raised the exact RangeError.
try {
  d.with({ day: 40 }, { overflow: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

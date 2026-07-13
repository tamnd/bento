// Temporal.PlainDate.prototype.add folds a duration into a calendar date. Years and months
// carry through the year, then the original day is regulated against the new month, so adding
// one month to January 31 clamps to the last day of February. Weeks fold to days and days
// balance through the calendar. The time units only count as whole days: adding 25 hours
// advances one day and the leftover 1 hour is dropped, while 23 hours leaves the date
// unchanged. subtract is add over the negated duration. The overflow option defaults to
// constrain, which clamps a short month; overflow reject throws a RangeError instead. The
// values were checked against @js-temporal/polyfill.
const d = new Temporal.PlainDate(2020, 1, 31);
console.log(d.add({ months: 1 }).toString());
console.log(d.add({ years: 4, months: 1 }).toString());
console.log(new Temporal.PlainDate(2020, 2, 29).add({ years: 1 }).toString());
console.log(d.add({ weeks: 2 }).toString());
console.log(d.add({ days: 30 }).toString());
console.log(d.add({ months: 1, days: 5 }).toString());
console.log(d.add({ hours: 25 }).toString());
console.log(d.add({ days: 1, hours: 25 }).toString());
console.log(d.add({ hours: 23 }).toString());
console.log(d.subtract({ months: 1 }).toString());
console.log(d.subtract({ days: 31 }).toString());

// overflow reject rejects a day that does not fit the resulting month, so reading the
// caught error's constructor name proves the runtime raised the exact RangeError.
try {
  d.add({ months: 1 }, { overflow: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

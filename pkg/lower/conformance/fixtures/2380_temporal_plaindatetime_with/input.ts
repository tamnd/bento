// PlainDateTime.with overlays a bag of date and time fields on the receiver and regulates each
// half under the overflow option, so an omitted field keeps its current value. The date half reads
// the year in the calendar's own reckoning and clamps the day to the resulting month under
// constrain; the time half clamps to its ISO maxima. The calendar carries through, and reject
// throws on an out-of-range field. Every value was checked against @js-temporal/polyfill.
const dt = new Temporal.PlainDateTime(2020, 1, 31, 13, 30, 45, 500, 250, 125);
console.log(dt.with({ month: 2 }).toString());
console.log(dt.with({ day: 15 }).toString());
console.log(dt.with({ hour: 6, minute: 0 }).toString());
console.log(dt.with({ year: 2021, month: 6, day: 10, hour: 8, minute: 5, second: 3 }).toString());
console.log(dt.with({ month: 13 }).toString());
console.log(dt.with({ hour: 25 }).toString());

const r = new Temporal.PlainDateTime(2020, 5, 15, 12, 0).withCalendar("roc");
console.log(r.with({ year: 100 }).toString());

try {
  dt.with({ hour: 25 }, { overflow: "reject" });
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

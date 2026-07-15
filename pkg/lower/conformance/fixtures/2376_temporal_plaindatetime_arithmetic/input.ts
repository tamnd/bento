// PlainDateTime arithmetic folds the time part into the wall clock and carries a whole day into
// the date. add and subtract regulate the date under the overflow rule, then apply the time and
// its day carry. until and since count the difference in the largestUnit, borrowing a day when the
// time part points against the calendar direction. round carries a day past midnight and rejects a
// day increment other than one. Every value was checked against @js-temporal/polyfill.
const base = new Temporal.PlainDateTime(2020, 1, 31, 12, 30, 45);
console.log(base.add({ months: 1 }).toString());
console.log(base.add({ hours: 13 }).toString());
console.log(base.add({ days: 1, hours: 25 }).toString());
console.log(base.subtract({ months: 1 }).toString());

const a = new Temporal.PlainDateTime(2020, 1, 1, 12, 0);
const b = new Temporal.PlainDateTime(2020, 1, 2, 6, 0);
console.log(a.until(b).toString());
console.log(a.since(b).toString());

const c = new Temporal.PlainDateTime(2020, 1, 31, 12, 30, 45);
const d = new Temporal.PlainDateTime(2021, 3, 30, 18, 45, 50, 500);
console.log(c.until(d).toString());
console.log(c.until(d, { largestUnit: "year" }).toString());
console.log(c.until(d, { largestUnit: "month" }).toString());

const rb = new Temporal.PlainDateTime(2020, 1, 31, 3, 34, 56, 987, 654, 321);
console.log(rb.round("day").toString());
console.log(new Temporal.PlainDateTime(2020, 1, 31, 18, 0).round("day").toString());
console.log(rb.round({ smallestUnit: "hour" }).toString());
console.log(rb.round({ smallestUnit: "minute", roundingIncrement: 15 }).toString());

try {
  base.add({ months: 1 }, { overflow: "reject" });
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

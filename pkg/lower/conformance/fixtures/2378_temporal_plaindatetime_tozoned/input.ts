// PlainDateTime.toZonedDateTime pins the wall clock to a time zone, resolving the exact instant
// under a disambiguation. compatible (the default) takes the earlier reading in a fall-back
// overlap and shifts forward across a spring-forward gap; earlier and later pick the two sides of
// a gap and the two overlap readings; reject throws on an ambiguous or gapped reading. The date
// carries its calendar through the zone bracket. Every value was checked against
// @js-temporal/polyfill.
const dt = new Temporal.PlainDateTime(2020, 3, 14, 15, 30, 45);
console.log(dt.toZonedDateTime("UTC").toString());
console.log(dt.toZonedDateTime("America/New_York").epochMilliseconds);

const gap = new Temporal.PlainDateTime(2020, 3, 8, 2, 30);
console.log(gap.toZonedDateTime("America/New_York").toString());
console.log(gap.toZonedDateTime("America/New_York", { disambiguation: "earlier" }).toString());
console.log(gap.toZonedDateTime("America/New_York", { disambiguation: "later" }).toString());

const dup = new Temporal.PlainDateTime(2020, 11, 1, 1, 30);
console.log(dup.toZonedDateTime("America/New_York").toString());
console.log(dup.toZonedDateTime("America/New_York", { disambiguation: "later" }).toString());

const g = new Temporal.PlainDateTime(2020, 3, 14, 15, 30).withCalendar("gregory");
console.log(g.toZonedDateTime("UTC").toString());

try {
  gap.toZonedDateTime("America/New_York", { disambiguation: "reject" });
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

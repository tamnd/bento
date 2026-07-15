// Temporal.ZonedDateTime reshaping keeps the instant fixed only where the wall clock does not move.
// with overlays date and time fields onto the wall clock and re-resolves preferring the original
// offset, so setting a minute keeps the noon hour and its offset, and a field that lands in a
// spring-forward gap is pushed forward while a field inside the fall-back overlap keeps its branch.
// Constrain clamps January the thirty-first into a shorter February, and a reject overflow throws a
// RangeError. withPlainTime swaps the time and defaults to midnight, withTimeZone keeps the instant
// and re-homes it onto the new zone, and withCalendar over a literal iso8601 is the identity. Every
// result was checked against @js-temporal/polyfill.
const z = Temporal.ZonedDateTime.from("2024-06-15T12:30:45[America/New_York]");
console.log(z.with({ minute: 0 }).toString());
console.log(Temporal.ZonedDateTime.from("2024-03-10T12:00:00[America/New_York]").with({ hour: 2, minute: 30 }).toString());
console.log(Temporal.ZonedDateTime.from("2024-11-03T01:30:00-05:00[America/New_York]").with({ minute: 15 }).toString());
console.log(Temporal.ZonedDateTime.from("2024-01-31T12:00:00[America/New_York]").with({ month: 2 }).toString());

try {
  Temporal.ZonedDateTime.from("2024-01-31T12:00:00[America/New_York]").with({ month: 2 }, { overflow: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

console.log(z.withPlainTime(new Temporal.PlainTime(8, 0)).toString());
console.log(z.withPlainTime().toString());
console.log(z.withTimeZone("Asia/Tokyo").toString());
console.log(z.withCalendar("iso8601").toString());

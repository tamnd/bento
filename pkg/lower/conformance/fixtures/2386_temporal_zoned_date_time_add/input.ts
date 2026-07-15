// Temporal.ZonedDateTime.prototype.add and subtract move the value by a duration, splitting the
// calendar part from the exact-time part. The calendar part adds to the wall-clock reading in the
// calendar and the offset re-resolves through the zone, so a whole day added across the New York
// spring-forward boundary keeps the wall clock at noon and reports -04:00, while an exact
// twenty-four hours stays twenty-four hours on the time line and reads 13:00. A day added into a
// fall-back overlap keeps the wall clock and takes the earlier offset, a stable zone adds exact
// time straight, and an out-of-range day under the reject overflow throws a RangeError. Every
// result was checked against @js-temporal/polyfill.
const z = Temporal.ZonedDateTime.from("2024-03-09T12:00:00-05:00[America/New_York]");
console.log(z.add({ days: 1 }).toString());
console.log(z.add({ hours: 24 }).toString());
console.log(z.add({ months: 1 }).toString());
console.log(z.add({ years: 1, months: 2, days: 3 }).toString());
console.log(z.subtract({ days: 1 }).toString());
console.log(z.subtract({ months: 1 }).toString());

const fallback = Temporal.ZonedDateTime.from("2024-11-02T01:30:00-04:00[America/New_York]");
console.log(fallback.add({ days: 1 }).toString());

const tokyo = Temporal.ZonedDateTime.from("2024-01-01T00:00:00+09:00[Asia/Tokyo]");
console.log(tokyo.add({ minutes: 90 }).toString());

const jan31 = Temporal.ZonedDateTime.from("2024-01-31T00:00:00-05:00[America/New_York]");
console.log(jan31.add({ months: 1 }).toString());

try {
  jan31.add({ months: 1 }, { overflow: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

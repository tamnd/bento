// Temporal.ZonedDateTime.prototype.round rounds the value to a smallestUnit. A time unit rounds the
// wall clock, so a half hour past rounds up and a fifteen-minute increment snaps to the quarter. A
// day unit rounds within the zoned day, whose length the daylight-saving transitions change: on the
// twenty-three-hour spring-forward day noon is under the half-day mark and rounds back to this
// midnight, while on the twenty-five-hour fall-back day noon is past the half-day mark and rounds
// forward to the next. Rounding a value inside the fall-back overlap keeps the offset branch it was
// on, and a roundingIncrement that does not divide its unit throws a RangeError. Every result was
// checked against @js-temporal/polyfill.
const a = Temporal.ZonedDateTime.from("2024-06-15T12:29:00[America/New_York]");
console.log(a.round("hour").toString());
console.log(Temporal.ZonedDateTime.from("2024-06-15T12:30:00[America/New_York]").round("hour").toString());
console.log(a.round({ smallestUnit: "minute", roundingIncrement: 15 }).toString());

console.log(Temporal.ZonedDateTime.from("2024-03-10T12:00:00[America/New_York]").round({ smallestUnit: "day" }).toString());
console.log(Temporal.ZonedDateTime.from("2024-11-03T12:00:00[America/New_York]").round({ smallestUnit: "day" }).toString());
console.log(Temporal.ZonedDateTime.from("2024-06-15T00:00:01[America/New_York]").round({ smallestUnit: "day", roundingMode: "ceil" }).toString());

console.log(Temporal.ZonedDateTime.from("2024-11-03T01:40:00-05:00[America/New_York]").round({ smallestUnit: "hour", roundingMode: "floor" }).toString());

try {
  a.round({ smallestUnit: "hour", roundingIncrement: 7 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

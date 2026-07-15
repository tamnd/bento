// Temporal.Duration.prototype.round rounds at a smallestUnit and rebalances to a largestUnit.
// Without a relativeTo reference the duration rounds over a fixed 24-hour day and may name no
// years, months, or weeks and no calendar unit, so a week largestUnit throws a RangeError. With
// a PlainDate reference every field resolves against the calendar: an irregular smallestUnit
// brackets the endpoint between two unit boundaries, a negative duration rounds in wall-clock
// terms, and the rounded date rebalances to the largestUnit. Every result was checked against
// @js-temporal/polyfill.
const rel = Temporal.PlainDate.from("2024-01-01");
console.log(Temporal.Duration.from({ days: 1, hours: 12 }).round("day").toString());
console.log(Temporal.Duration.from({ days: 1, hours: 2 }).round({ largestUnit: "hour" }).toString());
console.log(Temporal.Duration.from({ minutes: 37 }).round({ smallestUnit: "minute", roundingIncrement: 15 }).toString());
console.log(Temporal.Duration.from({ years: 1, months: 2 }).round({ smallestUnit: "month", relativeTo: rel }).toString());
console.log(Temporal.Duration.from({ days: 20 }).round({ smallestUnit: "week", relativeTo: rel }).toString());
console.log(Temporal.Duration.from({ days: -40 }).round({ smallestUnit: "month", relativeTo: rel }).toString());
console.log(Temporal.Duration.from({ months: 5 }).round({ smallestUnit: "month", roundingIncrement: 2, relativeTo: rel }).toString());
console.log(Temporal.Duration.from({ days: 1, hours: 1, minutes: 40 }).round({ smallestUnit: "hour", relativeTo: rel }).toString());

try {
  Temporal.Duration.from({ days: 1 }).round({ largestUnit: "week" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

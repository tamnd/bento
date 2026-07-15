// Temporal.Instant.prototype.round rounds the exact-time point to a multiple of a time unit. An
// Instant has no calendar, so the smallestUnit runs hour down to nanosecond and the rounding
// aligns to the day: the increment must divide the number of that unit in a 24-hour day, so hour
// accepts up to 24 and nanosecond up to a full day's worth. The argument is a smallestUnit string
// shorthand or an options bag with roundingIncrement and roundingMode, the default mode halfExpand.
// An increment that does not divide the day throws a RangeError. Every result was checked against
// @js-temporal/polyfill.
const b = Temporal.Instant.fromEpochNanoseconds(8130250500000n);
console.log(b.round("hour").toString());
console.log(b.round("minute").toString());
console.log(b.round({ smallestUnit: "second" }).toString());
console.log(b.round({ smallestUnit: "second", roundingMode: "ceil" }).toString());
console.log(b.round({ smallestUnit: "minute", roundingIncrement: 15 }).toString());
console.log(b.round({ smallestUnit: "hour", roundingIncrement: 6 }).toString());
console.log(b.round({ smallestUnit: "hour", roundingIncrement: 24 }).toString());
console.log(b.round({ smallestUnit: "nanosecond", roundingIncrement: 86400000000000 }).toString());

try {
  b.round({ smallestUnit: "hour", roundingIncrement: 5 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

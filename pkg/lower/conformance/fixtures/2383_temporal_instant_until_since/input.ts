// Temporal.Instant.prototype.until and since report the exact-time difference between two
// instants. An Instant carries no calendar, so the units run hour down to nanosecond and the
// default largestUnit is second. The difference balances from largestUnit down and rounds at
// smallestUnit under the rounding mode; since negates the result. A largestUnit smaller than
// the smallestUnit throws a RangeError. Every result was checked against @js-temporal/polyfill.
const a = Temporal.Instant.fromEpochNanoseconds(0n);
const b = Temporal.Instant.fromEpochNanoseconds(8130250500000n);
console.log(a.until(b).toString());
console.log(a.since(b).toString());
console.log(a.until(b, { largestUnit: "hour" }).toString());
console.log(a.until(b, { largestUnit: "minute" }).toString());
console.log(a.until(b, { smallestUnit: "second" }).toString());
console.log(a.until(b, { smallestUnit: "second", roundingMode: "ceil" }).toString());
console.log(a.until(b, { smallestUnit: "minute", roundingIncrement: 15, roundingMode: "floor" }).toString());
console.log(b.until(a).toString());
console.log(a.until(a).toString());

try {
  a.until(b, { largestUnit: "second", smallestUnit: "hour" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

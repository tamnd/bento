// Temporal.PlainTime.from over a property bag reads the named time fields at compile time
// and lays them over a zero base, so an absent field falls to zero. The overflow option
// chooses between clamping an out-of-range field (the default, constrain) and throwing a
// RangeError (reject). PlainTime.prototype.with lays the same bag over the receiver's fields,
// so an absent field holds the receiver's value. The values were checked against
// @js-temporal/polyfill.
console.log(Temporal.PlainTime.from({ hour: 12, minute: 30, second: 15 }).toString());
console.log(Temporal.PlainTime.from({ hour: 25 }).toString());
console.log(Temporal.PlainTime.from({ minute: 90 }).toString());
console.log(Temporal.PlainTime.from({ hour: 5, millisecond: 250 }).toString());

const base = new Temporal.PlainTime(12, 30, 15);
console.log(base.with({ minute: 45 }).toString());
console.log(base.with({ hour: 23, minute: 90 }).toString());

// reject turns an out-of-range field into a RangeError instead of clamping it.
try {
  Temporal.PlainTime.from({ hour: 25 }, { overflow: "reject" });
  console.log("no throw");
} catch (e: any) {
  console.log(e.name);
}

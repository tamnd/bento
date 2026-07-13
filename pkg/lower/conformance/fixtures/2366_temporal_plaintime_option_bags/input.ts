// Group 1's shared option-bag machinery, exercised end to end on the calendar-free PlainTime
// before the calendar-bearing types lean on it: the property-bag reader with the overflow
// option, the arithmetic duration-argument reader, and the rounding-options reader. A time
// is read from a bag, a duration is added to it, and the result is rounded, all with the
// three readers in one program. The values were checked against @js-temporal/polyfill.
const base = Temporal.PlainTime.from({ hour: 8, minute: 15, second: 30 });
const later = base.add({ hours: 5, minutes: 50 });
const rounded = later.round({ smallestUnit: "hour" });
console.log(base.toString());
console.log(later.toString());
console.log(rounded.toString());

// The default overflow constrains an out-of-range field, clamping it to its maximum.
const clamped = Temporal.PlainTime.from({ hour: 25, minute: 70 });
console.log(clamped.toString());

// An explicit reject overflow throws a RangeError on the same out-of-range field.
try {
  Temporal.PlainTime.from({ hour: 25 }, { overflow: "reject" });
  console.log("no throw");
} catch {
  console.log("RangeError");
}

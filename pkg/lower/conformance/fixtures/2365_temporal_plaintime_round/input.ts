// Temporal.PlainTime.prototype.round rounds a wall clock to a whole multiple of a smallest
// unit. The smallest unit is hour, minute, second, millisecond, microsecond, or nanosecond,
// given either as a string shorthand or on the options bag. The roundingIncrement scales the
// unit and must divide the next unit up evenly. The roundingMode picks the neighbour, with
// the default halfExpand rounding a tie away from zero. Rounding an hour up past 23 wraps to
// the next day's clock. An increment that does not evenly divide its unit is a RangeError.
// The values were checked against @js-temporal/polyfill.
const t = new Temporal.PlainTime(3, 34, 56, 987, 654, 321);
console.log(t.round("hour").toString());
console.log(t.round("minute").toString());
console.log(t.round("second").toString());
console.log(t.round("millisecond").toString());
console.log(t.round("microsecond").toString());
console.log(t.round("nanosecond").toString());
console.log(t.round({ smallestUnit: "minute", roundingIncrement: 15 }).toString());
console.log(t.round({ smallestUnit: "minute", roundingIncrement: 30, roundingMode: "ceil" }).toString());
console.log(t.round({ smallestUnit: "hour", roundingMode: "floor" }).toString());
console.log(t.round({ smallestUnit: "hour", roundingMode: "expand" }).toString());

// An hour rounded up past the last hour of the day wraps to the next day's clock.
const late = new Temporal.PlainTime(23, 59);
console.log(late.round({ smallestUnit: "hour", roundingMode: "ceil" }).toString());

// A tie rounds away from zero under the default halfExpand, and to the even neighbour under
// halfEven. Three o'clock plus a half hour ties between three and four; halfEven picks four
// because three is odd.
const tie = new Temporal.PlainTime(3, 30);
console.log(tie.round({ smallestUnit: "hour" }).toString());
console.log(tie.round({ smallestUnit: "hour", roundingMode: "halfEven" }).toString());

// An increment that does not divide its unit evenly is a RangeError at run time, and so is
// an increment that reaches the next unit up.
try {
  t.round({ smallestUnit: "hour", roundingIncrement: 5 });
  console.log("no throw");
} catch {
  console.log("RangeError");
}
try {
  t.round({ smallestUnit: "hour", roundingIncrement: 24 });
  console.log("no throw");
} catch {
  console.log("RangeError");
}

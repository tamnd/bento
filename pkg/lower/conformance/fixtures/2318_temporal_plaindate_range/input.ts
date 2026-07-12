// The Temporal.PlainDate constructor rejects a date outside the ISO calendar or the
// representable range with a RangeError: a month past twelve, a day past the month's
// length, and a date one day beyond the maximum all throw. A NaN component throws too,
// in ToIntegerWithTruncation, rather than settling on zero, so new PlainDate(NaN, 1, 1)
// raises instead of quietly producing the valid 0000-01-01. Catching the throw and
// reading the caught error's constructor name proves the runtime raised the exact
// RangeError the specification requires rather than running to a wrong value.
function ctorName(build: () => void): string {
  try {
    build();
  } catch (thrown: any) {
    return thrown.constructor.name;
  }
  return "no throw";
}

console.log(ctorName(() => {
  new Temporal.PlainDate(2020, 13, 1);
}));
console.log(ctorName(() => {
  new Temporal.PlainDate(2020, 2, 30);
}));
console.log(ctorName(() => {
  new Temporal.PlainDate(275760, 9, 14);
}));
console.log(ctorName(() => {
  new Temporal.PlainDate(NaN, 1, 1);
}));
console.log(ctorName(() => {
  new Temporal.PlainDate(2020, 1, 1);
}));

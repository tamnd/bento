// The Temporal.PlainYearMonth constructor rejects an invalid year-month with a RangeError: a
// month below 1 or above 12, a NaN year (which throws in ToIntegerWithTruncation rather than
// settling on year zero), and a year-month past either representable-range boundary all throw,
// while the year-month at the boundary succeeds. Catching the throw and reading the caught
// error's constructor name proves the runtime raised the exact RangeError the specification
// requires.
function ctorName(build: () => void): string {
  try {
    build();
  } catch (thrown: any) {
    return thrown.constructor.name;
  }
  return "no throw";
}

console.log(ctorName(() => {
  new Temporal.PlainYearMonth(2020, 0);
}));
console.log(ctorName(() => {
  new Temporal.PlainYearMonth(2020, 13);
}));
console.log(ctorName(() => {
  new Temporal.PlainYearMonth(NaN, 1);
}));
console.log(ctorName(() => {
  new Temporal.PlainYearMonth(-271821, 3);
}));
console.log(ctorName(() => {
  new Temporal.PlainYearMonth(275760, 10);
}));
console.log(ctorName(() => {
  new Temporal.PlainYearMonth(2020, 3);
}));

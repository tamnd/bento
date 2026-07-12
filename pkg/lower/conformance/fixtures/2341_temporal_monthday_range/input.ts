// The Temporal.PlainMonthDay constructor rejects an invalid month-day with a RangeError: a
// month above 12, a day past the month's length, a NaN month (which throws in
// ToIntegerWithTruncation), and February 30, which does not exist even in the leap reference
// year, all throw, while February 29 succeeds because the reference year 1972 is a leap year.
// Catching the throw and reading the caught error's constructor name proves the runtime raised
// the exact RangeError the specification requires.
function ctorName(build: () => void): string {
  try {
    build();
  } catch (thrown: any) {
    return thrown.constructor.name;
  }
  return "no throw";
}

console.log(ctorName(() => {
  new Temporal.PlainMonthDay(13, 1);
}));
console.log(ctorName(() => {
  new Temporal.PlainMonthDay(1, 32);
}));
console.log(ctorName(() => {
  new Temporal.PlainMonthDay(NaN, 1);
}));
console.log(ctorName(() => {
  new Temporal.PlainMonthDay(2, 30);
}));
console.log(ctorName(() => {
  new Temporal.PlainMonthDay(2, 29);
}));

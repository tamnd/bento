// The Temporal.PlainDateTime constructor rejects an out-of-range date or time component with
// a RangeError: a month past 12, a day past the month's length, an hour past 23, and a
// sub-second field past 999 all throw, and so does a NaN in either half, in
// ToIntegerWithTruncation. Catching the throw and reading the caught error's constructor
// name proves the runtime raised the exact RangeError the specification requires rather than
// running to a wrong value.
function ctorName(build: () => void): string {
  try {
    build();
  } catch (thrown: any) {
    return thrown.constructor.name;
  }
  return "no throw";
}

console.log(ctorName(() => {
  new Temporal.PlainDateTime(2020, 13, 1);
}));
console.log(ctorName(() => {
  new Temporal.PlainDateTime(2021, 2, 30);
}));
console.log(ctorName(() => {
  new Temporal.PlainDateTime(2020, 1, 1, 24);
}));
console.log(ctorName(() => {
  new Temporal.PlainDateTime(2020, 1, 1, 0, 0, 0, 0, 0, 1000);
}));
console.log(ctorName(() => {
  new Temporal.PlainDateTime(NaN, 1, 1);
}));
console.log(ctorName(() => {
  new Temporal.PlainDateTime(2020, 1, 1, 23, 59, 59, 999, 999, 999);
}));

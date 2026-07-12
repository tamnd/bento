// The Temporal.PlainTime constructor rejects a field outside its ISO range with a
// RangeError: an hour past 23, a minute or second past 59, and a sub-second field past
// 999 all throw. A NaN component throws too, in ToIntegerWithTruncation. Catching the
// throw and reading the caught error's constructor name proves the runtime raised the
// exact RangeError the specification requires rather than running to a wrong value.
function ctorName(build: () => void): string {
  try {
    build();
  } catch (thrown: any) {
    return thrown.constructor.name;
  }
  return "no throw";
}

console.log(ctorName(() => {
  new Temporal.PlainTime(24, 0, 0);
}));
console.log(ctorName(() => {
  new Temporal.PlainTime(0, 60, 0);
}));
console.log(ctorName(() => {
  new Temporal.PlainTime(0, 0, 0, 1000);
}));
console.log(ctorName(() => {
  new Temporal.PlainTime(NaN, 0, 0);
}));
console.log(ctorName(() => {
  new Temporal.PlainTime(23, 59, 59, 999, 999, 999);
}));

// The Temporal.Duration constructor rejects an invalid component with a RangeError: a
// non-integral value (Duration rejects rather than truncates, unlike PlainDate and
// PlainTime), a NaN, a set of components that do not share one sign, a years field at the
// 2^32 bound, and a seconds field at the 2^53 bound all throw, in ToIntegerIfIntegral and
// IsValidDuration. Catching the throw and reading the caught error's constructor name proves
// the runtime raised the exact RangeError the specification requires.
function ctorName(build: () => void): string {
  try {
    build();
  } catch (thrown: any) {
    return thrown.constructor.name;
  }
  return "no throw";
}

console.log(ctorName(() => {
  new Temporal.Duration(1.5);
}));
console.log(ctorName(() => {
  new Temporal.Duration(NaN);
}));
console.log(ctorName(() => {
  new Temporal.Duration(1, -2);
}));
console.log(ctorName(() => {
  new Temporal.Duration(4294967296);
}));
console.log(ctorName(() => {
  new Temporal.Duration(0, 0, 0, 0, 0, 0, 9007199254740992);
}));
console.log(ctorName(() => {
  new Temporal.Duration(1, 2, 3, 4, 5, 6, 7, 8, 9, 10);
}));

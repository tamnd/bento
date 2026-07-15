// Temporal.Duration.prototype.with overlays the present fields of a partial-duration bag onto the
// receiver, each absent field keeping the receiver's value, and Temporal.Duration.from over a bag
// reads the ten optional unit fields with each absent one defaulting to zero. Neither balances, so
// neither needs a relativeTo reference. An empty bag is a TypeError, and a fractional or mixed-sign
// field is a RangeError. Every result was checked against @js-temporal/polyfill.
const base = Temporal.Duration.from({ years: 1, months: 2, days: 3, hours: 4 });
console.log(base.toString());
console.log(base.with({ months: 5 }).toString());
console.log(base.with({ days: 10, hours: 0 }).toString());
console.log(base.with({ years: -1, months: -2, days: -3, hours: -4 }).toString());
console.log(Temporal.Duration.from({ hours: 1, minutes: 30 }).toString());
console.log(Temporal.Duration.from({ days: -2, hours: -3 }).toString());
console.log(Temporal.Duration.from({ minutes: 90 }).toString());

try {
  base.with({});
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

try {
  base.with({ months: 1.5 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

try {
  Temporal.Duration.from({});
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

try {
  Temporal.Duration.from({ months: 1.5 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

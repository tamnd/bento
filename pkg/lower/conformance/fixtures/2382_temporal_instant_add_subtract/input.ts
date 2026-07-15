// Temporal.Instant.prototype.add and subtract fold a duration's time part into the exact
// epoch point, negating the duration for subtract. An instant carries no wall clock or
// calendar, so the calendar units years through days are meaningless and each throws a
// RangeError. The duration reaches the methods as a bag, a Duration value, and a string.
// Every result is the UTC toString and was checked against @js-temporal/polyfill.
const base = Temporal.Instant.fromEpochNanoseconds(1000000000000000000n);
console.log(base.add({ hours: 1 }).toString());
console.log(base.add({ hours: 1, minutes: 30, seconds: 15 }).toString());
console.log(base.add({ nanoseconds: 500 }).toString());
console.log(base.subtract({ hours: 2 }).toString());
console.log(base.add({ hours: -1 }).toString());
console.log(base.add(Temporal.Duration.from("PT30M")).toString());
console.log(base.add("PT1H").toString());

try {
  base.add({ days: 1 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}
try {
  base.add({ weeks: 1 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}
try {
  base.add({ months: 1 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}
try {
  base.subtract({ years: 1 });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

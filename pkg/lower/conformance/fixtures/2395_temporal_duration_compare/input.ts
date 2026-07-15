// Temporal.Duration.compare orders two durations as -1, 0, or 1. Without a relativeTo reference
// neither may carry years, months, or weeks, else a RangeError, and each folds to a signed
// nanosecond count over a fixed 24-hour day. With a PlainDate reference every field resolves
// against the calendar before the two endpoints are ordered. Every result was checked against
// @js-temporal/polyfill.
const rel = Temporal.PlainDate.from("2024-01-01");
console.log(Temporal.Duration.compare(Temporal.Duration.from({ hours: 2 }), Temporal.Duration.from({ minutes: 90 })));
console.log(Temporal.Duration.compare(Temporal.Duration.from({ days: 1 }), Temporal.Duration.from({ hours: 24 })));
console.log(Temporal.Duration.compare(Temporal.Duration.from({ months: 1 }), Temporal.Duration.from({ days: 20 }), { relativeTo: rel }));
console.log(Temporal.Duration.compare(Temporal.Duration.from({ months: 1 }), Temporal.Duration.from({ days: 31 }), { relativeTo: rel }));
console.log(Temporal.Duration.compare(Temporal.Duration.from({ years: 1 }), Temporal.Duration.from({ days: 365 }), { relativeTo: rel }));

try {
  Temporal.Duration.compare(Temporal.Duration.from({ months: 1 }), Temporal.Duration.from({ days: 1 }));
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

// Temporal.Duration.prototype.total converts a duration to a single unit as a Number. Without a
// relativeTo reference the duration must be day-or-finer with no calendar units and the unit must
// be day or smaller, a day counting as a fixed 24 hours; a week, month, or year unit, or a
// years, months, or weeks field, throws a RangeError. With a PlainDate reference the fixed units
// divide the span and the irregular month and year interpolate between their boundaries. Every
// result was checked against @js-temporal/polyfill.
const rel = Temporal.PlainDate.from("2024-01-01");
console.log(Temporal.Duration.from({ days: 1, hours: 1 }).total("hour"));
console.log(Temporal.Duration.from({ days: 1, hours: 12 }).total("day"));
console.log(Temporal.Duration.from({ years: 1, months: 2 }).total({ unit: "month", relativeTo: rel }));
console.log(Temporal.Duration.from({ days: 20 }).total({ unit: "week", relativeTo: rel }));
console.log(Temporal.Duration.from({ months: 18 }).total({ unit: "year", relativeTo: rel }));
console.log(Temporal.Duration.from({ months: -1, days: -15 }).total({ unit: "month", relativeTo: rel }));

try {
  Temporal.Duration.from({ weeks: 2 }).total("day");
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

try {
  Temporal.Duration.from({ days: 20 }).total({ unit: "week" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

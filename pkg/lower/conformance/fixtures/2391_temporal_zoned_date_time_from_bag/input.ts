// Temporal.ZonedDateTime.from over a property bag reads the date and time fields into a wall clock,
// pins it to the timeZone, and folds it to an instant. A bag with no offset field resolves through
// the zone under the disambiguation option: an ordinary reading is unambiguous, a spring-forward gap
// shifts forward under compatible and back under earlier, and a fall-back overlap takes the earlier
// branch under compatible and the second under later. A bag that carries an offset field weighs it
// under the offset option: reject demands a matching zone offset, use takes the offset at face value,
// ignore drops it and disambiguates, and prefer keeps a match and otherwise disambiguates. overflow
// constrains an out-of-range field, and a gap under reject or a non-matching offset under reject
// throws a RangeError. Every result was checked against @js-temporal/polyfill.
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 6, day: 15, hour: 12, minute: 30, timeZone: "America/New_York" }).toString());
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 3, day: 10, hour: 2, minute: 30, timeZone: "America/New_York" }).toString());
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 3, day: 10, hour: 2, minute: 30, timeZone: "America/New_York" }, { disambiguation: "earlier" }).toString());
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 11, day: 3, hour: 1, minute: 30, timeZone: "America/New_York" }).toString());
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 11, day: 3, hour: 1, minute: 30, timeZone: "America/New_York" }, { disambiguation: "later" }).toString());
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 11, day: 3, hour: 1, minute: 30, timeZone: "America/New_York", offset: "-05:00" }).toString());
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 11, day: 3, hour: 1, minute: 30, timeZone: "America/New_York", offset: "+05:00" }, { offset: "ignore" }).toString());
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 11, day: 3, hour: 1, minute: 30, timeZone: "America/New_York", offset: "+05:00" }, { offset: "prefer" }).toString());
console.log(Temporal.ZonedDateTime.from({ year: 2024, month: 13, day: 15, hour: 12, timeZone: "America/New_York" }, { overflow: "constrain" }).toString());

try {
  Temporal.ZonedDateTime.from({ year: 2024, month: 3, day: 10, hour: 2, minute: 30, timeZone: "America/New_York" }, { disambiguation: "reject" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

try {
  Temporal.ZonedDateTime.from({ year: 2024, month: 11, day: 3, hour: 1, minute: 30, timeZone: "America/New_York", offset: "+05:00" });
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

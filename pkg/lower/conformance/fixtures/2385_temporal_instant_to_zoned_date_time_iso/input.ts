// Temporal.Instant.prototype.toZonedDateTimeISO pairs an exact instant with a time zone under the
// ISO 8601 calendar, giving the count a wall-clock reading. The zone's rules resolve the offset in
// force at the instant, so a named zone reports its own offset, a numeric offset zone reports
// itself, and an unrecognized identifier throws a RangeError. Every result was checked against
// @js-temporal/polyfill.
const i = Temporal.Instant.fromEpochNanoseconds(1000000000000000000n); // 2001-09-09T01:46:40Z
console.log(i.toZonedDateTimeISO("UTC").toString());
console.log(i.toZonedDateTimeISO("America/New_York").toString());
console.log(i.toZonedDateTimeISO("Asia/Tokyo").toString());
console.log(i.toZonedDateTimeISO("+05:30").toString());

const z = i.toZonedDateTimeISO("Asia/Tokyo");
console.log(z.calendarId);
console.log(z.timeZoneId);
console.log(z.hour);
console.log(z.offset);

try {
  i.toZonedDateTimeISO("Not/AZone");
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

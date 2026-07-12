// compare orders two ZonedDateTimes by their exact time alone, so the same instant in two
// zones compares equal, while equals also weighs the zone and calls them different. from
// copies a ZonedDateTime, and the conversions drop the zone to an Instant or narrow to a
// plain date, time, or date-time in the zone.
const utc = new Temporal.ZonedDateTime(0n, "UTC");
const ny = new Temporal.ZonedDateTime(0n, "America/New_York");
const later = new Temporal.ZonedDateTime(1000000000n, "UTC");
console.log(Temporal.ZonedDateTime.compare(utc, ny));
console.log(Temporal.ZonedDateTime.compare(utc, later));
console.log(utc.equals(ny));
console.log(utc.equals(new Temporal.ZonedDateTime(0n, "UTC")));
const copy = Temporal.ZonedDateTime.from(ny);
console.log(copy.equals(ny));
console.log(ny.toInstant().toString());
console.log(ny.toPlainDate().toString());
console.log(ny.toPlainTime().toString());
console.log(ny.toPlainDateTime().toString());

// Temporal.Now reads the clock. Each function returns the current time in one of the Temporal
// shapes: instant an exact point, timeZoneId the host's default zone, and the four ISO functions
// the current time as a ZonedDateTime, PlainDateTime, PlainDate, or PlainTime, in the default
// zone or in a named one. The clock is non-deterministic in general, so this fixture pins it: the
// harness sets BENTO_NOW_NS to a fixed nanosecond count and TZ to UTC, and the runtime reads
// those, so every function prints a value the oracle can carry. The fixed instant is
// 2023-11-14T22:13:20.123456789Z.
const i = Temporal.Now.instant();
console.log(i.toString());
console.log(Temporal.Now.timeZoneId());
console.log(Temporal.Now.zonedDateTimeISO().toString());
console.log(Temporal.Now.plainDateTimeISO().toString());
console.log(Temporal.Now.plainDateISO().toString());
console.log(Temporal.Now.plainTimeISO().toString());
console.log(Temporal.Now.zonedDateTimeISO("America/New_York").toString());
console.log(Temporal.Now.plainDateTimeISO("America/New_York").toString());
console.log(Temporal.Now.plainTimeISO("America/New_York").toString());

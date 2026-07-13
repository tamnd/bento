// PlainDate.toZonedDateTime pins a calendar date to a time zone, giving an exact instant. Passing
// just a time-zone string uses midnight; a {timeZone, plainTime} bag sets the wall-clock time. The
// zone's rules resolve the offset, so a named zone reports its own offset and a non-ISO calendar
// rides along into the result.
const d = new Temporal.PlainDate(2020, 3, 14);
console.log(d.toZonedDateTime("UTC").toString());
console.log(d.toZonedDateTime("UTC").epochMilliseconds);

const t = new Temporal.PlainTime(15, 30, 45);
console.log(d.toZonedDateTime({ timeZone: "America/New_York", plainTime: t }).toString());

// Spring-forward gap: 2020-03-08 02:30 does not exist in New York, so it shifts forward to 03:30.
const gap = new Temporal.PlainDate(2020, 3, 8);
console.log(gap.toZonedDateTime({ timeZone: "America/New_York", plainTime: new Temporal.PlainTime(2, 30) }).toString());

const roc = d.withCalendar("roc");
const rz = roc.toZonedDateTime("UTC");
console.log(rz.toString());
console.log(rz.calendarId);
console.log(rz.year);

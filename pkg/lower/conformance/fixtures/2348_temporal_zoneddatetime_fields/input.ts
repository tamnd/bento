// Temporal.ZonedDateTime pairs an exact time, the same nanosecond count an Instant holds,
// with a time zone that gives the count a wall-clock reading. The exact-time getters read the
// instant, and the wall-clock getters and the offset read the local time in the zone, which
// shifts across a daylight-saving transition. The default toString renders the local ISO 8601
// date-time, the offset at the instant, and the zone identifier in brackets.
const z = new Temporal.ZonedDateTime(0n, "UTC");
console.log(z.epochMilliseconds);
console.log(z.timeZoneId);
console.log(z.calendarId);
console.log(z.year);
console.log(z.hour);
console.log(z.offset);
console.log(z.toString());
const ny = new Temporal.ZonedDateTime(0n, "America/New_York");
console.log(ny.day);
console.log(ny.hour);
console.log(ny.offset);
console.log(ny.toString());
const summer = new Temporal.ZonedDateTime(1719792000000000000n, "America/New_York");
console.log(summer.hour);
console.log(summer.offset);
console.log(summer.toString());
const off = new Temporal.ZonedDateTime(0n, "+05:30");
console.log(off.hour);
console.log(off.minute);
console.log(off.offset);
console.log(off.toString());

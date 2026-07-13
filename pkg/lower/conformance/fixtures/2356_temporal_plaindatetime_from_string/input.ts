// Temporal.PlainDateTime.from over a string parses the ISO 8601 date-time. It reads a
// date-only string, whose time defaults to midnight, and a full date-time string, whose time
// it keeps, in the extended and basic forms with an optional sub-second fraction. An offset
// and a time-zone annotation are validated and dropped, a :60 leap second is constrained to
// :59, and a calendar annotation names the calendar the result carries, since a PlainDateTime
// has one. The values were checked against @js-temporal/polyfill.
const dateOnly = Temporal.PlainDateTime.from("2024-06-30");
console.log(dateOnly.toString());

const dateTime = Temporal.PlainDateTime.from("2024-06-30T12:30:45");
console.log(dateTime.toString(), dateTime.hour, dateTime.minute, dateTime.second);

const basic = Temporal.PlainDateTime.from("20240630T123045");
console.log(basic.toString());

const frac = Temporal.PlainDateTime.from("2024-06-30T12:30:45.123456789");
console.log(frac.toString(), frac.millisecond, frac.microsecond, frac.nanosecond);

const offset = Temporal.PlainDateTime.from("2024-06-30T08:15:30-05:00[America/New_York]");
console.log(offset.toString());

const leap = Temporal.PlainDateTime.from("2024-06-30T23:59:60");
console.log(leap.toString());

const cal = Temporal.PlainDateTime.from("2024-06-30T06:00:00[u-ca=japanese]");
console.log(cal.toString(), cal.calendarId, cal.era, cal.eraYear);

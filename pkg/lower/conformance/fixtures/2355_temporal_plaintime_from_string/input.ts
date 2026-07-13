// Temporal.PlainTime.from over a string parses the ISO 8601 wall-clock time. It reads the
// extended HH:MM:SS form and the basic HHMMSS form, a leading T time designator, a
// sub-second fraction, and a full date-time string whose date it validates and whose time
// it keeps. An offset and a time-zone annotation are validated and dropped, a :60 leap
// second is constrained to :59, and a calendar annotation is ignored whatever it names
// since a PlainTime carries no calendar. The values were checked against
// @js-temporal/polyfill.
const a = Temporal.PlainTime.from("12:30:45");
console.log(a.toString(), a.hour, a.minute, a.second);

const basic = Temporal.PlainTime.from("123045");
console.log(basic.toString());

const desig = Temporal.PlainTime.from("T12:30");
console.log(desig.toString());

const frac = Temporal.PlainTime.from("12:30:45.123456789");
console.log(frac.toString(), frac.millisecond, frac.microsecond, frac.nanosecond);

const fromDateTime = Temporal.PlainTime.from("2024-06-30T08:15:30");
console.log(fromDateTime.toString());

const offset = Temporal.PlainTime.from("12:30:00+05:30[America/New_York]");
console.log(offset.toString());

const leap = Temporal.PlainTime.from("23:59:60");
console.log(leap.toString());

const cal = Temporal.PlainTime.from("2024-06-30T06:00:00[u-ca=japanese]");
console.log(cal.toString());

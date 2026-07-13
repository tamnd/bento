// Temporal.PlainDate.from over a string parses the ISO 8601 calendar date. It reads the
// extended YYYY-MM-DD form and the basic YYYYMMDD form, the signed six-digit expanded year
// outside 0..9999, and a full date-time string whose time, offset, and time-zone annotation
// it validates and then drops since a PlainDate keeps only the date. A [u-ca=<id>]
// annotation names the calendar, so a from over a literal toString round-trips the
// calendar. The values were checked against @js-temporal/polyfill.
const a = Temporal.PlainDate.from("2024-06-30");
console.log(a.toString(), a.year, a.month, a.day);

const basic = Temporal.PlainDate.from("20240630");
console.log(basic.toString());

const expanded = Temporal.PlainDate.from("+002024-06-30");
console.log(expanded.toString());

const negative = Temporal.PlainDate.from("-000005-06-30");
console.log(negative.toString(), negative.year);

const withTime = Temporal.PlainDate.from("2024-06-30T12:00:00+05:30[America/New_York]");
console.log(withTime.toString());

const greg = Temporal.PlainDate.from("2024-06-30[u-ca=gregory]");
console.log(greg.toString(), greg.calendarId, greg.era);

const roc = Temporal.PlainDate.from("2024-06-30T23:59:59[u-ca=roc]");
console.log(roc.toString(), roc.calendarId, roc.year);

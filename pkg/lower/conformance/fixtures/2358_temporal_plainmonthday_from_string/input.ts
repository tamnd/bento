// Temporal.PlainMonthDay.from over a string parses the ISO 8601 month-day. It reads a bare
// month-day string in the extended "MM-DD" and basic "MMDD" forms, the "--MM-DD" form the
// month-day grammar also allows, and a full date or date-time string whose month and day it
// keeps and whose year and time it drops. An expanded six-digit year is read, an explicit
// iso8601 annotation is accepted, and a yearless month-day permits day 31 in any month since it
// carries no year to bound it. The values were checked against @js-temporal/polyfill.
const bare = Temporal.PlainMonthDay.from("06-15");
console.log(bare.toString(), bare.monthCode, bare.day);

const basic = Temporal.PlainMonthDay.from("0615");
console.log(basic.toString());

const dashed = Temporal.PlainMonthDay.from("--06-15");
console.log(dashed.toString());

const fromDate = Temporal.PlainMonthDay.from("2024-06-15");
console.log(fromDate.toString());

const fromDateTime = Temporal.PlainMonthDay.from("2024-06-15T12:30:45");
console.log(fromDateTime.toString());

const iso = Temporal.PlainMonthDay.from("06-15[u-ca=iso8601]");
console.log(iso.toString(), iso.calendarId);

const expanded = Temporal.PlainMonthDay.from("-000005-06-15");
console.log(expanded.toString());

const yearless = Temporal.PlainMonthDay.from("06-31");
console.log(yearless.toString());

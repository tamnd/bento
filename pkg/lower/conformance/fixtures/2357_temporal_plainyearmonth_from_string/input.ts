// Temporal.PlainYearMonth.from over a string parses the ISO 8601 year-month. It reads a bare
// year-month string, whose day the type does not carry, in the extended "YYYY-MM" and basic
// "YYYYMM" forms, and a full date or date-time string whose year and month it keeps and whose
// day and time it drops. An expanded six-digit year is read, an explicit iso8601 annotation is
// accepted, and an out-of-range month or day throws. The values were checked against
// @js-temporal/polyfill.
const bare = Temporal.PlainYearMonth.from("2024-06");
console.log(bare.toString(), bare.year, bare.month);

const basic = Temporal.PlainYearMonth.from("202406");
console.log(basic.toString());

const fromDate = Temporal.PlainYearMonth.from("2024-06-30");
console.log(fromDate.toString());

const fromDateTime = Temporal.PlainYearMonth.from("2024-06-30T12:30:45");
console.log(fromDateTime.toString());

const iso = Temporal.PlainYearMonth.from("2024-06[u-ca=iso8601]");
console.log(iso.toString(), iso.calendarId);

const expanded = Temporal.PlainYearMonth.from("-000005-06");
console.log(expanded.toString());

const code = Temporal.PlainYearMonth.from("2024-06");
console.log(code.monthCode, code.daysInMonth, code.inLeapYear);

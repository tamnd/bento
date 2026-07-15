// Temporal.PlainYearMonth.prototype.era and eraYear resolve the era at the first of the year-month's
// month, so a year-month reports the same era the date it came from does. Both are undefined under
// the ISO calendar. gregory names its era from the sign of the year, roc counts its year from 1912,
// and japanese resolves the nengo the month falls in. The year-months here come from a date through
// withCalendar and toPlainYearMonth. Every result was checked against @js-temporal/polyfill.
const iso = new Temporal.PlainDate(2020, 3, 15).toPlainYearMonth();
console.log(iso.era);
console.log(iso.eraYear);
const greg = new Temporal.PlainDate(2020, 3, 15).withCalendar("gregory").toPlainYearMonth();
console.log(greg.era);
console.log(greg.eraYear);
const gregBC = new Temporal.PlainDate(0, 3, 15).withCalendar("gregory").toPlainYearMonth();
console.log(gregBC.era);
console.log(gregBC.eraYear);
const roc = new Temporal.PlainDate(2020, 3, 15).withCalendar("roc").toPlainYearMonth();
console.log(roc.era);
console.log(roc.eraYear);
const jp = new Temporal.PlainDate(2020, 3, 15).withCalendar("japanese").toPlainYearMonth();
console.log(jp.era);
console.log(jp.eraYear);

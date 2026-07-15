// PlainDateTime.toLocaleString still hands back in this slice: a locale-aware rendering needs
// Intl, which bento does not lower yet, so the compiler reports the ceiling rather than emit a
// wrong string. The with, withPlainTime, withCalendar, toPlainDate, toPlainTime, and
// toZonedDateTime reshaping and conversion methods, and the add, subtract, until, since, and round
// arithmetic, all lower.
const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);
console.log(dt.toLocaleString());

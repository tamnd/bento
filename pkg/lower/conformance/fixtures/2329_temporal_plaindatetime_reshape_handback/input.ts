// PlainDateTime reshaping and conversion still hand back in this slice: with overlays a
// property bag, withPlainTime and withPlainDate swap a half, and toPlainDate,
// toPlainTime, toPlainYearMonth, toPlainMonthDay, and toZonedDateTime need the other
// Temporal types. Each waits on later work, so the compiler reports the ceiling rather
// than emit a wrong date-time. The add, subtract, until, since, and round methods lower.
const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);
const shifted = dt.with({ hour: 9 });
console.log(shifted.hour);

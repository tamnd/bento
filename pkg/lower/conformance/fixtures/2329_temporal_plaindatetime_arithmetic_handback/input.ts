// PlainDateTime arithmetic hands back in this slice: add and subtract need a Duration and
// the date-and-time carry math, until and since return a Duration, round takes an options
// bag, with reshapes through a property bag, and the toPlainDate and toZonedDateTime
// conversions need the other Temporal types. Each waits on later work, so the compiler
// reports the ceiling rather than emit a wrong date-time.
const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);
const later = dt.add({ hours: 1 });
console.log(later.hour);

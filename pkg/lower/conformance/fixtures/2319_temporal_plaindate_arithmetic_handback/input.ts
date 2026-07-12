// PlainDate arithmetic hands back in this slice: add and subtract need a Duration and
// the ISO date math to balance overflowing fields, until and since return a Duration,
// and with reshapes through a property bag. Each waits on the Duration type and the
// calendar field math, so the compiler reports the ceiling rather than emit a wrong
// date.
const d = new Temporal.PlainDate(2020, 2, 29);
const later = d.add({ days: 1 });
console.log(later.day);

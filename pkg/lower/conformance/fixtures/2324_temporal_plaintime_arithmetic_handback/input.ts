// PlainTime arithmetic hands back in this slice: add and subtract need a Duration and the
// wrap-around time math, until and since return a Duration, round takes an options bag,
// and with reshapes through a property bag. Each waits on the Duration type and options
// parsing, so the compiler reports the ceiling rather than emit a wrong time.
const t = new Temporal.PlainTime(12, 30);
const later = t.add({ hours: 1 });
console.log(later.hour);

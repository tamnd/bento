// PlainTime difference arithmetic hands back in this slice: until and since return a
// Duration between two times, and that leans on the units-and-rounding options bag the
// compiler cannot read yet. add, subtract, and with already lower, so the ceiling here is
// the difference math, and the compiler reports it rather than emit a wrong Duration.
const t = new Temporal.PlainTime(12, 30);
const b = new Temporal.PlainTime(14, 0);
const d = t.until(b);
console.log(d.hours);

// until and since return a Duration between two times. A Temporal.PlainTime argument already
// lowers to the difference math, but the methods also accept a plain-time-like bag or an ISO
// string that must be coerced to a PlainTime first. That coercion is a later slice, so a
// non-PlainTime argument hands back here rather than emit a wrong Duration.
const t = new Temporal.PlainTime(12, 30);
const d = t.until("14:00");
console.log(d.hours);

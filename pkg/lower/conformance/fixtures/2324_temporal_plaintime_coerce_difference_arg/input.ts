// until and since return a Duration between two times, and compare and equals order and test
// two times. Besides a Temporal.PlainTime argument, each accepts a plain-time-like bag or an
// ISO string, coercing it to a PlainTime first through ToTemporalTime, so the result is measured
// against the time the argument names rather than handing back.
const t = new Temporal.PlainTime(12, 30);
console.log(t.until("14:00:00").hours);
console.log(t.until({ hour: 15 }).hours);
console.log(Temporal.PlainTime.compare("12:30:00", "14:00:00"));
console.log(t.equals("12:30:00"));

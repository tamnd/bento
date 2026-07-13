// PlainDate.toPlainDateTime widens a date to a date-time by pairing it with a wall clock,
// which needs the PlainTime coercion and the combined type a later slice carries. It hands
// back here so the compiler reports the ceiling rather than emit a wrong value. add,
// subtract, until, since, and with already lower.
const d = new Temporal.PlainDate(2020, 2, 29);
const dt = d.toPlainDateTime();
console.log(dt.day);

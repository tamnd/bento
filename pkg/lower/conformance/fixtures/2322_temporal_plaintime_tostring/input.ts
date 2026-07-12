// Temporal.PlainTime.toString renders HH:MM:SS and appends a fractional-second part only
// when a sub-second field is set, trimmed to the fewest digits: a whole-second time has
// no fraction, 250 milliseconds reads as .25, and the all-nines sub-second reads to full
// nanosecond precision. toJSON produces the same string.
const whole = new Temporal.PlainTime(12, 30, 0);
console.log(whole.toString());
console.log(new Temporal.PlainTime(12, 30, 0, 250).toString());
console.log(new Temporal.PlainTime(1, 2, 3, 4, 5, 6).toString());
console.log(new Temporal.PlainTime(23, 59, 59, 999, 999, 999).toString());
console.log(whole.toJSON());

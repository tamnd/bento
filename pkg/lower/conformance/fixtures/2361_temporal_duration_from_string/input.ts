// Temporal.Duration.from over a string parses the ISO 8601 duration grammar. It reads a full
// duration with every unit, a fractional hours part cascading into whole minutes, a negative
// duration, a sub-second fraction spread across milliseconds, microseconds, and nanoseconds,
// and an all-zero duration rendering as PT0S. Every result is the toString and a field or two;
// the values were checked against @js-temporal/polyfill.
const full = Temporal.Duration.from("P1Y2M3W4DT5H6M7.5S");
console.log(full.toString(), full.days, full.seconds, full.milliseconds);

const frac = Temporal.Duration.from("PT1.5H");
console.log(frac.toString(), frac.hours, frac.minutes);

const neg = Temporal.Duration.from("-P2DT30M");
console.log(neg.toString(), neg.sign);

const sub = Temporal.Duration.from("PT1.234567891S");
console.log(sub.toString(), sub.milliseconds, sub.microseconds, sub.nanoseconds);

const zero = Temporal.Duration.from("P0D");
console.log(zero.toString(), zero.blank);

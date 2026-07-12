// Temporal.Duration: construction from up to ten signed component counts, then the field
// getters plus the derived sign and blank. A duration with every field set reads back each
// count, reports sign 1, and is not blank; an empty duration reports sign 0 and is blank.
// The values match @js-temporal/polyfill.
const d = new Temporal.Duration(1, 2, 3, 4, 5, 6, 7, 8, 9, 10);
console.log(d.years);
console.log(d.months);
console.log(d.weeks);
console.log(d.days);
console.log(d.hours);
console.log(d.minutes);
console.log(d.seconds);
console.log(d.milliseconds);
console.log(d.microseconds);
console.log(d.nanoseconds);
console.log(d.sign);
console.log(d.blank);
const empty = new Temporal.Duration();
console.log(empty.sign);
console.log(empty.blank);

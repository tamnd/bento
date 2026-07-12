// Temporal.Duration.negated flips the sign of every field, and abs makes every field
// non-negative; neither mutates the receiver. negated of a positive duration reports sign -1,
// and abs of a negative duration reports sign 1. The values match @js-temporal/polyfill.
const d = new Temporal.Duration(1, 2, 3, 4, 5, 6, 7, 8, 9, 10);
console.log(d.negated().toString());
console.log(d.negated().sign);
console.log(d.abs().toString());
const neg = new Temporal.Duration(0, 0, 0, -4);
console.log(neg.abs().toString());
console.log(neg.abs().sign);

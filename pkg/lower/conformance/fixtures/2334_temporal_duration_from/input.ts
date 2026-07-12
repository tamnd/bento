// Temporal.Duration.from over a Duration returns a fresh Duration with the same fields, the
// copy the specification makes. Reading the copy's toString and a field back proves it
// carries the source's components. The values match @js-temporal/polyfill.
const a = new Temporal.Duration(1, 2, 3, 4);
const b = Temporal.Duration.from(a);
console.log(b.toString());
console.log(b.days);

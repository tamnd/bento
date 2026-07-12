// The Instant statics build an exact time from the epoch and order two of them. compare
// returns the sign of the difference, from copies an Instant, and the two epoch factories
// build from a millisecond number or a nanosecond bigint. equals compares two instants for
// the same count.
const a = new Temporal.Instant(1000000000n);
const b = Temporal.Instant.fromEpochMilliseconds(2000);
console.log(Temporal.Instant.compare(a, b));
console.log(Temporal.Instant.compare(b, a));
console.log(a.equals(a));
console.log(a.equals(b));
const c = Temporal.Instant.from(a);
console.log(c.equals(a));
console.log(b.toString());
const d = Temporal.Instant.fromEpochNanoseconds(5000000000n);
console.log(d.toString());

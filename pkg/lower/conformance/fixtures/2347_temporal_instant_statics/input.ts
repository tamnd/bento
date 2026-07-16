// The Instant statics build an exact time from the epoch and order two of them. compare
// returns the sign of the difference, from copies an Instant, and the two epoch factories
// build from a millisecond number or a nanosecond bigint. equals compares two instants for
// the same count. Besides an Instant argument, compare and equals accept a string, coercing it to
// an Instant first through ToTemporalInstant, so the comparison measures against the instant the
// string names rather than handing back.
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
console.log(Temporal.Instant.compare("1970-01-01T00:00:00Z", "1970-01-01T00:00:01Z"));
console.log(a.equals("1970-01-01T00:00:01Z"));

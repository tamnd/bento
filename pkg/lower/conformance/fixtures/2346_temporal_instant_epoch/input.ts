// Temporal.Instant is an exact point on the UTC time line, a nanosecond count since the
// epoch. The epoch getters read the count as whole milliseconds and as the bigint itself,
// and the default toString renders the UTC ISO 8601 string with a Z designator, dropping a
// fractional second when the count lands on a whole second and borrowing into the previous
// day for a negative count.
const a = new Temporal.Instant(1000000000n);
console.log(a.epochMilliseconds);
console.log(a.epochNanoseconds.toString());
console.log(a.toString());
console.log(a.toJSON());
const frac = new Temporal.Instant(123456789n);
console.log(frac.toString());
const neg = new Temporal.Instant(-1n);
console.log(neg.toString());
console.log(neg.epochMilliseconds);

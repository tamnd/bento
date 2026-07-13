// Temporal.ZonedDateTime.from over a string resolves a wall clock in a named time zone to an
// exact instant. It reads a plain wall clock (no offset, disambiguated 'compatible'), a
// Z-designated string read back in the zone, the earlier reading in a fall-back overlap, the
// forward shift across a spring-forward gap, and a fixed-offset zone. Every result is the
// toString with the resolved offset and the epoch milliseconds; the values were checked
// against @js-temporal/polyfill.
const wall = Temporal.ZonedDateTime.from("2020-06-15T12:30:00[America/New_York]");
console.log(wall.toString(), wall.epochMilliseconds);

const zulu = Temporal.ZonedDateTime.from("2020-01-01T12:00:00Z[America/New_York]");
console.log(zulu.toString(), zulu.epochMilliseconds);

const overlap = Temporal.ZonedDateTime.from("2020-11-01T01:30:00[America/New_York]");
console.log(overlap.toString(), overlap.epochMilliseconds);

const gap = Temporal.ZonedDateTime.from("2020-03-08T02:30:00[America/New_York]");
console.log(gap.toString(), gap.epochMilliseconds);

const fixed = Temporal.ZonedDateTime.from("1970-01-01T00:00:00+00:00[UTC]");
console.log(fixed.toString(), fixed.epochMilliseconds);

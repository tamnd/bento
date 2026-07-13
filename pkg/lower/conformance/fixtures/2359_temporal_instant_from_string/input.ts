// Temporal.Instant.from over a string parses an exact UTC point. It reads a Z-designated
// instant, applies a negative and a sub-minute positive UTC offset to recover the UTC time,
// reads an expanded negative year, and ignores a calendar annotation since an instant carries
// no calendar. Every result is the UTC toString and the epoch milliseconds; the values were
// checked against @js-temporal/polyfill.
const utc = Temporal.Instant.from("2020-01-01T12:30:45Z");
console.log(utc.toString(), utc.epochMilliseconds);

const west = Temporal.Instant.from("2020-01-01T12:30:45-05:00");
console.log(west.toString(), west.epochMilliseconds);

const subMinute = Temporal.Instant.from("2020-01-01T00:00:00+05:00:30");
console.log(subMinute.toString(), subMinute.epochMilliseconds);

const expanded = Temporal.Instant.from("-000001-01-01T00:00:00Z");
console.log(expanded.toString(), expanded.epochMilliseconds);

const annotated = Temporal.Instant.from("2020-01-01T00:00:00Z[u-ca=hebrew]");
console.log(annotated.toString(), annotated.epochMilliseconds);

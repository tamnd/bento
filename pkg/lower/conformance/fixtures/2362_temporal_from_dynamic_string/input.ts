// Temporal.X.from over a string value not known until compile time routes through the same
// runtime parser a literal takes. A function parameter typed string is a dynamic string, so
// each call reads its Go string and parses at run time. Only the three calendar-free types are
// safe this way: PlainTime and Instant ignore any calendar the string names, and a Duration
// carries no calendar, so a dynamic string can never name a calendar bento cannot represent.
// The values were checked against @js-temporal/polyfill.
function durOf(s: string): string { return Temporal.Duration.from(s).toString(); }
function instMs(s: string): number { return Temporal.Instant.from(s).epochMilliseconds; }
function timeOf(s: string): string { return Temporal.PlainTime.from(s).toString(); }

console.log(durOf("PT1.5H"));
console.log(durOf("-P1Y2M"));
console.log(instMs("1970-01-01T00:00:01Z"));
console.log(timeOf("23:59:59.5"));
// A non-ISO calendar annotation is accepted and ignored, the spec-correct result a compile-time
// gate could not prove for a dynamic string.
console.log(timeOf("2024-06-30T12:30:45[u-ca=hebrew]"));

// Temporal.ZonedDateTime.prototype.until and since return the difference between two zoned
// date-times as a Duration balanced from the largestUnit down. A time-unit largestUnit counts the
// real elapsed time, so three calendar days across the New York spring-forward boundary are
// seventy-one hours, one short of seventy-two because a day on the transition is twenty-three hours
// long. The default largestUnit for a ZonedDateTime difference is hour, not day, so until with no
// options reports the same seventy-one hours. A calendar largestUnit splits the distance into a
// date part and an exact-time part that share one sign, weeks fold into days when only days are
// asked for, the difference reverses sign when the endpoints swap, and since is the negation of
// until. A time-of-day that falls short borrows a day, and a longer span in Tokyo counts months and
// days and the leftover hours. Every result was checked against @js-temporal/polyfill.
const a = Temporal.ZonedDateTime.from("2024-03-08T12:00:00[America/New_York]");
const b = Temporal.ZonedDateTime.from("2024-03-11T12:00:00[America/New_York]");
console.log(a.until(b).toString());
console.log(a.until(b, { largestUnit: "days" }).toString());
console.log(a.until(b, { largestUnit: "weeks" }).toString());
console.log(b.until(a, { largestUnit: "days" }).toString());
console.log(a.since(b, { largestUnit: "days" }).toString());

const early = Temporal.ZonedDateTime.from("2024-03-11T10:00:00[America/New_York]");
console.log(a.until(early, { largestUnit: "days" }).toString());
console.log(a.until(early, { largestUnit: "hours" }).toString());

const t1 = Temporal.ZonedDateTime.from("2024-01-01T00:00:00[Asia/Tokyo]");
const t2 = Temporal.ZonedDateTime.from("2024-04-06T06:30:00[Asia/Tokyo]");
console.log(t1.until(t2, { largestUnit: "months" }).toString());
console.log(t1.until(t2, { largestUnit: "years" }).toString());

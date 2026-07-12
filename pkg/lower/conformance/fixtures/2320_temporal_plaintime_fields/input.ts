// Temporal.PlainTime: construction from up to six number components and the six clean
// field getters. A time with every field set exercises the hour, minute, and second
// alongside the three sub-second fields. PlainTime carries no calendar and no zone, so
// there are no calendar-dependent getters here at all.
const t = new Temporal.PlainTime(1, 2, 3, 4, 5, 6);
console.log(t.hour);
console.log(t.minute);
console.log(t.second);
console.log(t.millisecond);
console.log(t.microsecond);
console.log(t.nanosecond);

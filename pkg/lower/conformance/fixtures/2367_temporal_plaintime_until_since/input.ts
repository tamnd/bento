const a = new Temporal.PlainTime(12, 30, 0);
const b = new Temporal.PlainTime(14, 45, 30, 250);

// until(b) is b - a, a positive duration going forward in the day.
console.log(a.until(b).toString());
// since(b) is a - b, the same magnitude with the opposite sign.
console.log(a.since(b).toString());

// largestUnit rolls the whole gap into minutes.
console.log(a.until(b, { largestUnit: "minute" }).toString());
// smallestUnit with a mode rounds the tail off.
console.log(a.until(b, { smallestUnit: "minute", roundingMode: "trunc" }).toString());
console.log(a.until(b, { smallestUnit: "minute", roundingMode: "ceil" }).toString());
console.log(a.until(b, { smallestUnit: "minute", roundingMode: "halfExpand" }).toString());

// roundingIncrement snaps to a multiple of the smallest unit.
console.log(a.until(b, { smallestUnit: "minute", roundingIncrement: 15, roundingMode: "floor" }).toString());

// A reversed pair yields a negative duration.
console.log(b.until(a).toString());
console.log(b.since(a).toString());

// Equal times are the zero duration.
console.log(a.until(a).toString());

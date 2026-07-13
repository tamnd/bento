// Temporal.PlainTime.prototype.add folds a duration into a wall clock. Only the time units
// count: days, weeks, months, and years leave the clock unchanged because they do not move
// the time of day. The result wraps mod 24 hours, so adding 25 hours advances one hour and
// subtracting past midnight lands on the previous day's clock. subtract is add over the
// negated duration. The argument may be a duration-like bag, a Temporal.Duration, or an ISO
// duration string. The values were checked against @js-temporal/polyfill.
const t = new Temporal.PlainTime(12, 30, 15);
console.log(t.add({ hours: 25 }).toString());
console.log(t.add({ days: 1 }).toString());
console.log(t.add({ months: 1 }).toString());
console.log(t.add({ hours: 90, minutes: 90 }).toString());
console.log(t.add({ milliseconds: 1500 }).toString());
console.log(t.add(Temporal.Duration.from("PT1H")).toString());
console.log(t.add("PT2H").toString());
console.log(t.subtract({ hours: 13 }).toString());
console.log(t.subtract({ minutes: 45 }).toString());

// The overflow option is accepted and ignored, since the fold never overflows a field.
console.log(t.add({ hours: 1 }, { overflow: "reject" }).toString());

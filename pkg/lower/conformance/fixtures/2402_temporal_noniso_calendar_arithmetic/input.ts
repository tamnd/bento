// The gregory, roc, and japanese calendars carry through the PlainDate and PlainDateTime movers the
// same way the ISO calendar does. Their date math runs on the proleptic-Gregorian ISO fields, so
// add, subtract, until, and since step and clamp exactly as ISO while the result keeps the receiver's
// calendar, and the era-bearing getters re-derive from the moved fields. with reshapes in the
// calendar's own reckoning: a roc year maps through 1911, a monthCode names the same month the
// getter reports since none of these calendars has a leap month, and the day clamps to the resulting
// month's length. Every result was checked against @js-temporal/polyfill.
const g = Temporal.PlainDate.from({ year: 2024, month: 1, day: 31, calendar: "gregory" });
console.log(g.add({ months: 1 }).toString());
console.log(g.subtract({ months: 2 }).toString());

const r = Temporal.PlainDate.from({ year: 113, month: 1, day: 31, calendar: "roc" });
console.log(r.add({ months: 1 }).toString());
const r2 = Temporal.PlainDate.from({ year: 114, month: 3, day: 1, calendar: "roc" });
console.log(r.until(r2, { largestUnit: "year" }).toString());
console.log(r2.since(r, { largestUnit: "year" }).toString());

const j = Temporal.PlainDate.from({ year: 2019, month: 4, day: 20, calendar: "japanese" });
const jp = j.add({ months: 1 });
console.log(jp.toString() + " " + jp.era + " " + jp.eraYear);

console.log(g.with({ monthCode: "M03" }).toString());
console.log(r.with({ monthCode: "M04" }).toString());
console.log(r.with({ year: 114 }).toString());
console.log(j.with({ year: 2020 }).toString());

const gdt = Temporal.PlainDateTime.from({ year: 2024, month: 1, day: 31, hour: 12, calendar: "gregory" });
console.log(gdt.add({ months: 1 }).toString());
console.log(gdt.with({ monthCode: "M03" }).toString());

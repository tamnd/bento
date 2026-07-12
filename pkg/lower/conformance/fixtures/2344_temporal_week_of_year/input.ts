// Temporal.PlainDate exposes the ISO 8601 week date, weekOfYear and yearOfWeek. The
// week can belong to the neighbouring year at a January or December boundary, so the
// week-numbering year differs from the calendar year there.
const a = new Temporal.PlainDate(2020, 1, 1);
console.log(a.weekOfYear);
console.log(a.yearOfWeek);
const b = new Temporal.PlainDate(2020, 6, 15);
console.log(b.weekOfYear);
const c = new Temporal.PlainDate(2020, 12, 31);
console.log(c.weekOfYear);
console.log(c.yearOfWeek);
const d = new Temporal.PlainDate(2021, 1, 1);
console.log(d.weekOfYear);
console.log(d.yearOfWeek);
const e = new Temporal.PlainDate(2023, 1, 1);
console.log(e.weekOfYear);
console.log(e.yearOfWeek);

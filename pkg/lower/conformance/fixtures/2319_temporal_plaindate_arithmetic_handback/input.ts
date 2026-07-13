// PlainDate.with reshapes a date from a bag of calendar fields, which needs the field
// resolution and constrain-or-reject balancing that a later slice carries. It hands back
// here so the compiler reports the ceiling rather than emit a wrong date. add, subtract,
// until, and since already lower.
const d = new Temporal.PlainDate(2020, 2, 29);
const e = d.with({ day: 15 });
console.log(e.day);

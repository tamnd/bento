// PlainDate difference hands back in this slice: until and since return the Duration
// between two dates, which needs the calendar field math to balance years, months, and
// days from the largest unit down. It waits on that difference model, so the compiler
// reports the ceiling rather than emit a wrong Duration. add and subtract already lower.
const d = new Temporal.PlainDate(2020, 2, 29);
const gap = d.until(new Temporal.PlainDate(2021, 3, 1));
console.log(gap.days);

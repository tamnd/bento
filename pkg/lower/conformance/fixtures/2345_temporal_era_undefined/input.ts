// The ISO 8601 calendar has no era, so era and eraYear read as undefined on both a
// PlainDate and a PlainDateTime, which answers the calendar-dependent getters off its
// date half. A narrowed weekOfYear reads as a plain number past the undefined guard.
const d = new Temporal.PlainDate(2020, 3, 15);
console.log(d.era);
console.log(d.eraYear);
const w = d.weekOfYear;
if (w !== undefined) {
  console.log(w + 100);
}
const dt = new Temporal.PlainDateTime(2020, 3, 15, 10, 30);
console.log(dt.era);
console.log(dt.weekOfYear);
console.log(dt.yearOfWeek);

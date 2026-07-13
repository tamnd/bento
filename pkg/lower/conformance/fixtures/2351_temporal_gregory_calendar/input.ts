// The gregory calendar shares the ISO date storage but interprets it: the proleptic
// year matches ISO, while era and eraYear split at the year boundary so ISO year 0
// reads as gregory-inverse eraYear 1 and ISO year -5 as eraYear 6. toString appends
// the RFC 9557 calendar annotation, and on a PlainDateTime it lands after the time.
// compare ignores the calendar; equals folds it in, so an iso date never equals a
// gregory one. Calendar ids are case-insensitive.
const d = new Temporal.PlainDate(2024, 6, 30, "gregory");
console.log(d.calendarId);
console.log(d.year);
console.log(d.era);
console.log(d.eraYear);
console.log(d.toString());

const bc = new Temporal.PlainDate(-5, 1, 1, "gregory");
console.log(bc.era);
console.log(bc.eraYear);

const zero = new Temporal.PlainDate(0, 1, 1, "gregory");
console.log(zero.eraYear);

const iso = new Temporal.PlainDate(2024, 6, 30);
console.log(Temporal.PlainDate.compare(d, iso));
console.log(d.equals(iso));
console.log(d.equals(d.withCalendar("gregory")));

const upper = new Temporal.PlainDate(2024, 6, 30, "GREGORY");
console.log(upper.calendarId);

const dt = new Temporal.PlainDateTime(2024, 6, 30, 12, 34, 56, 0, 0, 0, "gregory");
console.log(dt.toString());
console.log(dt.calendarId);

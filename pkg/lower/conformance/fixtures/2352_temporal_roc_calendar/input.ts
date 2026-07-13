// The roc (Minguo) calendar counts from 1912, so its year is the ISO year minus 1911
// while month and day stay ISO. The era splits at ISO year 1912: ISO 2024 is roc year
// 113 in the "roc" era, ISO 1912 is roc year 1, ISO 1911 is roc year 0 in the
// "roc-inverse" era eraYear 1, and ISO year -5 is "roc-inverse" eraYear 1917. toString
// prints the unchanged ISO year with the [u-ca=roc] annotation, after the time on a
// PlainDateTime. compare ignores the calendar; equals folds it in. Ids are
// case-insensitive.
const d = new Temporal.PlainDate(2024, 6, 30, "roc");
console.log(d.calendarId);
console.log(d.year);
console.log(d.era);
console.log(d.eraYear);
console.log(d.month, d.day);
console.log(d.toString());

const one = new Temporal.PlainDate(1912, 1, 1, "roc");
console.log(one.year, one.era, one.eraYear);

const zero = new Temporal.PlainDate(1911, 12, 31, "roc");
console.log(zero.year, zero.era, zero.eraYear);

const bc = new Temporal.PlainDate(-5, 1, 1, "roc");
console.log(bc.year, bc.eraYear);

const iso = new Temporal.PlainDate(2024, 6, 30);
console.log(Temporal.PlainDate.compare(d, iso));
console.log(d.equals(iso));

const upper = new Temporal.PlainDate(2024, 6, 30).withCalendar("ROC");
console.log(upper.calendarId);

const dt = new Temporal.PlainDateTime(2024, 6, 30, 12, 34, 56, 0, 0, 0, "roc");
console.log(dt.toString());
console.log(dt.year);
